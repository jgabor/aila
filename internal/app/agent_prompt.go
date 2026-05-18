package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

const defaultInteractiveAgentMaxSteps = 16

func (runner *inputRunner) applyAgentState(turn *tui.TranscriptTurn) {
	if runner.model.AgentProvider != "" {
		turn.AssistantSource = runner.model.AgentProvider
	}
	if runner.model.AgentModel != "" {
		turn.AssistantModel = runner.model.AgentModel
	}
	if runner.model.LastAgentFailure.Code != "" {
		turn.Diagnostics = append(turn.Diagnostics, agentFailureDiagnosticView(runner.model.LastAgentFailure))
	}
}

type agentPromptRunner struct {
	ctx          context.Context
	runner       agent.Runner
	provider     string
	model        string
	toolNames    []string
	instructions string
	maxSteps     int
}

type agentPromptOptions struct {
	MaxSteps int
}

func newInputRunnerWithDispatchAndAgent(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner) *inputRunner {
	return newInputRunnerWithDispatchAndAgentConfig(ctx, dispatch, agentRunner, "fake", "fake-readonly", []string{"read"})
}

func newInputRunnerWithDispatchAndAgentConfig(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner, provider string, model string, toolNames []string) *inputRunner {
	return newInputRunnerWithDispatchAndAgentConfigAndInstructions(ctx, dispatch, agentRunner, provider, model, toolNames, "")
}

func newInputRunnerWithDispatchAndAgentConfigAndInstructions(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner, provider string, model string, toolNames []string, instructions string) *inputRunner {
	return newInputRunnerWithDispatchAndAgentOptions(ctx, dispatch, agentRunner, provider, model, toolNames, instructions, agentPromptOptions{})
}

func newInputRunnerWithDispatchAndAgentOptions(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner, provider string, model string, toolNames []string, instructions string, options agentPromptOptions) *inputRunner {
	base := newInputRunnerWithDispatch(dispatch)
	if ctx == nil {
		ctx = context.Background()
	}
	if agentRunner != nil {
		base.agent = &agentPromptRunner{
			ctx:          ctx,
			runner:       agentRunner,
			provider:     defaultString(provider, "fake"),
			model:        defaultString(model, "fake-readonly"),
			toolNames:    append([]string(nil), toolNames...),
			instructions: strings.TrimSpace(instructions),
			maxSteps:     normalizedAgentMaxSteps(options.MaxSteps),
		}
	}
	return base
}

func normalizedAgentMaxSteps(maxSteps int) int {
	if maxSteps > 0 {
		return maxSteps
	}
	return defaultInteractiveAgentMaxSteps
}

func (runner *inputRunner) submitAgentPrompt(text string) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	model, effects := runner.update(runtime.AgentPromptSubmitted{Text: text, Provider: runner.agent.provider, Model: runner.agent.model, ToolNames: runner.agent.toolNames})
	runner.model = model
	if len(effects) == 0 {
		turn := transcriptTurn(runner.model.Transcript[before:])
		runner.applyRuntimeState(&turn)
		return buildAgentEvidenceTurn(turn)
	}
	agentEffect, ok := effects[0].(runtime.AgentPromptEffect)
	if !ok {
		runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: effects[0].Metadata(), Provider: runner.agent.provider, Model: runner.agent.model, Failure: runtime.FailureMetadata{Code: "invalid_agent_effect", Message: "agent prompt did not produce an agent effect"}})
		turn := transcriptTurn(runner.model.Transcript[before:])
		runner.applyRuntimeState(&turn)
		return buildAgentEvidenceTurn(turn)
	}
	return runner.executeAgentPromptEffect(agentEffect, before)
}

