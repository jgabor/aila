package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EditToolName  = "edit"
	WriteToolName = "write"

	MissingFileVersion = "missing"
)

const maxMutationErrorMessageBytes = 240

// MutationSourceMetadata records caller-visible provenance for mutation requests.
type MutationSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// EditRequest is the caller-facing edit tool contract before validation.
type EditRequest struct {
	Path           string
	TargetVersion  string
	OldText        string
	NewText        string
	ExpectedEffect string
	Source         MutationSourceMetadata
}

// WriteRequest is the caller-facing write tool contract before validation.
type WriteRequest struct {
	Path           string
	TargetVersion  string
	Content        string
	ExpectedEffect string
	Source         MutationSourceMetadata
}

// ValidatedEditRequest is safe, normalized, and ready for later execution.
type ValidatedEditRequest struct {
	ToolName                 string
	RequestedPath            string
	WorkspaceRoot            string
	WorkspaceRelativePath    string
	ResolvedPath             string
	RequestedPathWasAbsolute bool
	TargetVersion            string
	OldText                  string
	NewText                  string
	ExpectedEffect           string
	Source                   MutationSourceMetadata
}

// ValidatedWriteRequest is safe, normalized, and ready for later execution.
type ValidatedWriteRequest struct {
	ToolName                 string
	RequestedPath            string
	WorkspaceRoot            string
	WorkspaceRelativePath    string
	ResolvedPath             string
	RequestedPathWasAbsolute bool
	TargetVersion            string
	Content                  string
	ExpectedEffect           string
	Source                   MutationSourceMetadata
}

// MutationErrorKind is a bounded machine-readable mutation failure category.
type MutationErrorKind string

const (
	MutationErrorNone                  MutationErrorKind = "none"
	MutationErrorInvalidPath           MutationErrorKind = "invalid_path"
	MutationErrorOutsideWorkspace      MutationErrorKind = "outside_workspace"
	MutationErrorReservedPath          MutationErrorKind = "reserved_path"
	MutationErrorDirectoryLikePath     MutationErrorKind = "directory_like_path"
	MutationErrorInvalidContent        MutationErrorKind = "invalid_content"
	MutationErrorMissingFile           MutationErrorKind = "missing_file"
	MutationErrorDirectory             MutationErrorKind = "directory"
	MutationErrorPermission            MutationErrorKind = "permission_denied"
	MutationErrorSymlinkEscape         MutationErrorKind = "symlink_escape"
	MutationErrorTargetVersionMismatch MutationErrorKind = "target_version_mismatch"
	MutationErrorOldTextMismatch       MutationErrorKind = "old_text_mismatch"
	MutationErrorCanceled              MutationErrorKind = "canceled"
	MutationErrorExecution             MutationErrorKind = "execution_error"
)

// MutationError is safe to surface to callers without leaking host-local paths.
type MutationError struct {
	Kind    MutationErrorKind
	Message string
}

// MutationResult is the deterministic success/failure shape returned by edit/write paths.
type MutationResult struct {
	ToolName              string
	WorkspaceRelativePath string
	ResolvedPath          string
	ResolvedPathAvailable bool
	Status                string
	ExpectedEffect        string
	PreviousVersion       string
	NewVersion            string
	PreviousExists        bool
	BytesWritten          int
	ReplacementCount      int
	Error                 MutationError
	Source                MutationSourceMetadata
}

// ValidateEditRequest applies defaults and rejects paths unsafe for a workspace edit executor.
func ValidateEditRequest(workspaceRoot string, request EditRequest) (ValidatedEditRequest, MutationError) {
	path, err := validateMutationPath(workspaceRoot, request.Path)
	if err.Kind != "" {
		return ValidatedEditRequest{}, err
	}
	oldText := request.OldText
	if oldText == "" {
		return ValidatedEditRequest{}, mutationError(MutationErrorInvalidContent, "old text is required")
	}
	return ValidatedEditRequest{
		ToolName:                 EditToolName,
		RequestedPath:            request.Path,
		WorkspaceRoot:            path.workspaceRoot,
		WorkspaceRelativePath:    path.workspaceRelativePath,
		ResolvedPath:             path.resolvedPath,
		RequestedPathWasAbsolute: path.requestedPathWasAbsolute,
		TargetVersion:            strings.TrimSpace(request.TargetVersion),
		OldText:                  oldText,
		NewText:                  request.NewText,
		ExpectedEffect:           strings.TrimSpace(request.ExpectedEffect),
		Source:                   request.Source,
	}, MutationError{}
}

// ValidateWriteRequest applies defaults and rejects paths unsafe for a workspace write executor.
func ValidateWriteRequest(workspaceRoot string, request WriteRequest) (ValidatedWriteRequest, MutationError) {
	path, err := validateMutationPath(workspaceRoot, request.Path)
	if err.Kind != "" {
		return ValidatedWriteRequest{}, err
	}
	return ValidatedWriteRequest{
		ToolName:                 WriteToolName,
		RequestedPath:            request.Path,
		WorkspaceRoot:            path.workspaceRoot,
		WorkspaceRelativePath:    path.workspaceRelativePath,
		ResolvedPath:             path.resolvedPath,
		RequestedPathWasAbsolute: path.requestedPathWasAbsolute,
		TargetVersion:            strings.TrimSpace(request.TargetVersion),
		Content:                  request.Content,
		ExpectedEffect:           strings.TrimSpace(request.ExpectedEffect),
		Source:                   request.Source,
	}, MutationError{}
}

