package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	BashToolName = "bash"

	DefaultBashMaxOutputBytes = 32 * 1024
	DefaultBashTimeoutMillis  = 5000
)

const maxBashErrorMessageBytes = 240

// BashSourceMetadata records caller-visible provenance for a safe bash request.
type BashSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// BashRequest is the caller-facing safe inspection command contract before validation.
type BashRequest struct {
	Argv           []string
	WorkingDir     string
	MaxOutputBytes int
	TimeoutMillis  int
	Source         BashSourceMetadata
}

// ValidatedBashRequest is safe, normalized, and ready for later execution.
type ValidatedBashRequest struct {
	ToolName                 string
	RequestedArgv            []string
	EffectiveArgv            []string
	WorkspaceRoot            string
	RequestedWorkingDir      string
	WorkspaceRelativeWorkDir string
	ResolvedWorkingDir       string
	CommandFamily            string
	ExpectedEffect           string
	RequestedMaxOutputBytes  int
	RequestedTimeoutMillis   int
	EffectiveMaxOutputBytes  int
	EffectiveTimeoutMillis   int
	Source                   BashSourceMetadata
}

// BashOutput records one bounded command output stream.
type BashOutput struct {
	Text      string
	Bytes     int
	Truncated bool
}

// BashErrorKind is a bounded machine-readable safe bash failure category.
type BashErrorKind string

const (
	BashErrorNone             BashErrorKind = "none"
	BashErrorInvalidCommand   BashErrorKind = "invalid_command"
	BashErrorUnsafeCommand    BashErrorKind = "unsafe_command"
	BashErrorInvalidPath      BashErrorKind = "invalid_path"
	BashErrorOutsideWorkspace BashErrorKind = "outside_workspace"
	BashErrorReservedPath     BashErrorKind = "reserved_path"
	BashErrorInvalidRange     BashErrorKind = "invalid_range"
	BashErrorPermission       BashErrorKind = "permission_denied"
	BashErrorCanceled         BashErrorKind = "canceled"
	BashErrorTimeout          BashErrorKind = "timeout"
	BashErrorExecution        BashErrorKind = "execution_error"
)

// BashError is safe to surface to callers without leaking host-local paths.
type BashError struct {
	Kind    BashErrorKind
	Message string
}

// BashResult is the deterministic success/failure shape returned by safe bash paths.
type BashResult struct {
	ToolName                 string
	RequestedArgv            []string
	EffectiveArgv            []string
	WorkspaceRelativeWorkDir string
	CommandFamily            string
	ExpectedEffect           string
	ExitCode                 int
	Status                   string
	Stdout                   BashOutput
	Stderr                   BashOutput
	DurationMillis           int64
	Error                    BashError
	Source                   BashSourceMetadata
}

// ValidateBashRequest applies defaults and rejects commands outside M20's
// explicitly safe inspection allowlist. It performs only lexical validation.
func ValidateBashRequest(workspaceRoot string, request BashRequest) (ValidatedBashRequest, BashError) {
	root := filepath.Clean(workspaceRoot)
	if root == "." || !filepath.IsAbs(root) {
		return ValidatedBashRequest{}, bashError(BashErrorInvalidPath, "workspace root must be absolute")
	}
	argv := normalizeArgv(request.Argv)
	if len(argv) == 0 {
		return ValidatedBashRequest{}, bashError(BashErrorInvalidCommand, "command argv is required")
	}
	for _, arg := range argv {
		if arg == "" || looksLikeShellSyntax(arg) || looksLikeEnvironmentAssignment(arg) || isHomeOrXDGPath(arg) {
			return ValidatedBashRequest{}, bashError(BashErrorUnsafeCommand, "command contains unsupported shell syntax or host-local reference")
		}
	}

	workDir, relWorkDir, err := validateBashWorkingDir(root, request.WorkingDir)
	if err.Kind != "" {
		return ValidatedBashRequest{}, err
	}

	maxOutputBytes := request.MaxOutputBytes
	if maxOutputBytes == 0 {
		maxOutputBytes = DefaultBashMaxOutputBytes
	}
	if maxOutputBytes < 1 {
		return ValidatedBashRequest{}, bashError(BashErrorInvalidRange, "max output bytes must be positive")
	}
	timeoutMillis := request.TimeoutMillis
	if timeoutMillis == 0 {
		timeoutMillis = DefaultBashTimeoutMillis
	}
	if timeoutMillis < 1 {
		return ValidatedBashRequest{}, bashError(BashErrorInvalidRange, "timeout millis must be positive")
	}

	effectiveArgv, family, expected, err := validateSafeInspectionArgv(root, workDir, argv)
	if err.Kind != "" {
		return ValidatedBashRequest{}, err
	}

	return ValidatedBashRequest{
		ToolName:                 BashToolName,
		RequestedArgv:            append([]string(nil), request.Argv...),
		EffectiveArgv:            effectiveArgv,
		WorkspaceRoot:            root,
		RequestedWorkingDir:      request.WorkingDir,
		WorkspaceRelativeWorkDir: relWorkDir,
		ResolvedWorkingDir:       workDir,
		CommandFamily:            family,
		ExpectedEffect:           expected,
		RequestedMaxOutputBytes:  request.MaxOutputBytes,
		RequestedTimeoutMillis:   request.TimeoutMillis,
		EffectiveMaxOutputBytes:  maxOutputBytes,
		EffectiveTimeoutMillis:   timeoutMillis,
		Source:                   request.Source,
	}, BashError{}
}