func (runner *inputRunner) executeAgentPromptEffect(agentEffect runtime.AgentPromptEffect, before int) tui.TranscriptTurn {
	conversationContext := agentContextFromTranscript(runner.model.Transcript[:before])
	operation := agentEffect.Operation
	turnCtx, cancel := context.WithCancel(runner.agent.ctx)
	runner.activeAgentCancel = cancel
	defer func() {
		runner.activeAgentCancel = nil
		cancel()
	}()
	stream, err := runner.agent.runner.Stream(turnCtx, agent.RunRequest{
		Prompt:       strings.TrimSpace(agentEffect.Prompt),
		Instructions: runner.agent.instructions,
		Provider:     runner.agent.provider,
		Model:        runner.agent.model,
		SessionID:    "interactive-agent",
		Context:      conversationContext,
		RunID:        operation.ID,
		MaxSteps:     runner.agent.maxSteps,
		ToolNames:    append([]string(nil), agentEffect.ToolNames...),
	})
	if err != nil {
		runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: operation, Provider: runner.agent.provider, Model: runner.agent.model, Failure: runtime.FailureMetadata{Code: "stream_error", Message: err.Error(), Retryable: true}})
		turn := transcriptTurn(runner.model.Transcript[before:])
		runner.applyRuntimeState(&turn)
		runner.drainQueuedAgentPrompts()
		return buildAgentEvidenceTurn(turn)
	}

	var read *tui.ReadView
	for event := range stream {
		for _, message := range agent.AdaptEvents(operation, []agent.Event{event}) {
			if requested, ok := message.(runtime.AgentToolRequested); ok {
				runner.model, _ = runner.update(requested)
				switch requested.Request.Name {
				case "read":
					read = runner.executeAgentReadTool(requested.Request)
					continue
				case "find", "grep", "bash", "fetch":
					runner.executeAgentInspectionTool(requested.Request)
					continue
				case "edit", "write":
					cancel()
					return runner.proposeAgentWriteApproval(requested.Request, before)
				default:
					cancel()
					runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: operation, Provider: requested.Request.Provider, Model: requested.Request.Model, Failure: runtime.FailureMetadata{Code: "unsupported_tool", Message: unsupportedAgentToolMessage(requested.Request.Name)}})
					turn := transcriptTurn(runner.model.Transcript[before:])
					runner.applyRuntimeState(&turn)
					return buildAgentEvidenceTurn(turn)
				}
			}
			runner.model, _ = runner.update(message)
		}
	}
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	if read != nil {
		turn.Read = read
		turn.StatusDetail = "read tool dispatch"
	}
	runner.drainQueuedAgentPrompts()
	return buildAgentEvidenceTurn(turn)
}

func (runner *inputRunner) drainQueuedAgentPrompts() {
	for len(runner.model.Queued) > 0 && !runtimeActive(runner.model) {
		before := len(runner.model.Transcript)
		model, effects := runner.update(runtime.QueuedPromptDrainRequested{Provider: runner.agent.provider, Model: runner.agent.model, ToolNames: runner.agent.toolNames})
		runner.model = model
		if len(effects) == 0 {
			return
		}
		agentEffect, ok := effects[0].(runtime.AgentPromptEffect)
		if !ok {
			return
		}
		runner.executeAgentPromptEffect(agentEffect, before)
	}
}

func agentContextFromTranscript(transcript []runtime.TranscriptEntry) []agent.ContextMessage {
	if len(transcript) == 0 {
		return nil
	}
	context := make([]agent.ContextMessage, 0, len(transcript))
	for _, entry := range transcript {
		role, ok := agentContextRole(entry.Kind)
		if !ok {
			continue
		}
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}
		context = append(context, agent.ContextMessage{Role: role, Content: text})
	}
	return context
}

func agentContextRole(kind string) (string, bool) {
	switch kind {
	case "prompt":
		return "user", true
	case "tool_request", "tool":
		return "tool", true
	case "result", "paused", "failure":
		return "assistant", true
	default:
		return "", false
	}
}

