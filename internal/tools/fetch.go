package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	FetchToolName = "fetch"

	DefaultFetchMaxPreviewBytes = 32 * 1024
	DefaultFetchTimeoutMillis   = 10000
)

const maxFetchErrorMessageBytes = 240

// FetchClient is the minimal network boundary used by fetch execution.
type FetchClient interface {
	Do(*http.Request) (*http.Response, error)
}

// FetchSourceMetadata records caller-visible provenance for a fetch request.
type FetchSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// FetchRequest is the caller-facing network read contract before validation.
type FetchRequest struct {
	URL             string
	Method          string
	MaxPreviewBytes int
	TimeoutMillis   int
	Source          FetchSourceMetadata
}

// ValidatedFetchRequest is safe, normalized, and ready for later execution.
type ValidatedFetchRequest struct {
	ToolName                 string
	RequestedURL             string
	EffectiveURL             string
	RequestedMethod          string
	EffectiveMethod          string
	ExpectedEffect           string
	RequestedMaxPreviewBytes int
	RequestedTimeoutMillis   int
	EffectiveMaxPreviewBytes int
	EffectiveTimeoutMillis   int
	Source                   FetchSourceMetadata
}

// FetchTruncation records bounded network body omission decisions.
type FetchTruncation struct {
	PreviewBytesLimit int
	PreviewTruncated  bool
	OmittedBytesKnown bool
	OmittedBytes      int64
	Marker            string
}

// FetchErrorKind is a bounded machine-readable fetch failure category.
type FetchErrorKind string

const (
	FetchErrorNone              FetchErrorKind = "none"
	FetchErrorInvalidURL        FetchErrorKind = "invalid_url"
	FetchErrorUnsupportedScheme FetchErrorKind = "unsupported_scheme"
	FetchErrorInvalidMethod     FetchErrorKind = "invalid_method"
	FetchErrorInvalidRange      FetchErrorKind = "invalid_range"
	FetchErrorPermission        FetchErrorKind = "permission_denied"
	FetchErrorHTTPStatus        FetchErrorKind = "http_status"
	FetchErrorCanceled          FetchErrorKind = "canceled"
	FetchErrorTimeout           FetchErrorKind = "timeout"
	FetchErrorContent           FetchErrorKind = "content_error"
	FetchErrorExecution         FetchErrorKind = "execution_error"
)

// FetchError is safe to surface to callers without leaking host-local paths.
type FetchError struct {
	Kind    FetchErrorKind
	Message string
}

// FetchResult is the deterministic success/failure shape returned by fetch paths.
type FetchResult struct {
	ToolName       string
	RequestedURL   string
	EffectiveURL   string
	Method         string
	ExpectedEffect string
	Status         string
	HTTPStatusCode int
	HTTPStatus     string
	ContentType    string
	PreviewText    string
	Truncation     FetchTruncation
	DurationMillis int64
	Error          FetchError
	Source         FetchSourceMetadata
}

// ValidateFetchRequest applies defaults and rejects non-HTTP(S) or ambiguous
// network reads. It performs no network IO.
func ValidateFetchRequest(request FetchRequest) (ValidatedFetchRequest, FetchError) {
	rawURL := strings.TrimSpace(request.URL)
	if rawURL == "" {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidURL, "url is required")
	}
	if looksLikeShellSyntax(rawURL) || isHomeOrXDGPath(rawURL) {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidURL, "url contains unsupported shell syntax or host-local reference")
	}
	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidURL, "url must be absolute with a host")
	}
	if parsed.User != nil {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidURL, "url credentials are not allowed")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ValidatedFetchRequest{}, fetchError(FetchErrorUnsupportedScheme, "only http and https urls are fetchable")
	}
	method := strings.TrimSpace(request.Method)
	if method == "" {
		method = http.MethodGet
	}
	method = strings.ToUpper(method)
	if method != http.MethodGet && method != http.MethodHead {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidMethod, "fetch only supports GET and HEAD")
	}
	maxPreviewBytes := request.MaxPreviewBytes
	if maxPreviewBytes == 0 {
		maxPreviewBytes = DefaultFetchMaxPreviewBytes
	}
	if maxPreviewBytes < 1 {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidRange, "max preview bytes must be positive")
	}
	timeoutMillis := request.TimeoutMillis
	if timeoutMillis == 0 {
		timeoutMillis = DefaultFetchTimeoutMillis
	}
	if timeoutMillis < 1 {
		return ValidatedFetchRequest{}, fetchError(FetchErrorInvalidRange, "timeout millis must be positive")
	}

	return ValidatedFetchRequest{
		ToolName:                 FetchToolName,
		RequestedURL:             request.URL,
		EffectiveURL:             parsed.String(),
		RequestedMethod:          request.Method,
		EffectiveMethod:          method,
		ExpectedEffect:           "read remote content through bounded fetch",
		RequestedMaxPreviewBytes: request.MaxPreviewBytes,
		RequestedTimeoutMillis:   request.TimeoutMillis,
		EffectiveMaxPreviewBytes: maxPreviewBytes,
		EffectiveTimeoutMillis:   timeoutMillis,
		Source:                   request.Source,
	}, FetchError{}
}

