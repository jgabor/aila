package agent

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClassifyFakeReadinessProviderFamiliesAndCredentialSourceNames(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		request     ReadinessRequest
		wantFamily  ProviderFamily
		wantSources []string
		wantCheck   bool
	}{
		{name: "openai", request: ReadinessRequest{Provider: "openai", Model: "gpt-4.1"}, wantFamily: ProviderFamilyAPIKey, wantSources: []string{CredentialSourceOpenAIAPIKey}, wantCheck: true},
		{name: "opencode-zen", request: ReadinessRequest{Provider: "opencode-zen", Model: "zen-pro"}, wantFamily: ProviderFamilyAPIKey, wantSources: []string{CredentialSourceOpenCodeAPIKey}, wantCheck: true},
		{name: "custom", request: ReadinessRequest{Provider: "custom", Model: "deepseek-chat", BaseURL: "https://api.deepseek.com"}, wantFamily: ProviderFamilyCustom, wantSources: []string{CredentialSourceOpenAIAPIKey}, wantCheck: true},
		{name: "codex", request: ReadinessRequest{Provider: "codex", Model: "codex-high"}, wantFamily: ProviderFamilyDeviceCode, wantSources: []string{CredentialSourceDeviceCode}},
		{name: "copilot", request: ReadinessRequest{Provider: "copilot", Model: "copilot-chat"}, wantFamily: ProviderFamilyDeviceCode, wantSources: []string{CredentialSourceDeviceCode}},
		{name: "opencode-go", request: ReadinessRequest{Provider: "opencode-go", Model: "deepseek-v4-pro", Reasoning: "high"}, wantFamily: ProviderFamilyDeviceCode, wantSources: []string{CredentialSourceDeviceCode}},
		{name: "xiaomi-plan", request: ReadinessRequest{Provider: "xiaomi-plan", Model: "mi-pro"}, wantFamily: ProviderFamilyDeviceCode, wantSources: []string{CredentialSourceDeviceCode}},
		{name: "zai-plan", request: ReadinessRequest{Provider: "zai-plan", Model: "glm-4.5"}, wantFamily: ProviderFamilyDeviceCode, wantSources: []string{CredentialSourceDeviceCode}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ClassifyFakeReadiness(tc.request)
			if err != nil {
				t.Fatalf("classify fake readiness: %v", err)
			}
			if got.Provider != tc.request.Provider || got.Family != tc.wantFamily || got.Model != tc.request.Model || got.Reasoning != tc.request.Reasoning {
				t.Fatalf("readiness = %+v, want provider=%q family=%q model=%q reasoning=%q", got, tc.request.Provider, tc.wantFamily, tc.request.Model, tc.request.Reasoning)
			}
			if !reflect.DeepEqual(got.CredentialSourceNames, tc.wantSources) {
				t.Fatalf("credential sources = %#v, want %#v", got.CredentialSourceNames, tc.wantSources)
			}
			if !got.AvailableBeforeTurn || got.RequiresCredentialCheck != tc.wantCheck {
				t.Fatalf("readiness flags = available_before_turn:%v requires_credential_check:%v, want available true check %v", got.AvailableBeforeTurn, got.RequiresCredentialCheck, tc.wantCheck)
			}
		})
	}
}

func TestClassifyFakeReadinessMetadataIsDeterministicAndOrdered(t *testing.T) {
	t.Parallel()

	request := ReadinessRequest{Provider: "custom", Model: "deepseek-reasoner", Reasoning: "high", BaseURL: "https://api.deepseek.com"}
	first, err := ClassifyFakeReadiness(request)
	if err != nil {
		t.Fatalf("first classify: %v", err)
	}
	second, err := ClassifyFakeReadiness(request)
	if err != nil {
		t.Fatalf("second classify: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("classifications differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}

	want := []MetadataEntry{
		{Name: "provider", Value: "custom"},
		{Name: "family", Value: "custom"},
		{Name: "credential_source", Value: CredentialSourceOpenAIAPIKey},
		{Name: "model", Value: "deepseek-reasoner"},
		{Name: "model_class", Value: "reasoning"},
		{Name: "reasoning", Value: "high"},
		{Name: "base_url_source", Value: "llm.base_url"},
		{Name: "base_url", Value: "https://api.deepseek.com"},
	}
	if !reflect.DeepEqual(first.Metadata, want) {
		t.Fatalf("metadata = %#v, want %#v", first.Metadata, want)
	}
}

func TestClassifyFakeReadinessUnsupportedProviderTypedError(t *testing.T) {
	t.Parallel()

	_, err := ClassifyFakeReadiness(ReadinessRequest{Provider: "anthropic", Model: "claude"})
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("error = %v, want ErrUnsupportedProvider", err)
	}
	var typed UnsupportedProviderError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T %[1]v, want UnsupportedProviderError", err)
	}
	if typed.Provider != "anthropic" {
		t.Fatalf("typed provider = %q, want anthropic", typed.Provider)
	}
	if strings.Contains(err.Error(), "claude") {
		t.Fatalf("unsupported provider error leaked model detail: %v", err)
	}
}

func TestClassifyFakeReadinessUnavailableModelTypedError(t *testing.T) {
	t.Parallel()

	_, err := ClassifyFakeReadiness(ReadinessRequest{Provider: "openai", Model: "not-real"})
	if !errors.Is(err, ErrUnavailableModel) {
		t.Fatalf("error = %v, want ErrUnavailableModel", err)
	}
	var typed UnavailableModelError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T %[1]v, want UnavailableModelError", err)
	}
	if typed.Provider != "openai" || typed.Model != "not-real" {
		t.Fatalf("typed error = %+v, want provider openai model not-real", typed)
	}
}

func TestClassifyFakeReadinessDoesNotReadSecretsOrUseNetworkState(t *testing.T) {
	workspace := t.TempDir()
	secretPath := filepath.Join(workspace, ".env")
	secretContent := "OPENAI_API_KEY=secret-from-file\nOPENCODE_API_KEY=other-secret\n"
	if err := os.WriteFile(secretPath, []byte(secretContent), 0o644); err != nil {
		t.Fatalf("write secret fixture: %v", err)
	}
	t.Setenv("OPENAI_API_KEY", "secret-from-env")
	t.Setenv("OPENCODE_API_KEY", "other-secret-from-env")
	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(current); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	got, err := ClassifyFakeReadiness(ReadinessRequest{Provider: "openai", Model: "gpt-4.1"})
	if err != nil {
		t.Fatalf("classify fake readiness: %v", err)
	}
	if after, err := os.ReadFile(secretPath); err != nil || string(after) != secretContent {
		t.Fatalf("secret file after classify = %q, %v; want unchanged", string(after), err)
	}
	text := readinessText(got)
	for _, forbidden := range []string{"secret-from-file", "other-secret", "secret-from-env", "other-secret-from-env"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("readiness output leaked secret %q in %q", forbidden, text)
		}
	}
}

func readinessText(readiness ProviderReadiness) string {
	var builder strings.Builder
	builder.WriteString(readiness.Provider)
	builder.WriteString(readiness.Model)
	builder.WriteString(readiness.Reasoning)
	for _, source := range readiness.CredentialSourceNames {
		builder.WriteString(source)
	}
	for _, entry := range readiness.Metadata {
		builder.WriteString(entry.Name)
		builder.WriteString(entry.Value)
	}
	return builder.String()
}