// ExecuteBash runs a validated safe inspection command through argv execution,
// never through shell string evaluation.
func ExecuteBash(ctx context.Context, request ValidatedBashRequest) BashResult {
	if err := ctx.Err(); err != nil {
		return NewBashFailure(request, bashExecutionError(err), -1, "canceled")
	}
	if len(request.EffectiveArgv) == 0 {
		return NewBashFailure(request, bashError(BashErrorInvalidCommand, "effective command argv is required"), -1, "invalid")
	}
	if request.EffectiveMaxOutputBytes < 1 || request.EffectiveTimeoutMillis < 1 {
		return NewBashFailure(request, bashError(BashErrorInvalidRange, "effective bash bounds must be positive"), -1, "invalid")
	}

	realRoot, err := filepath.EvalSymlinks(request.WorkspaceRoot)
	if err != nil {
		return NewBashFailure(request, bashExecutionError(err), -1, "failed")
	}
	realWorkDir, err := filepath.EvalSymlinks(request.ResolvedWorkingDir)
	if err != nil {
		return NewBashFailure(request, bashExecutionError(err), -1, "failed")
	}
	if !pathInsideOrEqual(realRoot, realWorkDir) {
		return NewBashFailure(request, bashError(BashErrorOutsideWorkspace, "working directory escapes workspace"), -1, "failed")
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(request.EffectiveTimeoutMillis)*time.Millisecond)
	defer cancel()
	started := time.Now()
	cmd := exec.CommandContext(execCtx, request.EffectiveArgv[0], request.EffectiveArgv[1:]...)
	cmd.Dir = realWorkDir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_CEILING_DIRECTORIES="+filepath.Dir(realRoot))
	var stdout, stderr boundedBuffer
	stdout.limit = request.EffectiveMaxOutputBytes + 1
	stderr.limit = request.EffectiveMaxOutputBytes + 1
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	duration := time.Since(started).Milliseconds()
	stdoutText, stdoutTruncated := boundPreview(stdout.String(), request.EffectiveMaxOutputBytes)
	stderrText, stderrTruncated := boundPreview(stderr.String(), request.EffectiveMaxOutputBytes)
	result := BashResult{
		ToolName:                 BashToolName,
		RequestedArgv:            append([]string(nil), request.RequestedArgv...),
		EffectiveArgv:            append([]string(nil), request.EffectiveArgv...),
		WorkspaceRelativeWorkDir: request.WorkspaceRelativeWorkDir,
		CommandFamily:            request.CommandFamily,
		ExpectedEffect:           request.ExpectedEffect,
		ExitCode:                 0,
		Status:                   "completed",
		Stdout:                   BashOutput{Text: stdoutText, Bytes: len(stdoutText), Truncated: stdoutTruncated},
		Stderr:                   BashOutput{Text: stderrText, Bytes: len(stderrText), Truncated: stderrTruncated},
		DurationMillis:           duration,
		Error:                    BashError{Kind: BashErrorNone},
		Source:                   request.Source,
	}
	if runErr == nil {
		return result
	}
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		result.Status = "timeout"
		result.ExitCode = -1
		result.Error = bashError(BashErrorTimeout, "command timed out")
		return result
	}
	if errors.Is(execCtx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		result.Status = "canceled"
		result.ExitCode = -1
		result.Error = bashError(BashErrorCanceled, "command canceled")
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.Status = "failed"
		result.ExitCode = exitErr.ExitCode()
		result.Error = bashError(BashErrorExecution, "command exited with non-zero status")
		return result
	}
	result.Status = "failed"
	result.ExitCode = -1
	result.Error = bashExecutionError(runErr)
	return result
}

