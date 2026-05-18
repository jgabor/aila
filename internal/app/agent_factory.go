package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	goagent "github.com/jgabor/go-agent"
	openai "github.com/jgabor/go-agent/providers/openai"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/tools"
)

type agentBuildRunnerSelection struct {
	Runner    agent.Runner
	Provider  string
	Model     string
	ToolNames []string
}

func newAgentBuildRunnerFromConfig(workspacePath string, autonomyLevel string, config Config, lookupEnv agent.EnvLookup, client *http.Client) agentBuildRunnerSelection {
	ref := config.LLM.Model
	provider := ref.Provider
	modelLabel := ref.Label
	if strings.TrimSpace(modelLabel) == "" {
		modelLabel = strings.TrimSpace(provider + "/" + ref.Model)
	}

	readiness, err := agent.ClassifyFakeReadiness(agent.ReadinessRequest{Provider: ref.Provider, Model: ref.Model, Reasoning: ref.Reasoning, BaseURL: config.LLM.BaseURL})
	if err != nil {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureModelUnavailable), err.Error(), false)
	}
	if readiness.Family == agent.ProviderFamilyDeviceCode {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureModelUnavailable), fmt.Sprintf("provider %q uses device-code authentication, which is not wired for real go-agent turns yet", provider), false)
	}
	if provider != "openai" && provider != "custom" && provider != "opencode-go" {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureModelUnavailable), fmt.Sprintf("provider %q does not have a production go-agent model adapter yet", provider), false)
	}
	baseURL := openAICompatibleBaseURL(provider, config.LLM.BaseURL)
	if provider == "custom" && baseURL == "" {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureModelUnavailable), "llm.base_url is required for custom OpenAI-compatible models", false)
	}

	credential, err := agent.ResolveCredential(agent.CredentialRequest{
		Provider:    readiness.Provider,
		Family:      readiness.Family,
		SourceNames: readiness.CredentialSourceNames,
		LookupEnv:   lookupEnv,
	})
	if err != nil {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureProviderAuth), err.Error(), false)
	}

	agentTools, toolNames, err := newGoAgentBuildTools(workspacePath, permission.AutonomyLevel(autonomyLevel))
	if err != nil {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureStreamError), err.Error(), true)
	}
	chatModel := openai.ChatModel{
		Model:      ref.Model,
		APIKey:     credential.Value.Value(),
		BaseURL:    baseURL,
		HTTPClient: client,
		Options: openai.ChatOptions{
			ReasoningEffort:    openAIReasoningEffort(ref.Reasoning),
			IncludeStreamUsage: true,
		},
	}
	runner, err := agent.NewGoAgentRunner(chatModel, provider, modelLabel, agentTools...)
	if err != nil {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureStreamError), err.Error(), true)
	}
	return agentBuildRunnerSelection{Runner: runner, Provider: provider, Model: modelLabel, ToolNames: toolNames}
}

func openAICompatibleBaseURL(provider string, configured string) string {
	baseURL := strings.TrimSpace(configured)
	if baseURL != "" {
		return baseURL
	}
	if provider == "opencode-go" {
		return "https://opencode.ai/zen/go/v1"
	}
	return ""
}

func unavailableAgentBuildRunner(provider string, model string, code string, message string, retryable bool) agentBuildRunnerSelection {
	return agentBuildRunnerSelection{
		Runner:   agent.UnavailableRunner{Provider: provider, Model: model, Failure: agent.ProviderError{Code: code, Message: message, Retryable: retryable}},
		Provider: provider,
		Model:    model,
	}
}

func openAIReasoningEffort(value string) openai.ReasoningEffort {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return openai.ReasoningEffortLow
	case "medium":
		return openai.ReasoningEffortMedium
	case "high":
		return openai.ReasoningEffortHigh
	default:
		return ""
	}
}

type goAgentReadInput struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	LineLimit       int    `json:"line_limit"`
	MaxPreviewBytes int    `json:"max_preview_bytes"`
}

type goAgentWriteInput struct {
	Path           string `json:"path"`
	TargetVersion  string `json:"target_version"`
	Content        string `json:"content"`
	ExpectedEffect string `json:"expected_effect"`
}