// ExecuteFetch runs a validated network read using http.DefaultClient.
func ExecuteFetch(ctx context.Context, request ValidatedFetchRequest) FetchResult {
	return ExecuteFetchWithClient(ctx, request, http.DefaultClient)
}

// ExecuteFetchWithClient runs a validated network read through an injected
// client so tests can fake all network behavior without live external access.
func ExecuteFetchWithClient(ctx context.Context, request ValidatedFetchRequest, client FetchClient) FetchResult {
	if err := ctx.Err(); err != nil {
		return NewFetchFailure(request, fetchExecutionError(err), "canceled", 0, "")
	}
	if client == nil {
		return NewFetchFailure(request, fetchError(FetchErrorExecution, "fetch client is required"), "failed", 0, "")
	}
	if request.EffectiveURL == "" || request.EffectiveMethod == "" {
		return NewFetchFailure(request, fetchError(FetchErrorInvalidURL, "effective url and method are required"), "invalid", 0, "")
	}
	if request.EffectiveMaxPreviewBytes < 1 || request.EffectiveTimeoutMillis < 1 {
		return NewFetchFailure(request, fetchError(FetchErrorInvalidRange, "effective fetch bounds must be positive"), "invalid", 0, "")
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(request.EffectiveTimeoutMillis)*time.Millisecond)
	defer cancel()
	requestHTTP, err := http.NewRequestWithContext(execCtx, request.EffectiveMethod, request.EffectiveURL, nil)
	if err != nil {
		return NewFetchFailure(request, fetchError(FetchErrorInvalidURL, "effective url is invalid"), "invalid", 0, "")
	}
	requestHTTP.Header.Set("User-Agent", "aila-fetch")
	requestHTTP.Header.Set("Accept", "text/markdown, text/plain, text/html;q=0.8, application/json;q=0.7, */*;q=0.1")

	started := time.Now()
	response, err := client.Do(requestHTTP)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		failure := fetchExecutionError(err)
		status := "failed"
		if failure.Kind == FetchErrorTimeout {
			status = "timeout"
		}
		if failure.Kind == FetchErrorCanceled {
			status = "canceled"
		}
		result := NewFetchFailure(request, failure, status, 0, "")
		result.DurationMillis = duration
		return result
	}
	if response == nil {
		result := NewFetchFailure(request, fetchError(FetchErrorExecution, "fetch response is missing"), "failed", 0, "")
		result.DurationMillis = duration
		return result
	}
	defer func() { _ = response.Body.Close() }()

	contentType := response.Header.Get("Content-Type")
	preview, truncation, readErr := readFetchPreview(execCtx, response.Body, request.EffectiveMaxPreviewBytes, response.ContentLength)
	if readErr.Kind != "" {
		result := NewFetchFailure(request, readErr, statusForFetchError(readErr.Kind), response.StatusCode, contentType)
		result.HTTPStatus = response.Status
		result.DurationMillis = duration
		return result
	}
	result := NewFetchSuccess(request, response.StatusCode, response.Status, contentType, preview, truncation)
	result.DurationMillis = duration
	if response.StatusCode < 200 || response.StatusCode > 299 {
		result.Status = "http_error"
		result.Error = fetchError(FetchErrorHTTPStatus, fmt.Sprintf("remote returned %s", response.Status))
	}
	return result
}

