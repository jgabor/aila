package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	goagent "github.com/jgabor/go-agent"
	openai "github.com/jgabor/go-agent/providers/openai"

	"github.com/jgabor/aila/internal/agent"
	ailacontext "github.com/jgabor/aila/internal/context"
	"github.com/jgabor/aila/internal/permission"
	"github.com/jgabor/aila/internal/tools"
)

type agentBuildRunnerSelection struct {
	Runner       agent.Runner
	Provider     string
	Model        string
	ToolNames    []string
	Instructions string
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
	instructions := buildAgentInstructions(workspacePath, autonomyLevel, toolNames)
	runner, err := agent.NewGoAgentRunnerWithInstructions(chatModel, provider, modelLabel, instructions, agentTools...)
	if err != nil {
		return unavailableAgentBuildRunner(provider, modelLabel, string(agent.FailureStreamError), err.Error(), true)
	}
	return agentBuildRunnerSelection{Runner: runner, Provider: provider, Model: modelLabel, ToolNames: toolNames, Instructions: instructions}
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

type goAgentEditInput struct {
	Path           string `json:"path"`
	TargetVersion  string `json:"target_version"`
	OldText        string `json:"old_text"`
	NewText        string `json:"new_text"`
	ExpectedEffect string `json:"expected_effect"`
}

type goAgentFindInput struct {
	Pattern         string `json:"pattern"`
	MaxResults      int    `json:"max_results"`
	MaxPreviewBytes int    `json:"max_preview_bytes"`
}

type goAgentGrepInput struct {
	Query           string `json:"query"`
	Regex           bool   `json:"regex"`
	IncludePattern  string `json:"include_pattern"`
	MaxResults      int    `json:"max_results"`
	MaxPreviewBytes int    `json:"max_preview_bytes"`
}

type goAgentBashInput struct {
	Argv           []string `json:"argv"`
	WorkingDir     string   `json:"working_dir"`
	MaxOutputBytes int      `json:"max_output_bytes"`
	TimeoutMillis  int      `json:"timeout_millis"`
}

type goAgentFetchInput struct {
	URL             string `json:"url"`
	Method          string `json:"method"`
	MaxPreviewBytes int    `json:"max_preview_bytes"`
	TimeoutMillis   int    `json:"timeout_millis"`
}

func newGoAgentBuildTools(workspacePath string, autonomyLevel permission.AutonomyLevel) ([]goagent.Tool, []string, error) {
	definitions := []goagent.ToolDefinition{
		{
			Name:        tools.ReadToolName,
			Description: "Read a workspace file with bounded line and preview limits.",
			Schema:      goAgentReadToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: true, Retryable: true},
			Constraints: goagent.ToolConstraints{MaxOutputBytes: tools.DefaultReadMaxPreviewBytes},
			Function: func(ctx context.Context, input goAgentReadInput) (goagent.ToolResult, error) {
				return callGoAgentReadTool(ctx, workspacePath, autonomyLevel, input), nil
			},
		},
		{
			Name:        tools.FindToolName,
			Description: "Find workspace files by a bounded glob-style pattern.",
			Schema:      goAgentFindToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: true, Retryable: true},
			Constraints: goagent.ToolConstraints{MaxOutputBytes: tools.DefaultSearchMaxPreviewBytes * tools.DefaultSearchMaxResults},
			Function: func(ctx context.Context, input goAgentFindInput) (goagent.ToolResult, error) {
				return callGoAgentFindTool(ctx, workspacePath, autonomyLevel, input), nil
			},
		},
		{
			Name:        tools.GrepToolName,
			Description: "Search workspace file contents with bounded results and previews.",
			Schema:      goAgentGrepToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: true, Retryable: true},
			Constraints: goagent.ToolConstraints{MaxOutputBytes: tools.DefaultSearchMaxPreviewBytes * tools.DefaultSearchMaxResults},
			Function: func(ctx context.Context, input goAgentGrepInput) (goagent.ToolResult, error) {
				return callGoAgentGrepTool(ctx, workspacePath, autonomyLevel, input), nil
			},
		},
		{
			Name:        tools.BashToolName,
			Description: "Run a safe allowlisted inspection command through argv, never shell syntax.",
			Schema:      goAgentBashToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: true, Retryable: false},
			Constraints: goagent.ToolConstraints{MaxOutputBytes: tools.DefaultBashMaxOutputBytes},
			Function: func(ctx context.Context, input goAgentBashInput) (goagent.ToolResult, error) {
				return callGoAgentBashTool(ctx, workspacePath, autonomyLevel, input), nil
			},
		},
		{
			Name:        tools.FetchToolName,
			Description: "Fetch bounded HTTP(S) content with safe methods only.",
			Schema:      goAgentFetchToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: true, Retryable: true},
			Constraints: goagent.ToolConstraints{MaxOutputBytes: tools.DefaultFetchMaxPreviewBytes},
			Function: func(ctx context.Context, input goAgentFetchInput) (goagent.ToolResult, error) {
				return callGoAgentFetchTool(ctx, autonomyLevel, input), nil
			},
		},
		{
			Name:        tools.EditToolName,
			Description: "Request a guarded workspace edit; Aila will ask for approval before mutating files.",
			Schema:      goAgentEditToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: false, Retryable: false},
			Function: func(context.Context, goAgentEditInput) (goagent.ToolResult, error) {
				return guardedMutationToolResult(tools.EditToolName), nil
			},
		},
		{
			Name:        tools.WriteToolName,
			Description: "Request a guarded workspace write; Aila will ask for approval before mutating files.",
			Schema:      goAgentWriteToolSchema(),
			Safety:      goagent.ToolSafety{ReadOnly: false, Retryable: false},
			Function: func(context.Context, goAgentWriteInput) (goagent.ToolResult, error) {
				return guardedMutationToolResult(tools.WriteToolName), nil
			},
		},
	}
	agentTools := make([]goagent.Tool, 0, len(definitions))
	toolNames := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		tool, err := goagent.NewToolFromDefinition(definition)
		if err != nil {
			return nil, nil, fmt.Errorf("build %s tool: %w", definition.Name, err)
		}
		agentTools = append(agentTools, tool)
		toolNames = append(toolNames, definition.Name)
	}
	return agentTools, toolNames, nil
}

