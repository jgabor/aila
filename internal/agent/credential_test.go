package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const (
	testConfigSecret = "sk-config-synthetic-secret"
	testEnvSecret    = "sk-env-synthetic-secret"
)

func TestResolveCredentialSupportsDocumentedEnvNames(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		provider   string
		family     ProviderFamily
		sourceName string
	}{
		{name: "openai", provider: "openai", family: ProviderFamilyAPIKey, sourceName: CredentialSourceOpenAIAPIKey},
		{name: "custom", provider: "custom", family: ProviderFamilyCustom, sourceName: CredentialSourceOpenAIAPIKey},
		{name: "opencode-zen", provider: "opencode-zen", family: ProviderFamilyAPIKey, sourceName: CredentialSourceOpenCodeAPIKey},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			credential, err := ResolveCredential(CredentialRequest{
				Provider:    tc.provider,
				Family:      tc.family,
				SourceNames: []string{tc.sourceName},
				LookupEnv: func(name string) (string, bool) {
					if name != tc.sourceName {
						return "", false
					}
					return testEnvSecret, true
				},
			})
			if err != nil {
				t.Fatalf("resolve credential: %v", err)
			}
			if credential.SourceName != tc.sourceName || credential.Value.Value() != testEnvSecret || !credential.Present || !credential.Required {
				t.Fatalf("credential = %+v, want source %q present required", credential, tc.sourceName)
			}
		})
	}
}

func TestResolveCredentialConfigPrecedesEnv(t *testing.T) {
	t.Parallel()

	credential, err := ResolveCredential(CredentialRequest{
		Provider:    "openai",
		Family:      ProviderFamilyAPIKey,
		SourceNames: []string{CredentialSourceOpenAIAPIKey},
		ConfigCredential: &CredentialDescriptor{
			SourceName: "config:llm.api_key",
			Value:      NewSecret(testConfigSecret),
		},
		LookupEnv: func(name string) (string, bool) {
			return testEnvSecret, true
		},
	})
	if err != nil {
		t.Fatalf("resolve credential: %v", err)
	}
	if credential.SourceName != "config:llm.api_key" || credential.Value.Value() != testConfigSecret {
		t.Fatalf("credential = %+v, want config credential", credential)
	}
}

func TestResolveCredentialDeviceCodeDoesNotRequireAPIKeySecret(t *testing.T) {
	t.Parallel()

	credential, err := ResolveCredential(CredentialRequest{
		Provider:    "codex",
		Family:      ProviderFamilyDeviceCode,
		SourceNames: []string{CredentialSourceDeviceCode},
		LookupEnv: func(name string) (string, bool) {
			t.Fatalf("device-code resolver unexpectedly looked up env %q", name)
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("resolve credential: %v", err)
	}
	if credential.SourceName != CredentialSourceDeviceCode || credential.Required || credential.Present || credential.Value.Value() != "" {
		t.Fatalf("credential = %+v, want device-code no-secret credential", credential)
	}
}

func TestResolveCredentialMissingCredentialTypedError(t *testing.T) {
	t.Parallel()

	_, err := ResolveCredential(CredentialRequest{
		Provider:    "opencode-zen",
		Family:      ProviderFamilyAPIKey,
		SourceNames: []string{CredentialSourceOpenCodeAPIKey},
		LookupEnv: func(name string) (string, bool) {
			return "", false
		},
	})
	if !errors.Is(err, ErrMissingCredential) {
		t.Fatalf("error = %v, want ErrMissingCredential", err)
	}
	var typed MissingCredentialError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %T %[1]v, want MissingCredentialError", err)
	}
	if typed.Provider != "opencode-zen" || len(typed.SourceNames) != 1 || typed.SourceNames[0] != CredentialSourceOpenCodeAPIKey {
		t.Fatalf("typed error = %+v, want opencode-zen %s", typed, CredentialSourceOpenCodeAPIKey)
	}
}

func TestResolveFakeReadinessFailuresAreTyped(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		failure FakeReadinessFailure
		want    error
	}{
		{name: "invalid key", failure: FakeReadinessInvalidCredential, want: ErrInvalidCredential},
		{name: "expired token", failure: FakeReadinessExpiredToken, want: ErrExpiredToken},
		{name: "rate limit", failure: FakeReadinessRateLimited, want: ErrRateLimited},
		{name: "timeout", failure: FakeReadinessTimeout, want: ErrReadinessTimeout},
		{name: "provider unavailable", failure: FakeReadinessProviderUnavailable, want: ErrProviderUnavailable},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ResolveFakeReadiness(FakeReadinessRequest{
				ReadinessRequest: ReadinessRequest{Provider: "openai", Model: "gpt-4.1"},
				LookupEnv: func(name string) (string, bool) {
					return testEnvSecret, true
				},
				Failure:    tc.failure,
				RetryAfter: 30 * time.Second,
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			var typed CredentialFailureError
			if !errors.As(err, &typed) {
				t.Fatalf("error = %T %[1]v, want CredentialFailureError", err)
			}
			if typed.Provider != "openai" {
				t.Fatalf("typed provider = %q, want openai", typed.Provider)
			}
		})
	}
}

func TestResolveFakeReadinessUnavailableModelStillTyped(t *testing.T) {
	t.Parallel()

	_, err := ResolveFakeReadiness(FakeReadinessRequest{
		ReadinessRequest: ReadinessRequest{Provider: "openai", Model: "not-real"},
		LookupEnv: func(name string) (string, bool) {
			return testEnvSecret, true
		},
	})
	if !errors.Is(err, ErrUnavailableModel) {
		t.Fatalf("error = %v, want ErrUnavailableModel", err)
	}
}

func TestResolveFakeReadinessRedactsSecretsAcrossErrorsAndOutput(t *testing.T) {
	t.Parallel()

	readiness, err := ResolveFakeReadiness(FakeReadinessRequest{
		ReadinessRequest: ReadinessRequest{Provider: "custom", Model: "deepseek-chat", BaseURL: "https://api.deepseek.com"},
		ConfigCredential: &CredentialDescriptor{
			SourceName: "config:llm.api_key",
			Value:      NewSecret(testConfigSecret),
		},
	})
	if err != nil {
		t.Fatalf("resolve fake readiness: %v", err)
	}
	assertNoSyntheticSecrets(t, readinessText(readiness))

	credential := Credential{SourceName: "config:llm.api_key", Value: NewSecret(testConfigSecret), Present: true, Required: true}
	descriptor := CredentialDescriptor{SourceName: "config:llm.api_key", Value: NewSecret(testConfigSecret)}
	failure := CredentialFailureError{Kind: ErrInvalidCredential, Provider: "custom", SourceName: "config:llm.api_key"}
	for _, text := range []string{fmt.Sprint(credential), fmt.Sprintf("%+v", credential), fmt.Sprint(descriptor), fmt.Sprintf("%+v", descriptor), failure.Error()} {
		assertNoSyntheticSecrets(t, text)
		if !strings.Contains(text, "<redacted>") {
			t.Fatalf("text %q does not show redacted state", text)
		}
	}
}

func assertNoSyntheticSecrets(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{testConfigSecret, testEnvSecret} {
		if strings.Contains(text, secret) {
			t.Fatalf("text leaked synthetic secret %q in %q", secret, text)
		}
	}
}