// NewFetchSuccess shapes already-produced response data into the public result contract.
func NewFetchSuccess(request ValidatedFetchRequest, statusCode int, status string, contentType string, previewText string, truncation FetchTruncation) FetchResult {
	preview, previewTruncated := boundPreview(previewText, request.EffectiveMaxPreviewBytes)
	if previewTruncated {
		truncation.PreviewTruncated = true
	}
	truncation.PreviewBytesLimit = request.EffectiveMaxPreviewBytes
	if truncation.PreviewTruncated && truncation.Marker == "" {
		truncation.Marker = "preview_truncated"
	}
	return FetchResult{
		ToolName:       FetchToolName,
		RequestedURL:   request.RequestedURL,
		EffectiveURL:   request.EffectiveURL,
		Method:         request.EffectiveMethod,
		ExpectedEffect: request.ExpectedEffect,
		Status:         "completed",
		HTTPStatusCode: statusCode,
		HTTPStatus:     status,
		ContentType:    contentType,
		PreviewText:    preview,
		Truncation:     truncation,
		Error:          FetchError{Kind: FetchErrorNone},
		Source:         request.Source,
	}
}

// NewFetchFailure shapes validation or execution failures without network IO.
func NewFetchFailure(request ValidatedFetchRequest, err FetchError, status string, statusCode int, contentType string) FetchResult {
	if err.Kind == "" {
		err.Kind = FetchErrorExecution
	}
	if status == "" {
		status = "failed"
	}
	err.Message = boundString(err.Message, maxFetchErrorMessageBytes)
	return FetchResult{
		ToolName:       FetchToolName,
		RequestedURL:   request.RequestedURL,
		EffectiveURL:   request.EffectiveURL,
		Method:         request.EffectiveMethod,
		ExpectedEffect: request.ExpectedEffect,
		Status:         status,
		HTTPStatusCode: statusCode,
		ContentType:    contentType,
		Truncation:     FetchTruncation{PreviewBytesLimit: request.EffectiveMaxPreviewBytes},
		Error:          err,
		Source:         request.Source,
	}
}

func readFetchPreview(ctx context.Context, reader io.Reader, maxPreviewBytes int, contentLength int64) (string, FetchTruncation, FetchError) {
	if reader == nil {
		return "", FetchTruncation{PreviewBytesLimit: maxPreviewBytes}, FetchError{}
	}
	limited := io.LimitReader(reader, int64(maxPreviewBytes)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", FetchTruncation{PreviewBytesLimit: maxPreviewBytes}, fetchExecutionError(err)
	}
	if err := ctx.Err(); err != nil {
		return "", FetchTruncation{PreviewBytesLimit: maxPreviewBytes}, fetchExecutionError(err)
	}
	if !fetchUTF8Valid(body) {
		return "", FetchTruncation{PreviewBytesLimit: maxPreviewBytes}, fetchError(FetchErrorContent, "response body is not valid utf-8 text")
	}
	previewBytes := body
	truncated := len(body) > maxPreviewBytes
	if truncated {
		previewBytes = body[:maxPreviewBytes]
	}
	preview, previewTruncated := boundPreview(string(previewBytes), maxPreviewBytes)
	truncated = truncated || previewTruncated
	truncation := FetchTruncation{PreviewBytesLimit: maxPreviewBytes, PreviewTruncated: truncated}
	if truncated {
		truncation.Marker = "preview_truncated"
		if contentLength >= 0 {
			truncation.OmittedBytesKnown = true
			if omitted := contentLength - int64(len(preview)); omitted > 0 {
				truncation.OmittedBytes = omitted
			}
		}
	}
	return preview, truncation, FetchError{}
}

func statusForFetchError(kind FetchErrorKind) string {
	switch kind {
	case FetchErrorCanceled:
		return "canceled"
	case FetchErrorTimeout:
		return "timeout"
	case FetchErrorInvalidURL, FetchErrorUnsupportedScheme, FetchErrorInvalidMethod, FetchErrorInvalidRange:
		return "invalid"
	default:
		return "failed"
	}
}

func fetchError(kind FetchErrorKind, message string) FetchError {
	return FetchError{Kind: kind, Message: boundString(message, maxFetchErrorMessageBytes)}
}

func fetchExecutionError(err error) FetchError {
	switch {
	case err == nil:
		return FetchError{}
	case errors.Is(err, context.Canceled):
		return fetchError(FetchErrorCanceled, "fetch canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return fetchError(FetchErrorTimeout, "fetch timed out")
	default:
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			if errors.Is(urlErr.Err, context.Canceled) {
				return fetchError(FetchErrorCanceled, "fetch canceled")
			}
			if errors.Is(urlErr.Err, context.DeadlineExceeded) || urlErr.Timeout() {
				return fetchError(FetchErrorTimeout, "fetch timed out")
			}
		}
		return fetchError(FetchErrorExecution, "fetch failed")
	}
}

func fetchUTF8Valid(body []byte) bool {
	for _, b := range body {
		if b == 0 {
			return false
		}
	}
	return strings.ToValidUTF8(string(body), "") == string(body)
}