func (runner *inputRunner) executeAgentInspectionTool(request runtime.AgentToolRequest) {
	operation := runtime.OperationMetadata{ID: defaultString(request.ID, "agent-"+request.Name), Kind: runtime.OperationRead, Subject: request.Name}
	var effect runtime.Effect
	sourceID := defaultString(request.ID, "agent-"+request.Name)
	switch request.Name {
	case "find":
		effect = runtime.SearchToolEffect{Operation: operation, Request: runtime.SearchToolRequest{ToolName: runtime.SearchToolFind, Pattern: agentToolArgument(request.Arguments, "pattern"), MaxResults: intAgentToolArgument(request.Arguments, "max_results"), MaxPreviewBytes: intAgentToolArgument(request.Arguments, "max_preview_bytes"), Source: runtime.SearchSourceMetadata{Caller: "interactive-agent", RequestID: sourceID, Description: "agent-requested find"}}}
	case "grep":
		effect = runtime.SearchToolEffect{Operation: operation, Request: runtime.SearchToolRequest{ToolName: runtime.SearchToolGrep, Query: agentToolArgument(request.Arguments, "query"), Regex: boolAgentToolArgument(request.Arguments, "regex"), IncludePattern: agentToolArgument(request.Arguments, "include_pattern"), MaxResults: intAgentToolArgument(request.Arguments, "max_results"), MaxPreviewBytes: intAgentToolArgument(request.Arguments, "max_preview_bytes"), Source: runtime.SearchSourceMetadata{Caller: "interactive-agent", RequestID: sourceID, Description: "agent-requested grep"}}}
	case "bash":
		effect = runtime.BashToolEffect{Operation: operation, Request: runtime.BashToolRequest{Argv: stringSliceAgentToolArgument(request.Arguments, "argv"), WorkingDir: agentToolArgument(request.Arguments, "working_dir"), MaxOutputBytes: intAgentToolArgument(request.Arguments, "max_output_bytes"), TimeoutMillis: intAgentToolArgument(request.Arguments, "timeout_millis"), Source: runtime.BashSourceMetadata{Caller: "interactive-agent", RequestID: sourceID, Description: "agent-requested bash"}}}
	case "fetch":
		effect = runtime.FetchToolEffect{Operation: operation, Request: runtime.FetchToolRequest{URL: agentToolArgument(request.Arguments, "url"), Method: agentToolArgument(request.Arguments, "method"), MaxPreviewBytes: intAgentToolArgument(request.Arguments, "max_preview_bytes"), TimeoutMillis: intAgentToolArgument(request.Arguments, "timeout_millis"), Source: runtime.FetchSourceMetadata{Caller: "interactive-agent", RequestID: sourceID, Description: "agent-requested fetch"}}}
	default:
		return
	}
	for _, message := range runner.dispatchEffects([]runtime.Effect{effect}) {
		runner.model, _ = runner.update(message)
	}
}

func (runner *inputRunner) executeAgentReadTool(request runtime.AgentToolRequest) *tui.ReadView {
	path := agentToolArgument(request.Arguments, "path")
	lineLimit := intAgentToolArgument(request.Arguments, "line_limit")
	operation := runtime.OperationMetadata{ID: defaultString(request.ID, "agent-read"), Kind: runtime.OperationRead, Subject: path}
	messages := runner.dispatchEffects([]runtime.Effect{runtime.ReadToolEffect{Operation: operation, Request: runtime.ReadToolRequest{Path: path, LineLimit: lineLimit}}})
	for _, message := range messages {
		if completed, ok := message.(runtime.ReadToolCompleted); ok {
			model := runtime.Model{LastRead: completed.Result}
			return readView(model)
		}
	}
	return nil
}

func (runner *inputRunner) proposeAgentWriteApproval(request runtime.AgentToolRequest, transcriptStart int) tui.TranscriptTurn {
	mutation := agentWriteMutationRequest(request)
	proposal := agentWriteApprovalProposal(request, mutation)
	runner.rememberMutationApproval(proposal.ID, mutation)
	runner.apply(runtime.ApprovalProposed{Proposal: proposal})
	turn := transcriptTurn(runner.model.Transcript[transcriptStart:])
	runner.applyRuntimeState(&turn)
	return buildAgentEvidenceTurn(turn)
}

