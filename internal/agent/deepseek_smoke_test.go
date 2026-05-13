package agent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunDeepSeekSmokeSkipsByDefaultWithoutNetwork(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("default DeepSeek smoke used network")
		return nil, nil
	})
	result, err := RunDeepSeekSmoke(context.Background(), DeepSeekSmokeOptions{
		LookupEnv: func(string) (string, bool) { return "", false },
		Client:    &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("run smoke: %v", err)
	}
	if result.Status != DeepSeekSmokeSkipped || result.Reason != "live smoke disabled" {
		t.Fatalf("result = %+v, want disabled skip", result)
	}
}

func TestRunDeepSeekSmokeSkipsMissingCredentialSafely(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("missing-key DeepSeek smoke used network")
		return nil, nil
	})
	result, err := RunDeepSeekSmoke(context.Background(), DeepSeekSmokeOptions{
		WorkDir: workspace,
		LookupEnv: func(name string) (string, bool) {
			return map[string]string{DeepSeekSmokeEnv: "1"}[name], name == DeepSeekSmokeEnv
		},
		Client: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("run smoke: %v", err)
	}
	if result.Status != DeepSeekSmokeSkipped || result.Reason != "missing OPENAI_API_KEY" {
		t.Fatalf("result = %+v, want missing-key skip", result)
	}
}

func TestLoadOpenAIAPIKeyFromDotenvReadsOnlyOpenAIKeyAndRedacts(t *testing.T) {
	t.Parallel()

	secret := "deepseek-secret-value"
	path := filepath.Join(t.TempDir(), ".env")
	content := "# ignored\nOPENCODE_API_KEY=other-secret\nexport OPENAI_API_KEY='" + secret + "' # comment\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write .env fixture: %v", err)
	}

	value, ok, err := LoadOpenAIAPIKeyFromDotenv(path)
	if err != nil {
		t.Fatalf("load .env: %v", err)
	}
	if !ok || value != secret {
		t.Fatalf("loaded key = %q, %v; want approved OPENAI_API_KEY", value, ok)
	}
	redacted := NewSecret(value).String()
	if strings.Contains(redacted, secret) || redacted != "<redacted>" {
		t.Fatalf("secret string = %q, want redacted", redacted)
	}

	result := DeepSeekSmokeResult{Status: DeepSeekSmokePassed, Provider: "custom", Endpoint: DeepSeekSmokeBaseURL + "/v1/chat/completions", Model: DeepSeekSmokeModel, Reason: "ok"}
	if strings.Contains(result.String(), secret) || strings.Contains(result.String(), "other-secret") {
		t.Fatalf("smoke result leaked secret: %s", result)
	}
}

func TestRunDeepSeekSmokeUsesInjectedClientAndDoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	secret := "deepseek-secret-value"
	var sawAuthorization bool
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != DeepSeekSmokeBaseURL+"/v1/chat/completions" {
			t.Fatalf("endpoint = %s", request.URL.String())
		}
		if request.Header.Get("Authorization") == "Bearer "+secret {
			sawAuthorization = true
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), DeepSeekSmokeModel) {
			t.Fatalf("request body %s does not include model %s", body, DeepSeekSmokeModel)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"smoke"}`))}, nil
	})

	result, err := RunDeepSeekSmoke(context.Background(), DeepSeekSmokeOptions{
		LookupEnv: func(name string) (string, bool) {
			env := map[string]string{DeepSeekSmokeEnv: "1", CredentialSourceOpenAIAPIKey: secret}
			value, ok := env[name]
			return value, ok
		},
		Client: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("run smoke: %v", err)
	}
	if result.Status != DeepSeekSmokePassed || !sawAuthorization {
		t.Fatalf("result = %+v saw auth=%v, want pass with authorization", result, sawAuthorization)
	}
	if strings.Contains(result.String(), secret) || strings.Contains(errText(err), secret) {
		t.Fatalf("smoke output leaked secret: result=%s err=%v", result, err)
	}
}

func TestRunDeepSeekSmokeTimeoutUsesTypedRedactedError(t *testing.T) {
	t.Parallel()

	secret := "deepseek-secret-value"
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	result, err := RunDeepSeekSmoke(context.Background(), DeepSeekSmokeOptions{
		Timeout: 10 * time.Millisecond,
		LookupEnv: func(name string) (string, bool) {
			env := map[string]string{DeepSeekSmokeEnv: "1", CredentialSourceOpenAIAPIKey: secret}
			value, ok := env[name]
			return value, ok
		},
		Client: &http.Client{Transport: transport},
	})
	if !errors.Is(err, ErrReadinessTimeout) {
		t.Fatalf("error = %T %[1]v, want ErrReadinessTimeout", err)
	}
	if result.Status != DeepSeekSmokeFailed || result.Reason != ErrReadinessTimeout.Error() {
		t.Fatalf("result = %+v, want timeout failure", result)
	}
	if strings.Contains(result.String(), secret) || strings.Contains(err.Error(), secret) {
		t.Fatalf("timeout output leaked secret: result=%s err=%v", result, err)
	}
}

func TestLiveDeepSeekSmoke(t *testing.T) {
	if os.Getenv(DeepSeekSmokeEnv) != "1" {
		t.Skip("set AILA_LIVE_DEEPSEEK_SMOKE=1 to run approved live DeepSeek smoke")
	}

	result, err := RunDeepSeekSmoke(context.Background(), DeepSeekSmokeOptions{WorkDir: filepath.Join("..", "..")})
	if result.Status == DeepSeekSmokeSkipped {
		t.Skipf("DeepSeek smoke skipped: %s", result)
	}
	if err != nil {
		t.Fatalf("DeepSeek smoke failed: %s: %v", result, err)
	}
	if result.Status != DeepSeekSmokePassed {
		t.Fatalf("DeepSeek smoke result = %s, want passed", result)
	}
	t.Logf("DeepSeek smoke passed: endpoint=%s model=%s", result.Endpoint, result.Model)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