func buildAgentInstructions(workspacePath string, autonomyLevel string, toolNames []string) string {
	workspace := strings.TrimSpace(workspacePath)
	if workspace == "" {
		workspace = "."
	}
	lines := []string{
		"You are Aila, a terminal coding agent for this project.",
		"Keep statechart-MVU boundaries: runtime.Update is deterministic and IO-free; TUI is presentation-only.",
		"Workspace: " + workspace,
		"Project: github.com/jgabor/aila",
		"Project summary: Aila is a minimal fixed-product terminal coding agent built on go-agent and Bubble Tea.",
		"Workflow phase: build",
		"Active capability: build",
		"Autonomy level: " + strings.TrimSpace(autonomyLevel),
		"Fixed built-in tools only; no dynamic tools or plugins are available.",
		"Tool definitions:",
	}
	for _, line := range buildToolInstructionLines(toolNames) {
		lines = append(lines, "- "+line)
	}
	lines = append(lines,
		"Use read/find/grep/bash/fetch for bounded inspection. Use edit/write only to request a guarded mutation; Aila approval is required before files change.",
	)
	built := ailacontext.Build(ailacontext.BuildInput{
		UserConstraints: []ailacontext.UserConstraintInput{{Text: strings.Join(lines, "\n")}},
		MaxBytes:        8 * 1024,
	})
	if len(built.Blocks) == 0 || strings.TrimSpace(built.Blocks[0].Text) == "" {
		return strings.Join(lines, "\n")
	}
	return built.Blocks[0].Text
}

func buildToolInstructionLines(toolNames []string) []string {
	lines := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		switch strings.TrimSpace(name) {
		case tools.ReadToolName:
			lines = append(lines, "read(path,start_line,line_limit,max_preview_bytes): read a workspace file with bounded line and preview limits.")
		case tools.FindToolName:
			lines = append(lines, "find(pattern,max_results,max_preview_bytes): find workspace files by bounded glob-style pattern.")
		case tools.GrepToolName:
			lines = append(lines, "grep(query,regex,include_pattern,max_results,max_preview_bytes): search workspace file contents with bounded results and previews.")
		case tools.BashToolName:
			lines = append(lines, "bash(argv,working_dir,max_output_bytes,timeout_millis): run safe allowlisted inspection commands through argv, never shell syntax.")
		case tools.FetchToolName:
			lines = append(lines, "fetch(url,method,max_preview_bytes,timeout_millis): fetch bounded HTTP(S) content with safe methods only.")
		case tools.EditToolName:
			lines = append(lines, "edit(path,target_version,old_text,new_text,expected_effect): request a guarded exact-text edit; approval is required before mutation.")
		case tools.WriteToolName:
			lines = append(lines, "write(path,target_version,content,expected_effect): request a guarded whole-file write; approval is required before mutation.")
		}
	}
	return lines
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

func goAgentEditToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"path":            map[string]any{"type": "string", "description": "Workspace-relative file path to edit."},
			"target_version":  map[string]any{"type": "string", "description": "Expected current file version."},
			"old_text":        map[string]any{"type": "string", "description": "Exact text to replace."},
			"new_text":        map[string]any{"type": "string", "description": "Replacement text."},
			"expected_effect": map[string]any{"type": "string", "description": "Plain-language reason for the edit."},
		},
		"required": []string{"path", "old_text", "new_text", "expected_effect"},
	}
}

func goAgentFindToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"pattern":           map[string]any{"type": "string", "description": "Workspace glob-style pattern."},
			"max_results":       map[string]any{"type": "integer", "description": "Optional maximum match count."},
			"max_preview_bytes": map[string]any{"type": "integer", "description": "Optional maximum preview bytes per result."},
		},
		"required": []string{"pattern"},
	}
}

func goAgentGrepToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"query":             map[string]any{"type": "string", "description": "Text or regex query."},
			"regex":             map[string]any{"type": "boolean", "description": "Whether query is a regular expression."},
			"include_pattern":   map[string]any{"type": "string", "description": "Optional workspace glob-style include filter."},
			"max_results":       map[string]any{"type": "integer", "description": "Optional maximum match count."},
			"max_preview_bytes": map[string]any{"type": "integer", "description": "Optional maximum preview bytes per result."},
		},
		"required": []string{"query"},
	}
}

func goAgentBashToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"argv":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command argv, without shell syntax."},
			"working_dir":      map[string]any{"type": "string", "description": "Workspace-relative working directory."},
			"max_output_bytes": map[string]any{"type": "integer", "description": "Optional stdout/stderr byte bound."},
			"timeout_millis":   map[string]any{"type": "integer", "description": "Optional timeout in milliseconds."},
		},
		"required": []string{"argv"},
	}
}

func goAgentFetchToolSchema() goagent.ToolSchema {
	return goagent.ToolSchema{
		"type": "object",
		"properties": map[string]any{
			"url":               map[string]any{"type": "string", "description": "Absolute HTTP(S) URL."},
			"method":            map[string]any{"type": "string", "description": "GET or HEAD."},
			"max_preview_bytes": map[string]any{"type": "integer", "description": "Optional maximum preview bytes."},
			"timeout_millis":    map[string]any{"type": "integer", "description": "Optional timeout in milliseconds."},
		},
		"required": []string{"url"},
	}
}

