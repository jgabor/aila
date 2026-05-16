package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	FindToolName = "find"
	GrepToolName = "grep"

	DefaultSearchMaxResults      = 50
	DefaultSearchMaxPreviewBytes = 240
)

const (
	maxSearchErrorMessageBytes = 240
	maxSearchScanBytes         = 1 * 1024 * 1024
	maxSearchLineBytes         = 16 * 1024
)

// SearchSourceMetadata records caller-visible provenance for search requests.
type SearchSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// FindRequest is the caller-facing file discovery contract before validation.
type FindRequest struct {
	Pattern         string
	MaxResults      int
	MaxPreviewBytes int
	Source          SearchSourceMetadata
}

// GrepRequest is the caller-facing content search contract before validation.
type GrepRequest struct {
	Query           string
	Regex           bool
	IncludePattern  string
	MaxResults      int
	MaxPreviewBytes int
	Source          SearchSourceMetadata
}

// ValidatedFindRequest is safe, normalized, and ready for later execution.
type ValidatedFindRequest struct {
	ToolName                 string
	RequestedPattern         string
	WorkspaceRoot            string
	EffectivePattern         string
	RequestedMaxResults      int
	RequestedMaxPreviewBytes int
	EffectiveMaxResults      int
	EffectiveMaxPreviewBytes int
	Source                   SearchSourceMetadata
}

// ValidatedGrepRequest is safe, normalized, and ready for later execution.
type ValidatedGrepRequest struct {
	ToolName                 string
	RequestedQuery           string
	RequestedIncludePattern  string
	WorkspaceRoot            string
	EffectiveQuery           string
	Regex                    bool
	EffectiveIncludePattern  string
	RequestedMaxResults      int
	RequestedMaxPreviewBytes int
	EffectiveMaxResults      int
	EffectiveMaxPreviewBytes int
	Source                   SearchSourceMetadata
	compiled                 *regexp.Regexp
}

// SearchMatch records one path or content match.
type SearchMatch struct {
	Path        string
	LineNumber  int
	PreviewText string
}

// SearchTruncation records bounded search truncation and omission decisions.
type SearchTruncation struct {
	MaxResults        int
	MaxPreviewBytes   int
	OmittedResults    int
	OmittedFiles      int
	PreviewTruncated  bool
	ResultLimitHit    bool
	FileSkipCount     int
	TruncationMarkers string
}

// SearchErrorKind is a bounded machine-readable search failure category.
type SearchErrorKind string

const (
	SearchErrorNone             SearchErrorKind = "none"
	SearchErrorInvalidPath      SearchErrorKind = "invalid_path"
	SearchErrorOutsideWorkspace SearchErrorKind = "outside_workspace"
	SearchErrorReservedPath     SearchErrorKind = "reserved_path"
	SearchErrorInvalidPattern   SearchErrorKind = "invalid_pattern"
	SearchErrorInvalidQuery     SearchErrorKind = "invalid_query"
	SearchErrorInvalidRange     SearchErrorKind = "invalid_range"
	SearchErrorPermission       SearchErrorKind = "permission_denied"
	SearchErrorSymlinkEscape    SearchErrorKind = "symlink_escape"
	SearchErrorCanceled         SearchErrorKind = "canceled"
	SearchErrorExecution        SearchErrorKind = "execution_error"
)

// SearchError is safe to surface to callers without leaking host-local paths.
type SearchError struct {
	Kind    SearchErrorKind
	Message string
}

// SearchResult is the deterministic success/failure shape returned by find and grep.
type SearchResult struct {
	ToolName              string
	Pattern               string
	Query                 string
	Regex                 bool
	IncludePattern        string
	Matches               []SearchMatch
	Truncation            SearchTruncation
	Error                 SearchError
	Source                SearchSourceMetadata
	WorkspaceRoot         string
	WorkspaceRootResolved bool
}

// ValidateFindRequest applies defaults and rejects unsafe workspace discovery patterns.
func ValidateFindRequest(workspaceRoot string, request FindRequest) (ValidatedFindRequest, SearchError) {
	root, err := validateSearchRoot(workspaceRoot)
	if err.Kind != "" {
		return ValidatedFindRequest{}, err
	}
	pattern, err := validateSearchPattern(request.Pattern, true)
	if err.Kind != "" {
		return ValidatedFindRequest{}, err
	}
	maxResults, maxPreviewBytes, err := validateSearchBounds(request.MaxResults, request.MaxPreviewBytes)
	if err.Kind != "" {
		return ValidatedFindRequest{}, err
	}

	return ValidatedFindRequest{
		ToolName:                 FindToolName,
		RequestedPattern:         request.Pattern,
		WorkspaceRoot:            root,
		EffectivePattern:         pattern,
		RequestedMaxResults:      request.MaxResults,
		RequestedMaxPreviewBytes: request.MaxPreviewBytes,
		EffectiveMaxResults:      maxResults,
		EffectiveMaxPreviewBytes: maxPreviewBytes,
		Source:                   request.Source,
	}, SearchError{}
}