func newGoAgentBuildTools(workspacePath string, autonomyLevel permission.AutonomyLevel) ([]goagent.Tool, []string, error) {
	readTool, err := goagent.NewToolFromDefinition(goagent.ToolDefinition{
		Name:        tools.ReadToolName,
		Description: "Read a workspace file with bounded line and preview limits.",
		Schema:      goAgentReadToolSchema(),
		Safety:      goagent.ToolSafety{ReadOnly: true, Retryable: true},
		Constraints: goagent.ToolConstraints{MaxOutputBytes: tools.DefaultReadMaxPreviewBytes},
		Function: func(ctx context.Context, input goAgentReadInput) (goagent.ToolResult, error) {
			return callGoAgentReadTool(ctx, workspacePath, autonomyLevel, input), nil
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build read tool: %w", err)
	}
	writeTool, err := goagent.NewToolFromDefinition(goagent.ToolDefinition{
		Name:        tools.WriteToolName,
		Description: "Request a guarded workspace write; Aila will ask for approval before mutating files.",
		Schema:      goAgentWriteToolSchema(),
		Safety:      goagent.ToolSafety{ReadOnly: false, Retryable: false},
		Function: func(context.Context, goAgentWriteInput) (goagent.ToolResult, error) {
			return goagent.ToolResult{Content: "write request captured for Aila approval; do not claim the file was changed yet", Metadata: map[string]string{"approval_required": "true"}}, nil
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("build write tool: %w", err)
	}
	return []goagent.Tool{readTool, writeTool}, []string{tools.ReadToolName, tools.WriteToolName}, nil
}

func goAgentReadToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"path":              map[string]any{"type": "string", "description": "Workspace-relative file path to read."},
			"start_line":        map[string]any{"type": "integer", "description": "Optional 1-based start line."},
			"line_limit":        map[string]any{"type": "integer", "description": "Optional maximum number of lines."},
			"max_preview_bytes": map[string]any{"type": "integer", "description": "Optional maximum preview bytes."},
		},
		"required": []string{"path"},
	}
}

func goAgentWriteToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"path":            map[string]any{"type": "string", "description": "Workspace-relative file path to write."},
			"target_version":  map[string]any{"type": "string", "description": "Expected current file version, or missing for a new file."},
			"content":         map[string]any{"type": "string", "description": "Complete desired file content."},
			"expected_effect": map[string]any{"type": "string", "description": "Plain-language reason for the write."},
		},
		"required": []string{"path", "content", "expected_effect"},
	}
}

func callGoAgentReadTool(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, input goAgentReadInput) goagent.ToolResult {
	decision := permission.DecideRecord(autonomyLevel, permission.NewReadOperation(input.Path))
	if !decision.Allowed {
		return goagent.ToolResult{Name: tools.ReadToolName, Content: "read denied: " + decision.Reason, Metadata: map[string]string{"status": "denied"}}
	}
	validated, validationErr := tools.ValidateReadRequest(workspacePath, tools.ReadRequest{
		Path:            input.Path,
		StartLine:       input.StartLine,
		LineLimit:       input.LineLimit,
		MaxPreviewBytes: input.MaxPreviewBytes,
		Source: tools.ReadSourceMetadata{
			Caller:      "go-agent",
			RequestID:   "go-agent-read",
			Description: "model-requested workspace read",
		},
	})
	if validationErr.Kind != "" {
		return goagent.ToolResult{Name: tools.ReadToolName, Content: "read rejected: " + validationErr.Message, Metadata: map[string]string{"status": string(validationErr.Kind)}}
	}
	result := tools.ExecuteRead(ctx, validated)
	if result.Error.Kind != "" && result.Error.Kind != tools.ReadErrorNone {
		return goagent.ToolResult{Name: tools.ReadToolName, Content: "read failed: " + result.Error.Message, Metadata: map[string]string{"status": string(result.Error.Kind)}}
	}
	return goagent.ToolResult{
		Name:      tools.ReadToolName,
		Content:   result.PreviewText,
		Truncated: result.Truncation.PreviewTruncated || result.Truncation.LineLimitHit,
		SourceRef: result.WorkspaceRelativePath,
		Metadata: map[string]string{
			"status": "completed",
			"path":   result.WorkspaceRelativePath,
		},
	}
}
