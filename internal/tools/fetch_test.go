package tools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type fakeFetchClient struct {
	response *http.Response
	err      error
	seen     *http.Request
}

func (client *fakeFetchClient) Do(request *http.Request) (*http.Response, error) {
	client.seen = request
	return client.response, client.err
}

func TestValidateFetchRequestAllowsOnlyBoundedHTTPReads(t *testing.T) {
	t.Parallel()

	validated, err := ValidateFetchRequest(FetchRequest{URL: " https://example.com/docs?q=1 ", Source: FetchSourceMetadata{Caller: "test", RequestID: "fetch-1"}})
	if err.Kind != "" {
		t.Fatalf("ValidateFetchRequest error = %#v", err)
	}
	if validated.ToolName != FetchToolName || validated.EffectiveURL != "https://example.com/docs?q=1" || validated.EffectiveMethod != http.MethodGet || validated.EffectiveMaxPreviewBytes != DefaultFetchMaxPreviewBytes || validated.EffectiveTimeoutMillis != DefaultFetchTimeoutMillis || validated.ExpectedEffect == "" {
		t.Fatalf("validated fetch = %#v", validated)
	}
	if validated.Source.RequestID != "fetch-1" {
		t.Fatalf("source metadata = %#v", validated.Source)
	}
}

func TestValidateFetchRequestRejectsUnsafeOrAmbiguousURLs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		req  FetchRequest
		kind FetchErrorKind
	}{
		{name: "empty", req: FetchRequest{}, kind: FetchErrorInvalidURL},
		{name: "relative", req: FetchRequest{URL: "docs/page"}, kind: FetchErrorInvalidURL},
		{name: "file", req: FetchRequest{URL: "file:///etc/passwd"}, kind: FetchErrorInvalidURL},
		{name: "ftp", req: FetchRequest{URL: "ftp://example.com/file"}, kind: FetchErrorUnsupportedScheme},
		{name: "credentials", req: FetchRequest{URL: "https://u:p@example.com/"}, kind: FetchErrorInvalidURL},
		{name: "shell syntax", req: FetchRequest{URL: "https://example.com/a|wc"}, kind: FetchErrorInvalidURL},
		{name: "method", req: FetchRequest{URL: "https://example.com/", Method: "POST"}, kind: FetchErrorInvalidMethod},
		{name: "range", req: FetchRequest{URL: "https://example.com/", MaxPreviewBytes: -1}, kind: FetchErrorInvalidRange},
		{name: "timeout", req: FetchRequest{URL: "https://example.com/", TimeoutMillis: -1}, kind: FetchErrorInvalidRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateFetchRequest(tc.req)
			if err.Kind != tc.kind {
				t.Fatalf("error kind = %q, want %q (%#v)", err.Kind, tc.kind, err)
			}
		})
	}
}

func TestExecuteFetchWithClientReturnsBoundedSuccess(t *testing.T) {
	t.Parallel()

	validated, err := ValidateFetchRequest(FetchRequest{URL: "https://example.com/docs", MaxPreviewBytes: 12})
	if err.Kind != "" {
		t.Fatalf("ValidateFetchRequest error = %#v", err)
	}
	client := &fakeFetchClient{response: &http.Response{
		StatusCode:    200,
		Status:        "200 OK",
		Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader("hello network boundary")),
		ContentLength: int64(len("hello network boundary")),
	}}

	result := ExecuteFetchWithClient(context.Background(), validated, client)

	if result.Error.Kind != FetchErrorNone || result.Status != "completed" || result.HTTPStatusCode != 200 || result.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("fetch result = %#v", result)
	}
	if result.PreviewText != "hello networ" || !result.Truncation.PreviewTruncated || result.Truncation.Marker != "preview_truncated" || result.Truncation.OmittedBytes <= 0 {
		t.Fatalf("preview/truncation = %q %#v", result.PreviewText, result.Truncation)
	}
	if client.seen == nil || client.seen.Method != http.MethodGet || client.seen.URL.String() != "https://example.com/docs" || client.seen.Header.Get("User-Agent") != "aila-fetch" {
		t.Fatalf("request sent = %#v", client.seen)
	}
}

func TestExecuteFetchWithClientSurfacesHTTPAndNetworkErrors(t *testing.T) {
	t.Parallel()

	validated, err := ValidateFetchRequest(FetchRequest{URL: "https://example.com/missing", MaxPreviewBytes: 64})
	if err.Kind != "" {
		t.Fatalf("ValidateFetchRequest error = %#v", err)
	}
	httpClient := &fakeFetchClient{response: &http.Response{
		StatusCode:    404,
		Status:        "404 Not Found",
		Header:        http.Header{"Content-Type": []string{"text/plain"}},
		Body:          io.NopCloser(strings.NewReader("not here")),
		ContentLength: int64(len("not here")),
	}}
	statusResult := ExecuteFetchWithClient(context.Background(), validated, httpClient)
	if statusResult.Status != "http_error" || statusResult.Error.Kind != FetchErrorHTTPStatus || statusResult.PreviewText != "not here" || statusResult.HTTPStatusCode != 404 {
		t.Fatalf("http status result = %#v", statusResult)
	}

	networkResult := ExecuteFetchWithClient(context.Background(), validated, &fakeFetchClient{err: errors.New("dial boom")})
	if networkResult.Status != "failed" || networkResult.Error.Kind != FetchErrorExecution || strings.Contains(networkResult.Error.Message, "dial boom") {
		t.Fatalf("network result = %#v", networkResult)
	}
}

func TestExecuteFetchWithClientHandlesCancellationAndContentFailure(t *testing.T) {
	t.Parallel()

	validated, err := ValidateFetchRequest(FetchRequest{URL: "https://example.com/bin"})
	if err.Kind != "" {
		t.Fatalf("ValidateFetchRequest error = %#v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := ExecuteFetchWithClient(ctx, validated, &fakeFetchClient{})
	if canceled.Status != "canceled" || canceled.Error.Kind != FetchErrorCanceled {
		t.Fatalf("canceled result = %#v", canceled)
	}

	binary := ExecuteFetchWithClient(context.Background(), validated, &fakeFetchClient{response: &http.Response{
		StatusCode: 200,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader("ok\x00no")),
	}})
	if binary.Status != "failed" || binary.Error.Kind != FetchErrorContent || binary.PreviewText != "" {
		t.Fatalf("binary result = %#v", binary)
	}
}

func TestFetchContractDoesNotDependOnFutureToolSurfaces(t *testing.T) {
	t.Parallel()

	contentsBytes, readErr := os.ReadFile("fetch.go")
	if readErr != nil {
		t.Fatal(readErr)
	}
	contents := string(contentsBytes)
	for _, forbidden := range []string{"Provider", "OpenAI", "go-agent", "EditTool", "WriteTool", "workflow.", "mcp", "plugin"} {
		if strings.Contains(contents, forbidden) {
			t.Fatalf("fetch.go contains future/provider surface %q", forbidden)
		}
	}
}
