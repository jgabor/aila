package agent

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ProviderFamily string

const (
	ProviderFamilyAPIKey     ProviderFamily = "api_key"
	ProviderFamilyCustom     ProviderFamily = "custom"
	ProviderFamilyDeviceCode ProviderFamily = "device_code"
)

const (
	CredentialSourceOpenAIAPIKey   = "OPENAI_API_KEY"
	CredentialSourceOpenCodeAPIKey = "OPENCODE_API_KEY"
	CredentialSourceDeviceCode     = "device-code"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrUnavailableModel    = errors.New("unavailable model")
)

type ReadinessRequest struct {
	Provider  string
	Model     string
	Reasoning string
	BaseURL   string
}

type ProviderReadiness struct {
	Provider                string
	Family                  ProviderFamily
	CredentialSourceNames   []string
	Model                   string
	Reasoning               string
	Metadata                []MetadataEntry
	AvailableBeforeTurn     bool
	RequiresCredentialCheck bool
}

type MetadataEntry struct {
	Name  string
	Value string
}

type ModelDiagnosticStatus string

const (
	ModelDiagnosticAvailable   ModelDiagnosticStatus = "available"
	ModelDiagnosticDegraded    ModelDiagnosticStatus = "degraded"
	ModelDiagnosticUnavailable ModelDiagnosticStatus = "unavailable"
)

type ModelDiagnostic struct {
	Provider string
	Model    string
	Family   ProviderFamily
	Class    string
	Status   ModelDiagnosticStatus
	Error    string
}

type ModelDiagnosticFilter struct {
	Provider string
	Family   ProviderFamily
	Status   ModelDiagnosticStatus
	Class    string
	Search   []string
}

type UnsupportedProviderError struct {
	Provider string
}

func (err UnsupportedProviderError) Error() string {
	return fmt.Sprintf("unsupported provider %q", err.Provider)
}

func (err UnsupportedProviderError) Unwrap() error {
	return ErrUnsupportedProvider
}

type UnavailableModelError struct {
	Provider string
	Model    string
}

func (err UnavailableModelError) Error() string {
	return fmt.Sprintf("unavailable model %q for provider %q", err.Model, err.Provider)
}

func (err UnavailableModelError) Unwrap() error {
	return ErrUnavailableModel
}

type fakeProvider struct {
	family      ProviderFamily
	credential  string
	models      map[string]string
	baseURLName string
}

var fakeProviders = map[string]fakeProvider{
	"openai": {
		family:     ProviderFamilyAPIKey,
		credential: CredentialSourceOpenAIAPIKey,
		models: map[string]string{
			"gpt-4.1":      "general",
			"gpt-4.1-mini": "utility",
			"o4-mini":      "reasoning",
		},
	},
	"opencode-zen": {
		family:     ProviderFamilyAPIKey,
		credential: CredentialSourceOpenCodeAPIKey,
		models: map[string]string{
			"zen-flash": "utility",
			"zen-pro":   "general",
		},
	},
	"custom": {
		family:      ProviderFamilyCustom,
		credential:  CredentialSourceOpenAIAPIKey,
		baseURLName: "llm.base_url",
		models: map[string]string{
			"deepseek-chat":     "general",
			"deepseek-reasoner": "reasoning",
			"local-chat":        "general",
		},
	},
	"codex": {
		family:     ProviderFamilyDeviceCode,
		credential: CredentialSourceDeviceCode,
		models: map[string]string{
			"codex-high": "reasoning",
			"codex-low":  "utility",
		},
	},
	"copilot": {
		family:     ProviderFamilyDeviceCode,
		credential: CredentialSourceDeviceCode,
		models: map[string]string{
			"copilot-chat": "general",
			"copilot-fast": "utility",
		},
	},
	"opencode-go": {
		family:     ProviderFamilyAPIKey,
		credential: CredentialSourceOpenCodeAPIKey,
		models: map[string]string{
			"deepseek-v4-flash": "utility",
			"deepseek-v4-pro":   "reasoning",
		},
	},
	"xiaomi-plan": {
		family:     ProviderFamilyDeviceCode,
		credential: CredentialSourceDeviceCode,
		models: map[string]string{
			"mi-flash": "utility",
			"mi-pro":   "general",
		},
	},
	"zai-plan": {
		family:     ProviderFamilyDeviceCode,
		credential: CredentialSourceDeviceCode,
		models: map[string]string{
			"glm-4.5":     "reasoning",
			"glm-4.5-air": "utility",
		},
	},
}