// ValidateGrepRequest applies defaults and rejects unsafe query/include contracts.
func ValidateGrepRequest(workspaceRoot string, request GrepRequest) (ValidatedGrepRequest, SearchError) {
	root, err := validateSearchRoot(workspaceRoot)
	if err.Kind != "" {
		return ValidatedGrepRequest{}, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return ValidatedGrepRequest{}, searchError(SearchErrorInvalidQuery, "query is required")
	}
	var compiled *regexp.Regexp
	if request.Regex {
		var compileErr error
		compiled, compileErr = regexp.Compile(query)
		if compileErr != nil {
			return ValidatedGrepRequest{}, searchError(SearchErrorInvalidQuery, "regex query is invalid")
		}
	}
	includePattern := strings.TrimSpace(request.IncludePattern)
	if includePattern != "" {
		var includeErr SearchError
		includePattern, includeErr = validateSearchPattern(includePattern, false)
		if includeErr.Kind != "" {
			return ValidatedGrepRequest{}, includeErr
		}
	}
	maxResults, maxPreviewBytes, err := validateSearchBounds(request.MaxResults, request.MaxPreviewBytes)
	if err.Kind != "" {
		return ValidatedGrepRequest{}, err
	}

	return ValidatedGrepRequest{
		ToolName:                 GrepToolName,
		RequestedQuery:           request.Query,
		RequestedIncludePattern:  request.IncludePattern,
		WorkspaceRoot:            root,
		EffectiveQuery:           query,
		Regex:                    request.Regex,
		EffectiveIncludePattern:  includePattern,
		RequestedMaxResults:      request.MaxResults,
		RequestedMaxPreviewBytes: request.MaxPreviewBytes,
		EffectiveMaxResults:      maxResults,
		EffectiveMaxPreviewBytes: maxPreviewBytes,
		Source:                   request.Source,
		compiled:                 compiled,
	}, SearchError{}
}

