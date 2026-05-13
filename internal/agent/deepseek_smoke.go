package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DeepSeekSmokeEnv     = "AILA_LIVE_DEEPSEEK_SMOKE"
	DeepSeekSmokeBaseURL = "https://api.deepseek.com"
	DeepSeekSmokeModel   = "deepseek-v4-flash"

	defaultDeepSeekSmokeTimeout = 10 * time.Second
)

type DeepSeekSmokeStatus string

const (
	DeepSeekSmokeSkipped DeepSeekSmokeStatus = "skipped"
	DeepSeekSmokePassed  DeepSeekSmokeStatus = "passed"
	DeepSeekSmokeFailed  DeepSeekSmokeStatus = "failed"
)

type DeepSeekSmokeOptions struct {
	BaseURL   string
	Model     string
	WorkDir   string
	Timeout   time.Duration
	LookupEnv EnvLookup
	Client    *http.Client
}

type DeepSeekSmokeResult struct {
	Status   DeepSeekSmokeStatus
	Provider string
	Endpoint string
	Model    string
	Reason   string
}

func (result DeepSeekSmokeResult) String() string {
	if result.Reason == "" {
		return fmt.Sprintf("status=%s provider=%s endpoint=%s model=%s", result.Status, result.Provider, result.Endpoint, result.Model)
	}
	return fmt.Sprintf("status=%s provider=%s endpoint=%s model=%s reason=%s", result.Status, result.Provider, result.Endpoint, result.Model, result.Reason)
}

func RunDeepSeekSmoke(ctx context.Context, options DeepSeekSmokeOptions) (DeepSeekSmokeResult, error) {
	baseURL := strings.TrimRight(options.BaseURL, "/")
	if baseURL == "" {
		baseURL = DeepSeekSmokeBaseURL
	}
	model := options.Model
	if model == "" {
		model = DeepSeekSmokeModel
	}
	result := DeepSeekSmokeResult{Status: DeepSeekSmokeSkipped, Provider: "custom", Endpoint: baseURL + "/v1/chat/completions", Model: model}

	lookupEnv := options.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if value, ok := lookupEnv(DeepSeekSmokeEnv); !ok || value != "1" {
		result.Reason = "live smoke disabled"
		return result, nil
	}

	secret, ok, err := lookupOpenAIAPIKey(lookupEnv, options.WorkDir)
	if err != nil {
		result.Reason = "credential lookup failed"
		return result, fmt.Errorf("load %s for DeepSeek smoke: %w", CredentialSourceOpenAIAPIKey, err)
	}
	if !ok || secret.Value() == "" {
		result.Reason = "missing OPENAI_API_KEY"
		return result, nil
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultDeepSeekSmokeTimeout
	}
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 1,
	})
	if err != nil {
		result.Status = DeepSeekSmokeFailed
		result.Reason = "request build failed"
		return result, err
	}

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, result.Endpoint, bytes.NewReader(body))
	if err != nil {
		result.Status = DeepSeekSmokeFailed
		result.Reason = "request build failed"
		return result, fmt.Errorf("build DeepSeek smoke request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+secret.Value())
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		result.Status = DeepSeekSmokeFailed
		result.Reason = ErrReadinessTimeout.Error()
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return result, CredentialFailureError{Kind: ErrReadinessTimeout, Provider: result.Provider, SourceName: CredentialSourceOpenAIAPIKey}
		}
		result.Reason = ErrProviderUnavailable.Error()
		return result, fmt.Errorf("DeepSeek smoke request failed for endpoint %s model %s: %w", result.Endpoint, result.Model, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		result.Status = DeepSeekSmokeFailed
		result.Reason = fmt.Sprintf("http status %d", response.StatusCode)
		return result, fmt.Errorf("DeepSeek smoke failed for endpoint %s model %s: http status %d", result.Endpoint, result.Model, response.StatusCode)
	}

	result.Status = DeepSeekSmokePassed
	result.Reason = "ok"
	return result, nil
}

func lookupOpenAIAPIKey(lookupEnv EnvLookup, workDir string) (Secret, bool, error) {
	if value, ok := lookupEnv(CredentialSourceOpenAIAPIKey); ok && value != "" {
		return NewSecret(value), true, nil
	}

	path := filepath.Join(workDir, ".env")
	value, ok, err := LoadOpenAIAPIKeyFromDotenv(path)
	if err != nil {
		return Secret{}, false, err
	}
	if !ok || value == "" {
		return Secret{}, false, nil
	}
	return NewSecret(value), true, nil
}

func LoadOpenAIAPIKeyFromDotenv(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read .env: %w", err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != CredentialSourceOpenAIAPIKey {
			continue
		}
		value = strings.TrimSpace(stripDotenvComment(value))
		value = strings.Trim(value, `"'`)
		return value, value != "", nil
	}
	return "", false, nil
}

func stripDotenvComment(value string) string {
	inSingleQuote := false
	inDoubleQuote := false
	for index, char := range value {
		switch char {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return value[:index]
			}
		}
	}
	return value
}