// NewBashFailure shapes validation or execution failures without running a command.
func NewBashFailure(request ValidatedBashRequest, err BashError, exitCode int, status string) BashResult {
	if err.Kind == "" {
		err.Kind = BashErrorExecution
	}
	if status == "" {
		status = "failed"
	}
	err.Message = boundString(err.Message, maxBashErrorMessageBytes)
	return BashResult{
		ToolName:                 BashToolName,
		RequestedArgv:            append([]string(nil), request.RequestedArgv...),
		EffectiveArgv:            append([]string(nil), request.EffectiveArgv...),
		WorkspaceRelativeWorkDir: request.WorkspaceRelativeWorkDir,
		CommandFamily:            request.CommandFamily,
		ExpectedEffect:           request.ExpectedEffect,
		ExitCode:                 exitCode,
		Status:                   status,
		Error:                    err,
		Source:                   request.Source,
	}
}

func validateBashWorkingDir(root string, requested string) (string, string, BashError) {
	workDir := strings.TrimSpace(requested)
	if workDir == "" || workDir == "." {
		return root, ".", BashError{}
	}
	if filepath.IsAbs(workDir) || isHomeOrXDGPath(workDir) || hasTraversal(workDir) || isReservedWorkspacePath(workDir) {
		return "", "", bashError(BashErrorInvalidPath, "working directory must stay inside the workspace")
	}
	resolved := filepath.Clean(filepath.Join(root, workDir))
	rel, relErr := filepath.Rel(root, resolved)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", bashError(BashErrorOutsideWorkspace, "working directory must stay inside the workspace")
	}
	rel = filepath.ToSlash(rel)
	if isReservedWorkspacePath(rel) {
		return "", "", bashError(BashErrorReservedPath, "reserved workspace paths are not valid working directories")
	}
	return resolved, rel, BashError{}
}

func validateSafeInspectionArgv(root string, workDir string, argv []string) ([]string, string, string, BashError) {
	switch argv[0] {
	case "pwd":
		if len(argv) != 1 {
			return nil, "", "", bashError(BashErrorUnsafeCommand, "pwd accepts no arguments in safe inspection mode")
		}
		return append([]string(nil), argv...), "pwd", "print workspace working directory", BashError{}
	case "ls":
		for _, arg := range argv[1:] {
			if strings.HasPrefix(arg, "-") {
				if !allowedLSFlag(arg) {
					return nil, "", "", bashError(BashErrorUnsafeCommand, "ls flag is not allowed")
				}
				continue
			}
			if err := validateSafeCommandPath(root, workDir, arg); err.Kind != "" {
				return nil, "", "", err
			}
		}
		return append([]string(nil), argv...), "ls", "list workspace files", BashError{}
	case "git":
		if len(argv) < 2 {
			return nil, "", "", bashError(BashErrorUnsafeCommand, "git subcommand is required")
		}
		switch argv[1] {
		case "status":
			for _, arg := range argv[2:] {
				if !allowedGitStatusArg(arg) {
					return nil, "", "", bashError(BashErrorUnsafeCommand, "git status argument is not allowed")
				}
			}
			return append([]string(nil), argv...), "git status", "inspect git working tree status", BashError{}
		case "diff":
			if err := validateGitDiffArgs(root, workDir, argv[2:]); err.Kind != "" {
				return nil, "", "", err
			}
			effective := append([]string{"git", "diff", "--no-ext-diff"}, argv[2:]...)
			return effective, "git diff", "inspect git diff output", BashError{}
		default:
			return nil, "", "", bashError(BashErrorUnsafeCommand, "git subcommand is not allowed")
		}
	default:
		return nil, "", "", bashError(BashErrorUnsafeCommand, "command is not allowed for safe inspection")
	}
}