// ExecuteFind walks the workspace and returns bounded deterministic file matches.
func ExecuteFind(ctx context.Context, request ValidatedFindRequest) SearchResult {
	if err := ctx.Err(); err != nil {
		return NewSearchFailure(FindToolName, request.EffectivePattern, "", false, "", request.WorkspaceRoot, request.Source, searchExecutionError(err))
	}
	root, err := resolveSearchRoot(request.WorkspaceRoot)
	if err.Kind != "" {
		return NewSearchFailure(FindToolName, request.EffectivePattern, "", false, "", request.WorkspaceRoot, request.Source, err)
	}

	matches := make([]SearchMatch, 0, request.EffectiveMaxResults)
	omitted := 0
	skipped := 0
	walkErr := filepath.WalkDir(root, func(file string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if walkErr != nil {
			skipped++
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if file == root {
			return nil
		}
		rel, relErr := workspaceRelative(root, file)
		if relErr.Kind != "" {
			skipped++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if isReservedSearchPath(rel) || entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !matchesPattern(request.EffectivePattern, rel) {
			return nil
		}
		if len(matches) >= request.EffectiveMaxResults {
			omitted++
			return nil
		}
		matches = append(matches, SearchMatch{Path: rel})
		return nil
	})
	if walkErr != nil {
		return NewSearchFailure(FindToolName, request.EffectivePattern, "", false, "", request.WorkspaceRoot, request.Source, searchExecutionError(walkErr))
	}
	sortSearchMatches(matches)
	return NewSearchSuccess(FindToolName, request.EffectivePattern, "", false, "", root, request.Source, matches, request.EffectiveMaxResults, request.EffectiveMaxPreviewBytes, omitted, skipped)
}

// ExecuteGrep scans workspace text files and returns bounded deterministic content matches.
func ExecuteGrep(ctx context.Context, request ValidatedGrepRequest) SearchResult {
	if err := ctx.Err(); err != nil {
		return NewSearchFailure(GrepToolName, "", request.EffectiveQuery, request.Regex, request.EffectiveIncludePattern, request.WorkspaceRoot, request.Source, searchExecutionError(err))
	}
	root, err := resolveSearchRoot(request.WorkspaceRoot)
	if err.Kind != "" {
		return NewSearchFailure(GrepToolName, "", request.EffectiveQuery, request.Regex, request.EffectiveIncludePattern, request.WorkspaceRoot, request.Source, err)
	}

	matches := make([]SearchMatch, 0, request.EffectiveMaxResults)
	omitted := 0
	skipped := 0
	walkErr := filepath.WalkDir(root, func(file string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if walkErr != nil {
			skipped++
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if file == root {
			return nil
		}
		rel, relErr := workspaceRelative(root, file)
		if relErr.Kind != "" {
			skipped++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if isReservedSearchPath(rel) || entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if request.EffectiveIncludePattern != "" && !matchesPattern(request.EffectiveIncludePattern, rel) {
			return nil
		}
		fileMatches, fileOmitted, fileSkipped := grepFile(ctx, file, rel, request, len(matches))
		skipped += fileSkipped
		omitted += fileOmitted
		matches = append(matches, fileMatches...)
		return nil
	})
	if walkErr != nil {
		return NewSearchFailure(GrepToolName, "", request.EffectiveQuery, request.Regex, request.EffectiveIncludePattern, request.WorkspaceRoot, request.Source, searchExecutionError(walkErr))
	}
	sortSearchMatches(matches)
	return NewSearchSuccess(GrepToolName, "", request.EffectiveQuery, request.Regex, request.EffectiveIncludePattern, root, request.Source, matches, request.EffectiveMaxResults, request.EffectiveMaxPreviewBytes, omitted, skipped)
}

// NewSearchSuccess shapes already-produced matches into the public search result contract.
func NewSearchSuccess(toolName string, pattern string, query string, regex bool, includePattern string, workspaceRoot string, source SearchSourceMetadata, matches []SearchMatch, maxResults int, maxPreviewBytes int, omittedResults int, omittedFiles int) SearchResult {
	boundedMatches := make([]SearchMatch, 0, len(matches))
	previewTruncated := false
	for _, match := range matches {
		bounded := match
		bounded.PreviewText, previewTruncated = boundSearchPreview(bounded.PreviewText, maxPreviewBytes, previewTruncated)
		boundedMatches = append(boundedMatches, bounded)
	}
	markers := searchMarkers(previewTruncated, omittedResults > 0, omittedFiles > 0)
	return SearchResult{
		ToolName:       toolName,
		Pattern:        pattern,
		Query:          query,
		Regex:          regex,
		IncludePattern: includePattern,
		Matches:        boundedMatches,
		Truncation: SearchTruncation{
			MaxResults:        maxResults,
			MaxPreviewBytes:   maxPreviewBytes,
			OmittedResults:    omittedResults,
			OmittedFiles:      omittedFiles,
			PreviewTruncated:  previewTruncated,
			ResultLimitHit:    omittedResults > 0,
			FileSkipCount:     omittedFiles,
			TruncationMarkers: strings.Join(markers, ","),
		},
		Error:                 SearchError{Kind: SearchErrorNone},
		Source:                source,
		WorkspaceRoot:         workspaceRoot,
		WorkspaceRootResolved: workspaceRoot != "",
	}
}

// NewSearchFailure shapes validation or execution failures without requiring file IO.
func NewSearchFailure(toolName string, pattern string, query string, regex bool, includePattern string, workspaceRoot string, source SearchSourceMetadata, err SearchError) SearchResult {
	if err.Kind == "" {
		err.Kind = SearchErrorExecution
	}
	err.Message = boundString(err.Message, maxSearchErrorMessageBytes)
	return SearchResult{
		ToolName:       toolName,
		Pattern:        pattern,
		Query:          query,
		Regex:          regex,
		IncludePattern: includePattern,
		Truncation: SearchTruncation{
			MaxResults:      DefaultSearchMaxResults,
			MaxPreviewBytes: DefaultSearchMaxPreviewBytes,
		},
		Error:                 err,
		Source:                source,
		WorkspaceRoot:         workspaceRoot,
		WorkspaceRootResolved: workspaceRoot != "",
	}
}

func validateSearchRoot(workspaceRoot string) (string, SearchError) {
	root := filepath.Clean(workspaceRoot)
	if root == "." || !filepath.IsAbs(root) {
		return "", searchError(SearchErrorInvalidPath, "workspace root must be absolute")
	}
	return root, SearchError{}
}

func validateSearchPattern(pattern string, required bool) (string, SearchError) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		if required {
			return "", searchError(SearchErrorInvalidPattern, "pattern is required")
		}
		return "", SearchError{}
	}
	if isHomeOrXDGPath(pattern) || isReservedWorkspacePath(filepath.ToSlash(pattern)) {
		return "", searchError(SearchErrorReservedPath, "reserved workspace paths are not searchable")
	}
	if filepath.IsAbs(pattern) {
		return "", searchError(SearchErrorOutsideWorkspace, "pattern must stay inside the workspace")
	}
	if hasTraversal(pattern) {
		return "", searchError(SearchErrorInvalidPath, "path traversal is not allowed")
	}
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	if pattern == "." || pattern == "/" {
		return "", searchError(SearchErrorInvalidPattern, "pattern must target files")
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return "", searchError(SearchErrorInvalidPattern, "glob pattern is invalid")
	}
	return pattern, SearchError{}
}

func validateSearchBounds(maxResults int, maxPreviewBytes int) (int, int, SearchError) {
	if maxResults == 0 {
		maxResults = DefaultSearchMaxResults
	}
	if maxResults < 1 {
		return 0, 0, searchError(SearchErrorInvalidRange, "max results must be positive")
	}
	if maxPreviewBytes == 0 {
		maxPreviewBytes = DefaultSearchMaxPreviewBytes
	}
	if maxPreviewBytes < 1 {
		return 0, 0, searchError(SearchErrorInvalidRange, "max preview bytes must be positive")
	}
	return maxResults, maxPreviewBytes, SearchError{}
}

func searchError(kind SearchErrorKind, message string) SearchError {
	return SearchError{Kind: kind, Message: boundString(message, maxSearchErrorMessageBytes)}
}

func searchExecutionError(err error) SearchError {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return searchError(SearchErrorCanceled, "search canceled")
	case errors.Is(err, os.ErrPermission):
		return searchError(SearchErrorPermission, "path is not searchable")
	default:
		return searchError(SearchErrorExecution, "search failed")
	}
}

func resolveSearchRoot(root string) (string, SearchError) {
	root = filepath.Clean(root)
	if root == "." || !filepath.IsAbs(root) {
		return "", searchError(SearchErrorInvalidPath, "workspace root must be absolute")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", searchExecutionError(err)
	}
	return realRoot, SearchError{}
}

func workspaceRelative(root string, file string) (string, SearchError) {
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", searchError(SearchErrorSymlinkEscape, "resolved path escapes workspace")
	}
	return filepath.ToSlash(rel), SearchError{}
}