// ExecuteWrite writes a validated workspace file through the tool/effect path.
func ExecuteWrite(ctx context.Context, request ValidatedWriteRequest) MutationResult {
	if err := ctx.Err(); err != nil {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, request.ResolvedPath, request.ExpectedEffect, request.Source, mutationExecutionError(err))
	}
	resolvedPath, err := resolveMutationTarget(request.WorkspaceRoot, request.ResolvedPath)
	if err.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, request.ResolvedPath, request.ExpectedEffect, request.Source, err)
	}
	request.ResolvedPath = resolvedPath

	previousExists, previousVersion, versionErr := mutationFileVersion(resolvedPath)
	if versionErr.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, versionErr)
	}
	if mismatch := checkTargetVersion(request.TargetVersion, previousVersion); mismatch.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mismatch)
	}
	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mutationExecutionError(err))
	}
	if err := os.WriteFile(resolvedPath, []byte(request.Content), 0o644); err != nil {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mutationExecutionError(err))
	}
	_, newVersion, versionErr := mutationFileVersion(resolvedPath)
	if versionErr.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, versionErr)
	}
	return MutationResult{
		ToolName:              request.ToolName,
		WorkspaceRelativePath: request.WorkspaceRelativePath,
		ResolvedPath:          resolvedPath,
		ResolvedPathAvailable: true,
		Status:                "completed",
		ExpectedEffect:        request.ExpectedEffect,
		PreviousVersion:       previousVersion,
		NewVersion:            newVersion,
		PreviousExists:        previousExists,
		BytesWritten:          len([]byte(request.Content)),
		Source:                request.Source,
	}
}

// ExecuteEdit edits a validated workspace file through the tool/effect path.
func ExecuteEdit(ctx context.Context, request ValidatedEditRequest) MutationResult {
	if err := ctx.Err(); err != nil {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, request.ResolvedPath, request.ExpectedEffect, request.Source, mutationExecutionError(err))
	}
	resolvedPath, err := resolveMutationTarget(request.WorkspaceRoot, request.ResolvedPath)
	if err.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, request.ResolvedPath, request.ExpectedEffect, request.Source, err)
	}
	request.ResolvedPath = resolvedPath

	previousExists, previousVersion, versionErr := mutationFileVersion(resolvedPath)
	if versionErr.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, versionErr)
	}
	if !previousExists {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mutationError(MutationErrorMissingFile, "file does not exist"))
	}
	if mismatch := checkTargetVersion(request.TargetVersion, previousVersion); mismatch.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mismatch)
	}
	content, readErr := os.ReadFile(resolvedPath)
	if readErr != nil {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mutationExecutionError(readErr))
	}
	text := string(content)
	count := strings.Count(text, request.OldText)
	if count != 1 {
		message := "old text did not match exactly once"
		if count > 1 {
			message = "old text matched more than once"
		}
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mutationError(MutationErrorOldTextMismatch, message))
	}
	updated := strings.Replace(text, request.OldText, request.NewText, 1)
	if err := os.WriteFile(resolvedPath, []byte(updated), 0o644); err != nil {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, mutationExecutionError(err))
	}
	_, newVersion, versionErr := mutationFileVersion(resolvedPath)
	if versionErr.Kind != "" {
		return NewMutationFailure(request.ToolName, request.WorkspaceRelativePath, resolvedPath, request.ExpectedEffect, request.Source, versionErr)
	}
	return MutationResult{
		ToolName:              request.ToolName,
		WorkspaceRelativePath: request.WorkspaceRelativePath,
		ResolvedPath:          resolvedPath,
		ResolvedPathAvailable: true,
		Status:                "completed",
		ExpectedEffect:        request.ExpectedEffect,
		PreviousVersion:       previousVersion,
		NewVersion:            newVersion,
		PreviousExists:        true,
		BytesWritten:          len([]byte(updated)),
		ReplacementCount:      1,
		Source:                request.Source,
	}
}

// NewMutationFailure returns a bounded mutation failure result.
func NewMutationFailure(toolName string, workspaceRelativePath string, resolvedPath string, expectedEffect string, source MutationSourceMetadata, err MutationError) MutationResult {
	status := "failed"
	if err.Kind == MutationErrorPermission {
		status = "denied"
	}
	return MutationResult{
		ToolName:              toolName,
		WorkspaceRelativePath: workspaceRelativePath,
		ResolvedPath:          resolvedPath,
		ResolvedPathAvailable: resolvedPath != "",
		Status:                status,
		ExpectedEffect:        expectedEffect,
		Error:                 err,
		Source:                source,
	}
}

type mutationPath struct {
	workspaceRoot            string
	workspaceRelativePath    string
	resolvedPath             string
	requestedPathWasAbsolute bool
}