func agentWriteMutationRequest(request runtime.AgentToolRequest) runtime.MutationToolRequest {
	path := agentToolArgument(request.Arguments, "path")
	content := agentToolArgument(request.Arguments, "content")
	expectedEffect := defaultString(agentToolArgument(request.Arguments, "expected_effect"), "write workspace file requested by agent")
	toolName := runtime.MutationToolWrite
	if request.Name == "edit" {
		toolName = runtime.MutationToolEdit
		expectedEffect = defaultString(agentToolArgument(request.Arguments, "expected_effect"), "edit workspace file requested by agent")
	}
	return runtime.MutationToolRequest{
		ToolName:       toolName,
		Path:           path,
		TargetVersion:  defaultString(agentToolArgument(request.Arguments, "target_version"), "missing"),
		OldText:        agentToolArgument(request.Arguments, "old_text"),
		NewText:        agentToolArgument(request.Arguments, "new_text"),
		Content:        content,
		ExpectedEffect: expectedEffect,
		Source: runtime.MutationSourceMetadata{
			Caller:      "interactive-agent",
			RequestID:   defaultString(request.ID, "agent-"+request.Name),
			Description: "approved interactive agent " + request.Name + " request",
		},
	}
}

func agentWriteApprovalProposal(request runtime.AgentToolRequest, mutation runtime.MutationToolRequest) runtime.ApprovalProposal {
	path := defaultString(mutation.Path, "workspace file")
	contentPreview := strings.TrimSpace(mutation.Content)
	if contentPreview == "" {
		contentPreview = "agent supplied empty content"
	}
	return runtime.ApprovalProposal{
		ID:             "approval-" + defaultString(request.ID, "agent-write"),
		OperationKind:  "mutation",
		Target:         path,
		RiskSummary:    "Agent requested a workspace write; approval is required before executing it.",
		Preview:        []string{"agent requested " + string(mutation.ToolName) + " tool", "approval dispatches an app-owned mutation effect"},
		DefaultAction:  runtime.ApprovalActionDeny,
		Path:           mutation.Path,
		Command:        []string{string(mutation.ToolName), path},
		WorkingDir:     ".",
		ExpectedEffect: mutation.ExpectedEffect,
		DiffPreview:    []string{"--- " + path, "+++ " + path, "@@", "+" + contentPreview},
		Reversible:     false,
		RunID:          request.ID,
		Capability:     "interactive-write-build",
	}
}

func buildAgentEvidenceTurn(turn tui.TranscriptTurn) tui.TranscriptTurn {
	turn.Phase = workflow.PhaseBuild.DisplayLabel()
	turn.PhaseSource = workflow.PhaseBuild.String()
	turn.SurfaceTitle = "agent evidence"
	return turn
}

func agentToolArgument(arguments []runtime.AgentToolArgument, name string) string {
	for _, argument := range arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	return ""
}

func intAgentToolArgument(arguments []runtime.AgentToolArgument, name string) int {
	value := strings.TrimSpace(agentToolArgument(arguments, name))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func boolAgentToolArgument(arguments []runtime.AgentToolArgument, name string) bool {
	return strings.EqualFold(strings.TrimSpace(agentToolArgument(arguments, name)), "true")
}

func stringSliceAgentToolArgument(arguments []runtime.AgentToolArgument, name string) []string {
	value := strings.TrimSpace(agentToolArgument(arguments, name))
	if value == "" {
		return nil
	}
	value = strings.Trim(value, "[]")
	return strings.Fields(value)
}

func unsupportedAgentToolMessage(name string) string {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80]) + "..."
	}
	return "agent tool not available: " + name
}

func agentFailureDiagnosticView(failure runtime.FailureMetadata) tui.DiagnosticView {
	return tui.DiagnosticView{
		Severity:         string(diagnostic.SeverityError),
		Source:           string(diagnostic.SourceProvider),
		RecoveryAction:   "check provider configuration",
		AffectedArtifact: string(diagnostic.ArtifactProviderRequest),
		UserInputNeeded:  failure.Code == "provider_auth_failed" || failure.Code == "model_unavailable",
		BoundedMessage:   failure.Code + ": " + failure.Message,
	}
}
