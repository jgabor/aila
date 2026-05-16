package tools

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	ReadToolName = "read"

	DefaultReadStartLine       = 1
	DefaultReadLineLimit       = 120
	DefaultReadMaxPreviewBytes = 32 * 1024
)

const maxReadErrorMessageBytes = 240

// ReadRequest is the caller-facing read tool contract before validation.
type ReadRequest struct {
	Path            string
	StartLine       int
	LineLimit       int
	MaxPreviewBytes int
	Source          ReadSourceMetadata
}

// ReadSourceMetadata records caller-visible provenance for a read request.
type ReadSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// ValidatedReadRequest is safe, normalized, and ready for later execution.
type ValidatedReadRequest struct {
	ToolName                 string
	RequestedPath            string
	WorkspaceRelativePath    string
	ResolvedPath             string
	RequestedPathWasAbsolute bool
	RequestedStartLine       int
	RequestedLineLimit       int
	RequestedMaxPreviewBytes int
	EffectiveStartLine       int
	EffectiveLineLimit       int
	EffectiveMaxPreviewBytes int
	Source                   ReadSourceMetadata
}

// ReadLineRange records inclusive 1-based line bounds. EndLine is 0 when no
// line has been selected yet.
type ReadLineRange struct {
	StartLine int
	EndLine   int
	Limit     int
}

// ReadTruncation records bounded preview truncation decisions.
type ReadTruncation struct {
	PreviewBytesLimit int
	PreviewTruncated  bool
	LineLimitHit      bool
	Marker            string
}

// ReadErrorKind is a bounded machine-readable read failure category.
type ReadErrorKind string

const (
	ReadErrorNone              ReadErrorKind = "none"
	ReadErrorInvalidPath       ReadErrorKind = "invalid_path"
	ReadErrorOutsideWorkspace  ReadErrorKind = "outside_workspace"
	ReadErrorReservedPath      ReadErrorKind = "reserved_path"
	ReadErrorDirectoryLikePath ReadErrorKind = "directory_like_path"
	ReadErrorInvalidRange      ReadErrorKind = "invalid_range"
	ReadErrorExecution         ReadErrorKind = "execution_error"
)

// ReadError is safe to surface to callers without leaking host-local paths.
type ReadError struct {
	Kind    ReadErrorKind
	Message string
}

// ReadResult is the deterministic success/failure shape returned by read paths.
type ReadResult struct {
	ToolName              string
	WorkspaceRelativePath string
	ResolvedPath          string
	ResolvedPathAvailable bool
	RequestedRange        ReadLineRange
	EffectiveRange        ReadLineRange
	PreviewText           string
	Truncation            ReadTruncation
	Error                 ReadError
	Source                ReadSourceMetadata
}

// ValidateReadRequest applies defaults and rejects paths that are unsafe for a
// future workspace read executor. It performs only lexical path validation.
func ValidateReadRequest(workspaceRoot string, request ReadRequest) (ValidatedReadRequest, ReadError) {
	root := filepath.Clean(workspaceRoot)
	if root == "." || !filepath.IsAbs(root) {
		return ValidatedReadRequest{}, readError(ReadErrorInvalidPath, "workspace root must be absolute")
	}

	path := strings.TrimSpace(request.Path)
	if path == "" {
		return ValidatedReadRequest{}, readError(ReadErrorInvalidPath, "path is required")
	}
	if isHomeOrXDGPath(path) {
		return ValidatedReadRequest{}, readError(ReadErrorReservedPath, "home and xdg paths are not readable by this contract")
	}
	if isDirectoryLikePath(path) {
		return ValidatedReadRequest{}, readError(ReadErrorDirectoryLikePath, "directory-like paths are not readable")
	}
	if hasTraversal(path) {
		return ValidatedReadRequest{}, readError(ReadErrorInvalidPath, "path traversal is not allowed")
	}

	cleanPath := filepath.Clean(path)
	wasAbsolute := filepath.IsAbs(cleanPath)

	resolvedPath := cleanPath
	if !wasAbsolute {
		resolvedPath = filepath.Join(root, cleanPath)
	}
	resolvedPath = filepath.Clean(resolvedPath)

	workspaceRelativePath, err := filepath.Rel(root, resolvedPath)
	if err != nil || workspaceRelativePath == "." || strings.HasPrefix(workspaceRelativePath, ".."+string(filepath.Separator)) || workspaceRelativePath == ".." || filepath.IsAbs(workspaceRelativePath) {
		return ValidatedReadRequest{}, readError(ReadErrorOutsideWorkspace, "path must stay inside the workspace")
	}
	workspaceRelativePath = filepath.ToSlash(workspaceRelativePath)
	if isReservedWorkspacePath(workspaceRelativePath) {
		return ValidatedReadRequest{}, readError(ReadErrorReservedPath, "reserved workspace paths are not readable")
	}

	startLine := request.StartLine
	if startLine == 0 {
		startLine = DefaultReadStartLine
	}
	if startLine < 1 {
		return ValidatedReadRequest{}, readError(ReadErrorInvalidRange, "start line must be positive")
	}

	lineLimit := request.LineLimit
	if lineLimit == 0 {
		lineLimit = DefaultReadLineLimit
	}
	if lineLimit < 1 {
		return ValidatedReadRequest{}, readError(ReadErrorInvalidRange, "line limit must be positive")
	}

	maxPreviewBytes := request.MaxPreviewBytes
	if maxPreviewBytes == 0 {
		maxPreviewBytes = DefaultReadMaxPreviewBytes
	}
	if maxPreviewBytes < 1 {
		return ValidatedReadRequest{}, readError(ReadErrorInvalidRange, "max preview bytes must be positive")
	}

	return ValidatedReadRequest{
		ToolName:                 ReadToolName,
		RequestedPath:            request.Path,
		WorkspaceRelativePath:    workspaceRelativePath,
		ResolvedPath:             resolvedPath,
		RequestedPathWasAbsolute: wasAbsolute,
		RequestedStartLine:       request.StartLine,
		RequestedLineLimit:       request.LineLimit,
		RequestedMaxPreviewBytes: request.MaxPreviewBytes,
		EffectiveStartLine:       startLine,
		EffectiveLineLimit:       lineLimit,
		EffectiveMaxPreviewBytes: maxPreviewBytes,
		Source:                   request.Source,
	}, ReadError{}
}