func validateMutationPath(workspaceRoot string, requestedPath string) (mutationPath, MutationError) {
	root := filepath.Clean(workspaceRoot)
	if root == "." || !filepath.IsAbs(root) {
		return mutationPath{}, mutationError(MutationErrorInvalidPath, "workspace root must be absolute")
	}
	path := strings.TrimSpace(requestedPath)
	if path == "" {
		return mutationPath{}, mutationError(MutationErrorInvalidPath, "path is required")
	}
	if isHomeOrXDGPath(path) {
		return mutationPath{}, mutationError(MutationErrorReservedPath, "home and xdg paths are not writable by this contract")
	}
	if isDirectoryLikePath(path) {
		return mutationPath{}, mutationError(MutationErrorDirectoryLikePath, "directory-like paths are not writable")
	}
	if hasTraversal(path) {
		return mutationPath{}, mutationError(MutationErrorInvalidPath, "path traversal is not allowed")
	}

	cleanPath := filepath.Clean(path)
	wasAbsolute := filepath.IsAbs(cleanPath)
	resolvedPath := cleanPath
	if !wasAbsolute {
		resolvedPath = filepath.Join(root, cleanPath)
	}
	resolvedPath = filepath.Clean(resolvedPath)
	workspaceRelativePath, relErr := filepath.Rel(root, resolvedPath)
	if relErr != nil || workspaceRelativePath == "." || strings.HasPrefix(workspaceRelativePath, ".."+string(filepath.Separator)) || workspaceRelativePath == ".." || filepath.IsAbs(workspaceRelativePath) {
		return mutationPath{}, mutationError(MutationErrorOutsideWorkspace, "path must stay inside the workspace")
	}
	workspaceRelativePath = filepath.ToSlash(workspaceRelativePath)
	if isReservedWorkspacePath(workspaceRelativePath) {
		return mutationPath{}, mutationError(MutationErrorReservedPath, "reserved workspace paths are not writable")
	}
	return mutationPath{workspaceRoot: root, workspaceRelativePath: workspaceRelativePath, resolvedPath: resolvedPath, requestedPathWasAbsolute: wasAbsolute}, MutationError{}
}

func resolveMutationTarget(workspaceRoot string, resolvedPath string) (string, MutationError) {
	root := filepath.Clean(workspaceRoot)
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", mutationExecutionError(err)
	}
	if info, statErr := os.Lstat(resolvedPath); statErr == nil {
		if info.IsDir() {
			return "", mutationError(MutationErrorDirectory, "path is a directory")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, evalErr := filepath.EvalSymlinks(resolvedPath)
			if evalErr != nil {
				return "", mutationExecutionError(evalErr)
			}
			if !pathWithin(rootEval, target) {
				return "", mutationError(MutationErrorSymlinkEscape, "symlink target escapes workspace")
			}
			return target, MutationError{}
		}
		return resolvedPath, MutationError{}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", mutationExecutionError(statErr)
	}

	parent := filepath.Dir(resolvedPath)
	for {
		info, statErr := os.Lstat(parent)
		if statErr == nil {
			if !info.IsDir() {
				return "", mutationError(MutationErrorDirectory, "parent path is not a directory")
			}
			parentEval, evalErr := filepath.EvalSymlinks(parent)
			if evalErr != nil {
				return "", mutationExecutionError(evalErr)
			}
			if !pathWithin(rootEval, parentEval) {
				return "", mutationError(MutationErrorSymlinkEscape, "parent path escapes workspace")
			}
			return resolvedPath, MutationError{}
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", mutationExecutionError(statErr)
		}
		nextParent := filepath.Dir(parent)
		if nextParent == parent || !pathWithin(root, nextParent) {
			return "", mutationError(MutationErrorOutsideWorkspace, "path must stay inside the workspace")
		}
		parent = nextParent
	}
}

func pathWithin(root string, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func mutationFileVersion(path string) (bool, string, MutationError) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, MissingFileVersion, MutationError{}
	}
	if err != nil {
		return false, "", mutationExecutionError(err)
	}
	sum := sha256.Sum256(content)
	return true, "sha256:" + hex.EncodeToString(sum[:]), MutationError{}
}

func checkTargetVersion(expected string, actual string) MutationError {
	expected = strings.TrimSpace(expected)
	if expected == "" || expected == actual {
		return MutationError{}
	}
	return mutationError(MutationErrorTargetVersionMismatch, fmt.Sprintf("target version mismatch: expected %s", expected))
}

func mutationError(kind MutationErrorKind, message string) MutationError {
	return MutationError{Kind: kind, Message: boundMutationErrorMessage(message)}
}

func mutationExecutionError(err error) MutationError {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return mutationError(MutationErrorCanceled, "mutation canceled")
	}
	if errors.Is(err, os.ErrPermission) {
		return mutationError(MutationErrorPermission, "permission denied")
	}
	return mutationError(MutationErrorExecution, err.Error())
}

func boundMutationErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= maxMutationErrorMessageBytes {
		return message
	}
	return message[:maxMutationErrorMessageBytes] + "..."
}