func validateSafeCommandPath(root string, workDir string, value string) BashError {
	pathValue := strings.TrimSpace(value)
	if pathValue == "" || filepath.IsAbs(pathValue) || isHomeOrXDGPath(pathValue) || hasTraversal(pathValue) || hasShellGlob(pathValue) {
		return bashError(BashErrorInvalidPath, "command path must be a literal workspace-relative path")
	}
	resolved := filepath.Clean(filepath.Join(workDir, pathValue))
	rel, relErr := filepath.Rel(root, resolved)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return bashError(BashErrorOutsideWorkspace, "command path must stay inside the workspace")
	}
	if isReservedWorkspacePath(filepath.ToSlash(rel)) {
		return bashError(BashErrorReservedPath, "reserved workspace paths are not inspectable")
	}
	return BashError{}
}

func validateGitDiffArgs(root string, workDir string, args []string) BashError {
	pathMode := false
	for _, arg := range args {
		if arg == "--" {
			pathMode = true
			continue
		}
		if pathMode || !strings.HasPrefix(arg, "-") {
			if err := validateSafeCommandPath(root, workDir, arg); err.Kind != "" {
				return err
			}
			continue
		}
		if !allowedGitDiffFlag(arg) {
			return bashError(BashErrorUnsafeCommand, "git diff argument is not allowed")
		}
	}
	return BashError{}
}

func normalizeArgv(argv []string) []string {
	normalized := make([]string, 0, len(argv))
	for _, arg := range argv {
		normalized = append(normalized, strings.TrimSpace(arg))
	}
	return normalized
}

func allowedLSFlag(arg string) bool {
	switch arg {
	case "-1", "-a", "-l", "-la", "-al":
		return true
	default:
		return false
	}
}

func allowedGitStatusArg(arg string) bool {
	switch arg {
	case "--short", "-s", "--branch", "-b", "--porcelain", "--porcelain=v1", "--show-stash":
		return true
	default:
		return false
	}
}

func allowedGitDiffFlag(arg string) bool {
	switch arg {
	case "--cached", "--staged", "--stat", "--name-only", "--name-status", "--check", "--color=never", "--", "-U0":
		return true
	default:
		return false
	}
}

func looksLikeShellSyntax(arg string) bool {
	return strings.ContainsAny(arg, "\x00\n\r;&|<>()`$")
}

func looksLikeEnvironmentAssignment(arg string) bool {
	name, _, ok := strings.Cut(arg, "=")
	if !ok || name == "" || strings.HasPrefix(arg, "--") {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func hasShellGlob(arg string) bool {
	return strings.ContainsAny(arg, "*?[]{}")
}

func bashError(kind BashErrorKind, message string) BashError {
	return BashError{Kind: kind, Message: boundString(message, maxBashErrorMessageBytes)}
}

func bashExecutionError(err error) BashError {
	switch {
	case errors.Is(err, context.Canceled):
		return bashError(BashErrorCanceled, "command canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return bashError(BashErrorTimeout, "command timed out")
	case errors.Is(err, os.ErrPermission):
		return bashError(BashErrorPermission, "command is not executable")
	default:
		return bashError(BashErrorExecution, "command failed")
	}
}

func pathInsideOrEqual(root string, value string) bool {
	if root == value {
		return true
	}
	return pathInside(root, value)
}

type boundedBuffer struct {
	limit int
	buf   bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 || b.buf.Len() >= b.limit {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	value := b.buf.String()
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

func FormatBashArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return strings.Join(argv, " ")
}

func BashResultSummary(result BashResult) string {
	command := FormatBashArgv(result.RequestedArgv)
	if result.Error.Kind != "" && result.Error.Kind != BashErrorNone {
		return fmt.Sprintf("bash %s failed: %s", command, result.Error.Message)
	}
	return fmt.Sprintf("bash %s completed with status %s", command, result.Status)
}
