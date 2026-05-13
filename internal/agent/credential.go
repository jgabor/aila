package agent

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMissingCredential   = errors.New("missing credential")
	ErrInvalidCredential   = errors.New("invalid credential")
	ErrExpiredToken        = errors.New("expired token")
	ErrRateLimited         = errors.New("rate limited")
	ErrReadinessTimeout    = errors.New("readiness timeout")
	ErrProviderUnavailable = errors.New("provider unavailable")
)

type EnvLookup func(string) (string, bool)

type Secret struct {
	value string
}

func NewSecret(value string) Secret {
	return Secret{value: value}
}

func (secret Secret) Value() string {
	return secret.value
}

func (secret Secret) String() string {
	if secret.value == "" {
		return "<empty>"
	}
	return "<redacted>"
}

type CredentialDescriptor struct {
	SourceName string
	Value      Secret
}

func (descriptor CredentialDescriptor) String() string {
	return fmt.Sprintf("credential source %q value %s", descriptor.SourceName, descriptor.Value)
}

type Credential struct {
	SourceName string
	Value      Secret
	Present    bool
	Required   bool
}

func (credential Credential) String() string {
	if !credential.Required {
		return fmt.Sprintf("credential source %q not required", credential.SourceName)
	}
	if !credential.Present {
		return fmt.Sprintf("credential source %q missing", credential.SourceName)
	}
	return fmt.Sprintf("credential source %q present value %s", credential.SourceName, credential.Value)
}

type CredentialRequest struct {
	Provider         string
	Family           ProviderFamily
	SourceNames      []string
	ConfigCredential *CredentialDescriptor
	LookupEnv        EnvLookup
}

func ResolveCredential(request CredentialRequest) (Credential, error) {
	if request.Family == ProviderFamilyDeviceCode {
		return Credential{SourceName: CredentialSourceDeviceCode, Required: false}, nil
	}

	if request.ConfigCredential != nil && request.ConfigCredential.Value.Value() != "" {
		return Credential{
			SourceName: request.ConfigCredential.SourceName,
			Value:      request.ConfigCredential.Value,
			Present:    true,
			Required:   true,
		}, nil
	}

	if request.LookupEnv != nil {
		for _, sourceName := range request.SourceNames {
			if value, ok := request.LookupEnv(sourceName); ok && value != "" {
				return Credential{SourceName: sourceName, Value: NewSecret(value), Present: true, Required: true}, nil
			}
		}
	}

	return Credential{}, MissingCredentialError{Provider: request.Provider, SourceNames: request.SourceNames}
}

type MissingCredentialError struct {
	Provider    string
	SourceNames []string
}

func (err MissingCredentialError) Error() string {
	return fmt.Sprintf("missing credential for provider %q from %s", err.Provider, strings.Join(err.SourceNames, ", "))
}

func (err MissingCredentialError) Unwrap() error {
	return ErrMissingCredential
}

type CredentialFailureError struct {
	Kind       error
	Provider   string
	SourceName string
	RetryAfter time.Duration
}

func (err CredentialFailureError) Error() string {
	message := err.Kind.Error()
	if err.RetryAfter > 0 {
		message = fmt.Sprintf("%s retry_after=%s", message, err.RetryAfter)
	}
	if err.SourceName == "" {
		return fmt.Sprintf("%s for provider %q", message, err.Provider)
	}
	return fmt.Sprintf("%s for provider %q credential source %q value <redacted>", message, err.Provider, err.SourceName)
}

func (err CredentialFailureError) Unwrap() error {
	return err.Kind
}

type FakeReadinessFailure string

const (
	FakeReadinessOK                  FakeReadinessFailure = ""
	FakeReadinessInvalidCredential   FakeReadinessFailure = "invalid_credential"
	FakeReadinessExpiredToken        FakeReadinessFailure = "expired_token"
	FakeReadinessRateLimited         FakeReadinessFailure = "rate_limited"
	FakeReadinessTimeout             FakeReadinessFailure = "timeout"
	FakeReadinessProviderUnavailable FakeReadinessFailure = "provider_unavailable"
)

type FakeReadinessRequest struct {
	ReadinessRequest
	ConfigCredential *CredentialDescriptor
	LookupEnv        EnvLookup
	Failure          FakeReadinessFailure
	RetryAfter       time.Duration
}

func ResolveFakeReadiness(request FakeReadinessRequest) (ProviderReadiness, error) {
	readiness, err := ClassifyFakeReadiness(request.ReadinessRequest)
	if err != nil {
		return ProviderReadiness{}, err
	}

	credential, err := ResolveCredential(CredentialRequest{
		Provider:         readiness.Provider,
		Family:           readiness.Family,
		SourceNames:      readiness.CredentialSourceNames,
		ConfigCredential: request.ConfigCredential,
		LookupEnv:        request.LookupEnv,
	})
	if err != nil {
		return ProviderReadiness{}, err
	}

	readiness.Metadata = append(readiness.Metadata,
		MetadataEntry{Name: "credential_resolved_source", Value: credential.SourceName},
		MetadataEntry{Name: "credential_present", Value: fmt.Sprintf("%t", credential.Present)},
	)

	switch request.Failure {
	case FakeReadinessOK:
		return readiness, nil
	case FakeReadinessInvalidCredential:
		return ProviderReadiness{}, CredentialFailureError{Kind: ErrInvalidCredential, Provider: readiness.Provider, SourceName: credential.SourceName}
	case FakeReadinessExpiredToken:
		return ProviderReadiness{}, CredentialFailureError{Kind: ErrExpiredToken, Provider: readiness.Provider, SourceName: credential.SourceName}
	case FakeReadinessRateLimited:
		return ProviderReadiness{}, CredentialFailureError{Kind: ErrRateLimited, Provider: readiness.Provider, SourceName: credential.SourceName, RetryAfter: request.RetryAfter}
	case FakeReadinessTimeout:
		return ProviderReadiness{}, CredentialFailureError{Kind: ErrReadinessTimeout, Provider: readiness.Provider, SourceName: credential.SourceName}
	case FakeReadinessProviderUnavailable:
		return ProviderReadiness{}, CredentialFailureError{Kind: ErrProviderUnavailable, Provider: readiness.Provider}
	default:
		return ProviderReadiness{}, fmt.Errorf("unsupported fake readiness failure %q", request.Failure)
	}
}
