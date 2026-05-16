package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
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
	ctx    context.Context
	runner agent.Runner
}

func newInputRunnerWithDispatchAndAgent(ctx context.Context, dispatch runtimeDispatchFunc, agentRunner agent.Runner) *inputRunner {
	base := newInputRunnerWithDispatch(dispatch)
	if ctx == nil {
		ctx = context.Background()
	}
	if agentRunner != nil {
		base.agent = &agentPromptRunner{ctx: ctx, runner: agentRunner}
	}
	return base
}

func (runner *inputRunner) submitAgentPrompt(text string) tui.TranscriptTurn {
	before := len(runner.model.Transcript)
	model, effects := runner.update(runtime.PromptSubmitted{Text: text})
	runner.model = model
	if len(effects) == 0 {
		turn := transcriptTurn(runner.model.Transcript[before:])
		runner.applyRuntimeState(&turn)
		return turn
	}
	operation := effects[0].Metadata()
	stream, err := runner.agent.runner.Stream(runner.agent.ctx, agent.RunRequest{Prompt: strings.TrimSpace(text), Provider: "fake", Model: "fake-readonly", RunID: operation.ID, MaxSteps: 4, ToolNames: []string{"read"}})
	if err != nil {
		runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: operation, Provider: "fake", Model: "fake-readonly", Failure: runtime.FailureMetadata{Code: "stream_error", Message: err.Error(), Retryable: true}})
		turn := transcriptTurn(runner.model.Transcript[before:])
		runner.applyRuntimeState(&turn)
		return turn
	}

	var read *tui.ReadView
	for event := range stream {
		for _, message := range agent.AdaptEvents(operation, []agent.Event{event}) {
			if requested, ok := message.(runtime.AgentToolRequested); ok {
				runner.model, _ = runner.update(requested)
				if requested.Request.Name == "read" {
					read = runner.executeAgentReadTool(requested.Request)
					continue
				}
				runner.model, _ = runner.update(runtime.AgentTurnFailed{Operation: operation, Provider: requested.Request.Provider, Model: requested.Request.Model, Failure: runtime.FailureMetadata{Code: "unsupported_tool", Message: "read-only agent tool not available: " + requested.Request.Name}})
				continue
			}
			runner.model, _ = runner.update(message)
		}
	}
	turn := transcriptTurn(runner.model.Transcript[before:])
	runner.applyRuntimeState(&turn)
	if read != nil {
		turn.Read = read
	}
	return turn
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