func guardedMutationToolResult(name string) goagent.ToolResult {
	return goagent.ToolResult{Name: name, Content: name + " request captured for Aila approval; do not claim the file was changed yet", Metadata: map[string]string{"approval_required": "true"}}
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

func callGoAgentFindTool(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, input goAgentFindInput) goagent.ToolResult {
	decision := permission.DecideRecord(autonomyLevel, permission.NewFindOperation(input.Pattern))
	if !decision.Allowed {
		return toolFailureResult(tools.FindToolName, "find denied: "+decision.Reason, "denied")
	}
	validated, validationErr := tools.ValidateFindRequest(workspacePath, tools.FindRequest{Pattern: input.Pattern, MaxResults: input.MaxResults, MaxPreviewBytes: input.MaxPreviewBytes, Source: goAgentSearchSource(tools.FindToolName)})
	if validationErr.Kind != "" {
		return toolFailureResult(tools.FindToolName, "find rejected: "+validationErr.Message, string(validationErr.Kind))
	}
	return searchToolResult(tools.ExecuteFind(ctx, validated))
}

func callGoAgentGrepTool(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, input goAgentGrepInput) goagent.ToolResult {
	decision := permission.DecideRecord(autonomyLevel, permission.NewGrepOperation(input.Query, input.IncludePattern))
	if !decision.Allowed {
		return toolFailureResult(tools.GrepToolName, "grep denied: "+decision.Reason, "denied")
	}
	validated, validationErr := tools.ValidateGrepRequest(workspacePath, tools.GrepRequest{Query: input.Query, Regex: input.Regex, IncludePattern: input.IncludePattern, MaxResults: input.MaxResults, MaxPreviewBytes: input.MaxPreviewBytes, Source: goAgentSearchSource(tools.GrepToolName)})
	if validationErr.Kind != "" {
		return toolFailureResult(tools.GrepToolName, "grep rejected: "+validationErr.Message, string(validationErr.Kind))
	}
	return searchToolResult(tools.ExecuteGrep(ctx, validated))
}

func callGoAgentBashTool(ctx context.Context, workspacePath string, autonomyLevel permission.AutonomyLevel, input goAgentBashInput) goagent.ToolResult {
	validated, validationErr := tools.ValidateBashRequest(workspacePath, tools.BashRequest{Argv: input.Argv, WorkingDir: input.WorkingDir, MaxOutputBytes: input.MaxOutputBytes, TimeoutMillis: input.TimeoutMillis, Source: tools.BashSourceMetadata{Caller: "go-agent", RequestID: "go-agent-bash", Description: "model-requested safe inspection command"}})
	if validationErr.Kind != "" {
		return toolFailureResult(tools.BashToolName, "bash rejected: "+validationErr.Message, string(validationErr.Kind))
	}
	decision := permission.DecideRecord(autonomyLevel, permission.NewBashInspectionOperation(validated.EffectiveArgv, validated.WorkspaceRelativeWorkDir, validated.ExpectedEffect))
	if !decision.Allowed {
		return toolFailureResult(tools.BashToolName, "bash denied: "+decision.Reason, "denied")
	}
	result := tools.ExecuteBash(ctx, validated)
	content := strings.TrimSpace(result.Stdout.Text)
	if content == "" {
		content = strings.TrimSpace(result.Stderr.Text)
	}
	if result.Error.Kind != "" && result.Error.Kind != tools.BashErrorNone {
		content = "bash failed: " + result.Error.Message
	}
	return goagent.ToolResult{Name: tools.BashToolName, Content: content, Truncated: result.Stdout.Truncated || result.Stderr.Truncated, Metadata: map[string]string{"status": result.Status, "exit_code": fmt.Sprint(result.ExitCode)}}
}

func callGoAgentFetchTool(ctx context.Context, autonomyLevel permission.AutonomyLevel, input goAgentFetchInput) goagent.ToolResult {
	validated, validationErr := tools.ValidateFetchRequest(tools.FetchRequest{URL: input.URL, Method: input.Method, MaxPreviewBytes: input.MaxPreviewBytes, TimeoutMillis: input.TimeoutMillis, Source: tools.FetchSourceMetadata{Caller: "go-agent", RequestID: "go-agent-fetch", Description: "model-requested remote read"}})
	if validationErr.Kind != "" {
		return toolFailureResult(tools.FetchToolName, "fetch rejected: "+validationErr.Message, string(validationErr.Kind))
	}
	decision := permission.DecideRecord(autonomyLevel, permission.NewFetchOperation(validated.EffectiveURL))
	if !decision.Allowed {
		return toolFailureResult(tools.FetchToolName, "fetch denied: "+decision.Reason, "denied")
	}
	result := tools.ExecuteFetch(ctx, validated)
	content := result.PreviewText
	if result.Error.Kind != "" && result.Error.Kind != tools.FetchErrorNone {
		content = "fetch failed: " + result.Error.Message
	}
	return goagent.ToolResult{Name: tools.FetchToolName, Content: content, Truncated: result.Truncation.PreviewTruncated, SourceRef: result.EffectiveURL, Metadata: map[string]string{"status": result.Status, "http_status": fmt.Sprint(result.HTTPStatusCode)}}
}

func goAgentSearchSource(tool string) tools.SearchSourceMetadata {
	return tools.SearchSourceMetadata{Caller: "go-agent", RequestID: "go-agent-" + tool, Description: "model-requested workspace search"}
}

func searchToolResult(result tools.SearchResult) goagent.ToolResult {
	if result.Error.Kind != "" && result.Error.Kind != tools.SearchErrorNone {
		return toolFailureResult(result.ToolName, result.ToolName+" failed: "+result.Error.Message, string(result.Error.Kind))
	}
	data, _ := json.Marshal(result.Matches)
	return goagent.ToolResult{Name: result.ToolName, Content: string(data), Truncated: result.Truncation.ResultLimitHit || result.Truncation.PreviewTruncated, Metadata: map[string]string{"status": "completed", "matches": fmt.Sprint(len(result.Matches))}}
}

func toolFailureResult(name string, content string, status string) goagent.ToolResult {
	return goagent.ToolResult{Name: name, Content: content, Metadata: map[string]string{"status": status}}
}
