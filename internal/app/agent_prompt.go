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
	ctx       context.Context
	runner    agent.Runner
	provider  string
	model     string
	toolNames []string
}

func newInputRunnerWithDispatchAndAgent(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner) *inputRunner {
	return newInputRunnerWithDispatchAndAgentConfig(ctx, dispatch, agentRunner, "fake", "fake-readonly", []string{"read"})
}

func newInputRunnerWithDispatchAndAgentConfig(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner, provider string, model string, toolNames []string) *inputRunner {
	base := newInputRunnerWithDispatch(dispatch)
	if ctx == nil {
		ctx = context.Background()
	}
	if agentRunner != nil {
		base.agent = &agentPromptRunner{
			ctx:       ctx,
			runner:    agentRunner,
			provider:  defaultString(provider, "fake"),
			model:     defaultString(model, "fake-readonly"),
			toolNames: append([]string(nil), toolNames...),
		}
	}
	return base
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
	operation := agentEffect.Operation
	turnCtx, cancel := context.WithCancel(runner.agent.ctx)
	defer cancel()
	stream, err := runner.agent.runner.Stream(turnCtx, agent.RunRequest{
		Prompt:    strings.TrimSpace(text),
		Provider:  runner.agent.provider,
		Model:     runner.agent.model,
		RunID:     operation.ID,
		MaxSteps:  4,
		ToolNames: append([]string(nil), agentEffect.ToolNames...),
	})
	if err != nil {
		runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: operation, Provider: runner.agent.provider, Model: runner.agent.model, Failure: runtime.FailureMetadata{Code: "stream_error", Message: err.Error(), Retryable: true}})
		turn := transcriptTurn(runner.model.Transcript[before:])
		runner.applyRuntimeState(&turn)
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
				case "write":
					cancel()
					return runner.proposeAgentWriteApproval(requested.Request, before)
				default:
					runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: operation, Provider: requested.Request.Provider, Model: requested.Request.Model, Failure: runtime.FailureMetadata{Code: "unsupported_tool", Message: "agent tool not available: " + requested.Request.Name}})
					continue
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
	return buildAgentEvidenceTurn(turn)
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
	return runtime.MutationToolRequest{
		Path:           path,
		TargetVersion:  defaultString(agentToolArgument(request.Arguments, "target_version"), "missing"),
		Content:        content,
		ExpectedEffect: expectedEffect,
		Source: runtime.MutationSourceMetadata{
			Caller:      "interactive-agent",
			RequestID:   defaultString(request.ID, "agent-write"),
			Description: "approved interactive agent write request",
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
		Preview:        []string{"agent requested write tool", "approval dispatches an app-owned write effect"},
		DefaultAction:  runtime.ApprovalActionDeny,
		Path:           mutation.Path,
		Command:        []string{"write", path},
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