var fakeModelDiagnosticOverrides = map[string]struct {
	status ModelDiagnosticStatus
	err    string
}{
	"custom/deepseek-chat": {status: ModelDiagnosticUnavailable, err: ErrProviderUnavailable.Error()},
	"openai/gpt-4.1":       {status: ModelDiagnosticDegraded, err: ErrReadinessTimeout.Error()},
}

func ListFakeModelDiagnostics(filter ModelDiagnosticFilter) []ModelDiagnostic {
	diagnostics := make([]ModelDiagnostic, 0)
	for providerName, provider := range fakeProviders {
		models := make([]string, 0, len(provider.models))
		for model := range provider.models {
			models = append(models, model)
		}
		sort.Strings(models)

		for _, model := range models {
			readiness, err := ClassifyFakeReadiness(ReadinessRequest{Provider: providerName, Model: model})
			if err != nil {
				continue
			}
			diagnostic := ModelDiagnostic{
				Provider: readiness.Provider,
				Model:    readiness.Model,
				Family:   readiness.Family,
				Class:    metadataValue(readiness.Metadata, "model_class"),
				Status:   ModelDiagnosticAvailable,
				Error:    "-",
			}
			if override, ok := fakeModelDiagnosticOverrides[diagnostic.Provider+"/"+diagnostic.Model]; ok {
				diagnostic.Status = override.status
				diagnostic.Error = override.err
			}
			if matchesModelDiagnosticFilter(diagnostic, filter) {
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		return left.Model < right.Model
	})
	return diagnostics
}

func metadataValue(metadata []MetadataEntry, name string) string {
	for _, entry := range metadata {
		if entry.Name == name {
			return entry.Value
		}
	}
	return ""
}

func matchesModelDiagnosticFilter(diagnostic ModelDiagnostic, filter ModelDiagnosticFilter) bool {
	if filter.Provider != "" && diagnostic.Provider != filter.Provider {
		return false
	}
	if filter.Family != "" && diagnostic.Family != filter.Family {
		return false
	}
	if filter.Status != "" && diagnostic.Status != filter.Status {
		return false
	}
	if filter.Class != "" && diagnostic.Class != filter.Class {
		return false
	}
	for _, token := range filter.Search {
		if !strings.Contains(diagnostic.Provider, token) && !strings.Contains(diagnostic.Model, token) && !strings.Contains(diagnostic.Class, token) {
			return false
		}
	}
	return true
}

func ClassifyFakeReadiness(request ReadinessRequest) (ProviderReadiness, error) {
	provider, ok := fakeProviders[request.Provider]
	if !ok {
		return ProviderReadiness{}, UnsupportedProviderError{Provider: request.Provider}
	}
	modelClass, ok := provider.models[request.Model]
	if !ok {
		return ProviderReadiness{}, UnavailableModelError{Provider: request.Provider, Model: request.Model}
	}

	metadata := []MetadataEntry{
		{Name: "provider", Value: request.Provider},
		{Name: "family", Value: string(provider.family)},
		{Name: "credential_source", Value: provider.credential},
		{Name: "model", Value: request.Model},
		{Name: "model_class", Value: modelClass},
		{Name: "reasoning", Value: request.Reasoning},
	}
	if provider.family == ProviderFamilyCustom {
		metadata = append(metadata,
			MetadataEntry{Name: "base_url_source", Value: provider.baseURLName},
			MetadataEntry{Name: "base_url", Value: request.BaseURL},
		)
	}

	requiresCredentialCheck := provider.family != ProviderFamilyDeviceCode

	return ProviderReadiness{
		Provider:                request.Provider,
		Family:                  provider.family,
		CredentialSourceNames:   []string{provider.credential},
		Model:                   request.Model,
		Reasoning:               request.Reasoning,
		Metadata:                metadata,
		AvailableBeforeTurn:     true,
		RequiresCredentialCheck: requiresCredentialCheck,
	}, nil
}