func isReservedSearchPath(rel string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return first == ".agentera" || first == ".aila" || first == ".git"
}

func matchesPattern(pattern string, rel string) bool {
	rel = filepath.ToSlash(rel)
	base := path.Base(rel)
	for _, candidate := range patternCandidates(pattern) {
		if ok, _ := path.Match(candidate, rel); ok {
			return true
		}
		if ok, _ := path.Match(candidate, base); ok {
			return true
		}
	}
	return pattern == rel || pattern == base
}

func patternCandidates(pattern string) []string {
	patterns := []string{pattern}
	if strings.HasPrefix(pattern, "**/") {
		patterns = append(patterns, strings.TrimPrefix(pattern, "**/"))
	}
	if strings.Contains(pattern, "/**/") {
		patterns = append(patterns, strings.ReplaceAll(pattern, "/**/", "/"))
	}
	return patterns
}

func grepFile(ctx context.Context, file string, rel string, request ValidatedGrepRequest, existingMatches int) ([]SearchMatch, int, int) {
	opened, err := os.Open(file)
	if err != nil {
		return nil, 0, 1
	}
	defer func() { _ = opened.Close() }()

	reader := bufio.NewReader(opened)
	matches := make([]SearchMatch, 0)
	omitted := 0
	lineNumber := 1
	scannedBytes := 0
	for {
		if ctx.Err() != nil {
			return matches, omitted, 1
		}
		line, readErr := reader.ReadSlice('\n')
		scannedBytes += len(line)
		if len(line) > maxSearchLineBytes || scannedBytes > maxSearchScanBytes || bytes.IndexByte(line, 0) >= 0 || !utf8.Valid(line) {
			return matches, omitted, 1
		}
		if len(line) > 0 && grepLineMatches(line, request) {
			if existingMatches+len(matches) >= request.EffectiveMaxResults {
				omitted++
			} else {
				matches = append(matches, SearchMatch{Path: rel, LineNumber: lineNumber, PreviewText: strings.TrimRight(string(line), "\r\n")})
			}
		}
		if errors.Is(readErr, bufio.ErrBufferFull) {
			continue
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return matches, omitted, 1
		}
		lineNumber++
	}
	return matches, omitted, 0
}

func grepLineMatches(line []byte, request ValidatedGrepRequest) bool {
	if request.Regex {
		return request.compiled.Match(line)
	}
	return strings.Contains(string(line), request.EffectiveQuery)
}

func sortSearchMatches(matches []SearchMatch) {
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].LineNumber < matches[j].LineNumber
	})
}

func boundSearchPreview(text string, maxBytes int, previous bool) (string, bool) {
	bounded, truncated := boundPreview(text, maxBytes)
	return bounded, previous || truncated
}

func searchMarkers(previewTruncated bool, resultLimitHit bool, filesOmitted bool) []string {
	var markers []string
	if previewTruncated {
		markers = append(markers, "preview_truncated")
	}
	if resultLimitHit {
		markers = append(markers, "result_limit_hit")
	}
	if filesOmitted {
		markers = append(markers, "files_omitted")
	}
	return markers
}