// NewReadSuccess shapes already-produced preview text into the public read result
// contract. Later execution code will own obtaining the preview bytes.
func NewReadSuccess(request ValidatedReadRequest, previewText string, effectiveEndLine int, lineLimitHit bool) ReadResult {
	preview, previewTruncated := boundPreview(previewText, request.EffectiveMaxPreviewBytes)
	marker := ""
	if previewTruncated {
		marker = "preview_truncated"
	}
	if lineLimitHit {
		if marker != "" {
			marker += ","
		}
		marker += "line_limit_hit"
	}

	return ReadResult{
		ToolName:              ReadToolName,
		WorkspaceRelativePath: request.WorkspaceRelativePath,
		ResolvedPath:          request.ResolvedPath,
		ResolvedPathAvailable: true,
		RequestedRange: ReadLineRange{
			StartLine: requestedOrDefault(request.RequestedStartLine, request.EffectiveStartLine),
			EndLine:   0,
			Limit:     requestedOrDefault(request.RequestedLineLimit, request.EffectiveLineLimit),
		},
		EffectiveRange: ReadLineRange{
			StartLine: request.EffectiveStartLine,
			EndLine:   effectiveEndLine,
			Limit:     request.EffectiveLineLimit,
		},
		PreviewText: preview,
		Truncation: ReadTruncation{
			PreviewBytesLimit: request.EffectiveMaxPreviewBytes,
			PreviewTruncated:  previewTruncated,
			LineLimitHit:      lineLimitHit,
			Marker:            marker,
		},
		Error:  ReadError{Kind: ReadErrorNone},
		Source: request.Source,
	}
}

// NewReadFailure shapes validation or execution failures without requiring file IO.
func NewReadFailure(request ValidatedReadRequest, err ReadError) ReadResult {
	if err.Kind == "" {
		err.Kind = ReadErrorExecution
	}
	err.Message = boundString(err.Message, maxReadErrorMessageBytes)

	return ReadResult{
		ToolName:              ReadToolName,
		WorkspaceRelativePath: request.WorkspaceRelativePath,
		ResolvedPath:          request.ResolvedPath,
		ResolvedPathAvailable: request.ResolvedPath != "",
		RequestedRange: ReadLineRange{
			StartLine: requestedOrDefault(request.RequestedStartLine, request.EffectiveStartLine),
			EndLine:   0,
			Limit:     requestedOrDefault(request.RequestedLineLimit, request.EffectiveLineLimit),
		},
		EffectiveRange: ReadLineRange{
			StartLine: request.EffectiveStartLine,
			EndLine:   0,
			Limit:     request.EffectiveLineLimit,
		},
		Truncation: ReadTruncation{PreviewBytesLimit: request.EffectiveMaxPreviewBytes},
		Error:      err,
		Source:     request.Source,
	}
}

func isHomeOrXDGPath(path string) bool {
	slashPath := filepath.ToSlash(path)
	return slashPath == "~" || strings.HasPrefix(slashPath, "~/") || strings.HasPrefix(slashPath, "$HOME") || strings.HasPrefix(slashPath, "${HOME}") || strings.HasPrefix(slashPath, "$XDG_") || strings.HasPrefix(slashPath, "${XDG_")
}

func isDirectoryLikePath(path string) bool {
	return path == "." || path == string(filepath.Separator) || strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(filepath.Separator))
}

func hasTraversal(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isReservedWorkspacePath(path string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(path), "/")
	return first == ".agentera" || first == ".aila"
}

func readError(kind ReadErrorKind, message string) ReadError {
	return ReadError{Kind: kind, Message: boundString(message, maxReadErrorMessageBytes)}
}

func boundPreview(text string, maxBytes int) (string, bool) {
	if len(text) <= maxBytes {
		return text, false
	}
	bounded := text[:maxBytes]
	for !utf8.ValidString(bounded) && len(bounded) > 0 {
		bounded = bounded[:len(bounded)-1]
	}
	return bounded, true
}

func boundString(text string, maxBytes int) string {
	bounded, truncated := boundPreview(text, maxBytes)
	if !truncated {
		return bounded
	}
	return fmt.Sprintf("%s...", strings.TrimRight(bounded, "."))
}

func requestedOrDefault(requested int, effective int) int {
	if requested != 0 {
		return requested
	}
	return effective
}
