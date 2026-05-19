package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/workflow"
)

const capabilityPromptContextBudget = 4096

func (runner *inputRunner) dispatchAgentOwnedEffects(effects []runtime.Effect) []runtime.Message {
	messages := make([]runtime.Message, 0, len(effects))
	var delegated []runtime.Effect
	flushDelegated := func() {
		if len(delegated) == 0 {
			return
		}
		messages = append(messages, runner.dispatch(delegated)...)
		delegated = nil
	}
	for _, effect := range effects {
		capabilityEffect, ok := effect.(runtime.CapabilityEffect)
		if !ok {
			delegated = append(delegated, effect)
			continue
		}
		flushDelegated()
		messages = append(messages, runner.dispatchCapabilityEffect(capabilityEffect)...)
	}
	flushDelegated()
	return messages
}

func (runner *inputRunner) dispatchCapabilityEffect(effect runtime.CapabilityEffect) []runtime.Message {
	execution := effect.Execution
	switch execution.Path {
	case capability.ExecutionPathWaiting:
		if execution.Waiting != nil {
			return []runtime.Message{runtime.CapabilityCompleted{Operation: effect.Operation, Payload: *execution.Waiting}}
		}
	case capability.ExecutionPathStuck:
		if execution.Stuck != nil {
			return []runtime.Message{runtime.CapabilityCompleted{Operation: effect.Operation, Payload: *execution.Stuck}}
		}
	case capability.ExecutionPathModelBacked:
		return runner.dispatchModelBackedCapability(effect)
	}
	return []runtime.Message{runtime.CapabilityCompleted{Operation: effect.Operation, Payload: capability.ExitPayload{
		Capability: effect.Request.Capability,
		Signal:     capability.ExitStuck,
		Blocker:    "capability execution did not include a runnable model boundary",
		Attempted:  false,
		NextAction: "Retry after checking the capability registration and runner setup.",
	}}}
}

func (runner *inputRunner) dispatchModelBackedCapability(effect runtime.CapabilityEffect) []runtime.Message {
	if runner.agent == nil || runner.agent.runner == nil {
		return []runtime.Message{runtime.CapabilityCompleted{Operation: effect.Operation, Payload: capability.ExitPayload{
			Capability: effect.Request.Capability,
			Signal:     capability.ExitStuck,
			Blocker:    "model-backed capability execution requires an app-owned agent runner",
			Attempted:  false,
			NextAction: "Configure a supported model provider or run the deterministic fallback path.",
		}}}
	}
	if usesDeterministicFakeRunner(runner.agent.runner) {
		payload, err := capability.RunBuiltIn(runner.agent.ctx, effect.Request)
		if err != nil {
			payload = capabilityFailurePayload(effect, "deterministic_fallback_failed", err.Error(), false, false)
		}
		return []runtime.Message{runtime.CapabilityCompleted{Operation: effect.Operation, Payload: payload}}
	}
	runRequest := runner.capabilityRunRequest(effect)
	ctx := runner.agent.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	stream, err := runner.agent.runner.Stream(ctx, runRequest)
	if err != nil {
		return []runtime.Message{runtime.CapabilityCompleted{Operation: effect.Operation, Payload: capabilityFailurePayload(effect, "stream_error", err.Error(), true, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))}}
	}
	messages := make([]runtime.Message, 0, 4)
	var text strings.Builder
	for event := range stream {
		switch event.Kind {
		case agent.EventAssistantDelta:
			text.WriteString(event.Text)
			messages = append(messages, runtime.CapabilityOutputDelta{Operation: effect.Operation, Capability: effect.Request.Capability, Sequence: event.Sequence, Text: event.Text})
		case agent.EventToolRequest:
			if payload, ok := runner.executeCapabilityToolRequest(effect, event); ok {
				messages = append(messages, runtime.CapabilityCompleted{Operation: effect.Operation, Payload: payload})
				return messages
			}
		case agent.EventError:
			messages = append(messages, runtime.CapabilityCompleted{Operation: effect.Operation, Payload: capabilityFailurePayload(effect, defaultString(event.Error.Code, "runner_error"), defaultString(event.Error.Message, event.Error.Code), true, false)})
			return messages
		}
		if err := ctx.Err(); err != nil {
			messages = append(messages, runtime.CapabilityCompleted{Operation: effect.Operation, Payload: capabilityFailurePayload(effect, "cancelled", err.Error(), true, true)})
			return messages
		}
	}
	summary := strings.TrimSpace(text.String())
	if summary == "" {
		summary = "Model-backed capability run completed."
	}
	if payload, parsed, malformed := modelBackedCapabilityPayload(effect, summary); parsed {
		messages = append(messages, runtime.CapabilityCompleted{Operation: effect.Operation, Payload: payload})
		return messages
	} else if malformed {
		messages = append(messages, runtime.CapabilityCompleted{Operation: effect.Operation, Payload: malformedCapabilityPayload(effect, summary)})
		return messages
	}
	messages = append(messages, runtime.CapabilityCompleted{Operation: effect.Operation, Payload: capability.ExitPayload{
		Capability:   effect.Request.Capability,
		Signal:       capability.ExitComplete,
		Summary:      summary,
		Attempted:    true,
		NextAction:   "Review the unstructured capability summary and rerun if a typed payload is required.",
		SourceRefs:   append([]capability.SourceRef(nil), effect.Request.SourceRefs...),
		ArtifactRefs: capabilityArtifactRefs(effect.Request.SourceRefs),
	}})
	return messages
}

func usesDeterministicFakeRunner(runner agent.Runner) bool {
	switch runner.(type) {
	case agent.FakeReadOnlyRunner, *agent.FakeReadOnlyRunner, agent.FakeBuildRunner, *agent.FakeBuildRunner:
		return true
	default:
		return false
	}
}

func modelBackedCapabilityPayload(effect runtime.CapabilityEffect, output string) (capability.ExitPayload, bool, bool) {
	jsonText, ok := extractJSONObject(output)
	if !ok {
		return capability.ExitPayload{}, false, strings.Contains(output, "{") || strings.Contains(output, "}")
	}
	var payload capability.ExitPayload
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return capability.ExitPayload{}, false, true
	}
	payload = normalizeModelBackedPayload(effect, payload, output)
	if err := capability.ValidateExitPayload(payload); err != nil {
		return capability.ExitPayload{}, false, true
	}
	return payload, true, false
}

func capabilityFailurePayload(effect runtime.CapabilityEffect, code, detail string, attempted bool, cancelled bool) capability.ExitPayload {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = strings.TrimSpace(code)
	}
	blocker := "capability model run failed"
	nextAction := "Check provider credentials and model availability, then retry the capability with the same scope."
	if cancelled {
		blocker = "capability model run cancelled"
		nextAction = "Start the capability again when you are ready; no capability-owned file or tool IO was performed."
	}
	if detail != "" {
		blocker += ": " + detail
	}
	return capability.ExitPayload{Capability: effect.Request.Capability, Signal: capability.ExitStuck, Summary: blocker, Blocker: blocker, Attempted: attempted, NextAction: nextAction, SourceRefs: append([]capability.SourceRef(nil), effect.Request.SourceRefs...), ArtifactRefs: capabilityArtifactRefs(effect.Request.SourceRefs), Concerns: []string{"recovery: " + nextAction}}
}

func malformedCapabilityPayload(effect runtime.CapabilityEffect, output string) capability.ExitPayload {
	summary := "Capability model output was malformed; typed exit payload was not accepted."
	return capability.ExitPayload{Capability: effect.Request.Capability, Signal: capability.ExitFlagged, Summary: summary, Concerns: []string{"model output could not be parsed as one valid ExitPayload", strings.TrimSpace(boundText(output, 240))}, Attempted: true, NextAction: "Retry the capability and ask for exactly one valid ExitPayload JSON object, or use the visible summary as provisional evidence.", SourceRefs: append([]capability.SourceRef(nil), effect.Request.SourceRefs...), ArtifactRefs: capabilityArtifactRefs(effect.Request.SourceRefs)}
}

func (runner *inputRunner) executeCapabilityToolRequest(effect runtime.CapabilityEffect, event agent.Event) (capability.ExitPayload, bool) {
	request := runtime.AgentToolRequest{ID: strings.TrimSpace(event.ToolCallID), Name: strings.TrimSpace(event.ToolName), Arguments: runtimeToolArgumentsForCapability(event.Arguments), Provider: event.Provider, Model: event.Model, Sequence: event.Sequence}
	if request.Name == "" {
		return unsupportedCapabilityToolPayload(effect, "missing tool name"), true
	}
	savedStatus := runner.model.Status
	savedActiveOperation := runner.model.ActiveOperation
	savedActiveCapability := runner.model.ActiveCapability
	sourceID := defaultString(request.ID, "capability-"+request.Name)
	operation := runtime.OperationMetadata{ID: sourceID, Kind: capabilityToolOperationKind(request.Name), Subject: request.Name, Source: "capability.tool"}
	effectToDispatch, ok := capabilityToolEffect(operation, effect.Request.Capability, request)
	if !ok {
		return unsupportedCapabilityToolPayload(effect, request.Name), true
	}
	for _, message := range runner.dispatch([]runtime.Effect{effectToDispatch}) {
		runner.model, _ = runner.update(message)
	}
	runner.model.Status = savedStatus
	runner.model.ActiveOperation = savedActiveOperation
	runner.model.ActiveCapability = savedActiveCapability
	return capability.ExitPayload{}, false
}

func capabilityToolEffect(operation runtime.OperationMetadata, name capability.Name, request runtime.AgentToolRequest) (runtime.Effect, bool) {
	sourceID := defaultString(request.ID, "capability-"+request.Name)
	caller := "capability:" + string(name)
	switch request.Name {
	case "read":
		return runtime.ReadToolEffect{Operation: operation, Request: runtime.ReadToolRequest{Path: agentToolArgument(request.Arguments, "path"), LineLimit: intAgentToolArgument(request.Arguments, "line_limit"), Source: runtime.ReadSourceMetadata{Caller: caller, RequestID: sourceID, Description: "capability-requested read"}}}, true
	case "find":
		return runtime.SearchToolEffect{Operation: operation, Request: runtime.SearchToolRequest{ToolName: runtime.SearchToolFind, Pattern: agentToolArgument(request.Arguments, "pattern"), MaxResults: intAgentToolArgument(request.Arguments, "max_results"), MaxPreviewBytes: intAgentToolArgument(request.Arguments, "max_preview_bytes"), Source: runtime.SearchSourceMetadata{Caller: caller, RequestID: sourceID, Description: "capability-requested find"}}}, true
	case "grep":
		return runtime.SearchToolEffect{Operation: operation, Request: runtime.SearchToolRequest{ToolName: runtime.SearchToolGrep, Query: agentToolArgument(request.Arguments, "query"), Regex: boolAgentToolArgument(request.Arguments, "regex"), IncludePattern: agentToolArgument(request.Arguments, "include_pattern"), MaxResults: intAgentToolArgument(request.Arguments, "max_results"), MaxPreviewBytes: intAgentToolArgument(request.Arguments, "max_preview_bytes"), Source: runtime.SearchSourceMetadata{Caller: caller, RequestID: sourceID, Description: "capability-requested grep"}}}, true
	case "bash":
		return runtime.BashToolEffect{Operation: operation, Request: runtime.BashToolRequest{Argv: stringSliceAgentToolArgument(request.Arguments, "argv"), WorkingDir: agentToolArgument(request.Arguments, "working_dir"), MaxOutputBytes: intAgentToolArgument(request.Arguments, "max_output_bytes"), TimeoutMillis: intAgentToolArgument(request.Arguments, "timeout_millis"), Source: runtime.BashSourceMetadata{Caller: caller, RequestID: sourceID, Description: "capability-requested bash"}}}, true
	case "fetch":
		return runtime.FetchToolEffect{Operation: operation, Request: runtime.FetchToolRequest{URL: agentToolArgument(request.Arguments, "url"), Method: agentToolArgument(request.Arguments, "method"), MaxPreviewBytes: intAgentToolArgument(request.Arguments, "max_preview_bytes"), TimeoutMillis: intAgentToolArgument(request.Arguments, "timeout_millis"), Source: runtime.FetchSourceMetadata{Caller: caller, RequestID: sourceID, Description: "capability-requested fetch"}}}, true
	}
	return nil, false
}

func capabilityToolOperationKind(name string) runtime.OperationKind {
	switch name {
	case "find":
		return runtime.OperationFind
	case "grep":
		return runtime.OperationGrep
	case "bash":
		return runtime.OperationBash
	case "fetch":
		return runtime.OperationFetch
	default:
		return runtime.OperationRead
	}
}

func unsupportedCapabilityToolPayload(effect runtime.CapabilityEffect, name string) capability.ExitPayload {
	return capability.ExitPayload{Capability: effect.Request.Capability, Signal: capability.ExitFlagged, Summary: "Capability model requested an unsupported or mutation tool; no capability-owned IO was performed.", Concerns: []string{"unsupported tool request: " + boundText(name, 80)}, Attempted: true, NextAction: "Retry with allowed inspection tools, or route requested mutation through explicit approval outside the capability run.", SourceRefs: append([]capability.SourceRef(nil), effect.Request.SourceRefs...), ArtifactRefs: capabilityArtifactRefs(effect.Request.SourceRefs)}
}

func runtimeToolArgumentsForCapability(arguments []agent.ToolArgument) []runtime.AgentToolArgument {
	if len(arguments) == 0 {
		return nil
	}
	out := make([]runtime.AgentToolArgument, 0, len(arguments))
	for _, argument := range arguments {
		out = append(out, runtime.AgentToolArgument{Name: argument.Name, Value: argument.Value})
	}
	return out
}

func boundText(value string, maxRunes int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if maxRunes <= 0 || len([]rune(value)) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes]) + "..."
}

func normalizeModelBackedPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload, output string) capability.ExitPayload {
	request := effect.Request
	if payload.Capability == "" {
		payload.Capability = request.Capability
	}
	if payload.Signal == "" {
		payload.Signal = capability.ExitComplete
	}
	if strings.TrimSpace(payload.Summary) == "" {
		payload.Summary = strings.TrimSpace(output)
	}
	payload.Attempted = true
	if len(payload.SourceRefs) == 0 {
		payload.SourceRefs = append([]capability.SourceRef(nil), request.SourceRefs...)
	}
	if len(payload.ArtifactRefs) == 0 {
		payload.ArtifactRefs = capabilityArtifactRefs(payload.SourceRefs)
	}
	payload.BoundaryRequests = normalizeCapabilityBoundaryRequests(request, payload)
	switch payload.Capability {
	case capability.NameVision:
		payload = normalizeModelVisionPayload(effect, payload)
	case capability.NameDiscuss:
		payload = normalizeModelDiscussPayload(effect, payload)
	case capability.NameResearch:
		payload = normalizeModelResearchPayload(payload)
	case capability.NamePlan:
		payload = normalizeModelPlanPayload(effect, payload)
	case capability.NameBuild:
		payload = normalizeModelBuildPayload(effect, payload)
	case capability.NameOptimize:
		payload = normalizeModelBuildOwnedPayload(effect, payload, payload.Optimize != nil, outputNextAction(payload.Optimize))
		if payload.Optimize != nil && len(payload.Optimize.SourceRefs) == 0 {
			payload.Optimize.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
		}
		if payload.Optimize != nil {
			payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, optimizeModelArtifactRefs(payload.Optimize)...)
		}
	case capability.NameDocument:
		payload = normalizeModelBuildOwnedPayload(effect, payload, payload.Document != nil, outputNextAction(payload.Document))
		if payload.Document != nil && len(payload.Document.SourceRefs) == 0 {
			payload.Document.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
		}
		if payload.Document != nil && strings.TrimSpace(payload.Document.DocumentArtifactPath) != "" {
			payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, capability.ArtifactRef{ID: "document-artifact", Kind: "state_artifact", Path: payload.Document.DocumentArtifactPath})
		}
	case capability.NameDesign:
		payload = normalizeModelBuildOwnedPayload(effect, payload, payload.Design != nil, outputNextAction(payload.Design))
		if payload.Design != nil && len(payload.Design.SourceRefs) == 0 {
			payload.Design.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
		}
		if payload.Design != nil && strings.TrimSpace(payload.Design.DesignArtifactPath) != "" {
			payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, capability.ArtifactRef{ID: "design-artifact", Kind: "state_artifact", Path: payload.Design.DesignArtifactPath})
		}
	case capability.NameAudit:
		payload = normalizeModelAuditPayload(effect, payload)
	case capability.NameProfile:
		payload = normalizeModelProfilePayload(payload)
	case capability.NameOrchestrate:
		payload = normalizeModelOrchestratePayload(effect, payload)
	case capability.NameBrief:
		payload.RecommendedSuccessor = ""
	}
	return payload
}

func normalizeModelVisionPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload) capability.ExitPayload {
	if payload.Vision == nil {
		return payload
	}
	if len(payload.Vision.SourceRefs) == 0 {
		payload.Vision.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = payload.Vision.NextAction
	}
	if payload.RecommendedSuccessor == "" {
		want := workflow.PhasePlan
		if payload.Signal == capability.ExitFlagged || len(payload.Vision.Blockers) > 0 {
			want = workflow.PhaseDeliberate
		}
		if workflowSuccessorValid(effect.Request.Phase, want) {
			payload.RecommendedSuccessor = want
		}
	}
	if strings.TrimSpace(payload.Vision.ArtifactPath) != "" {
		payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, capability.ArtifactRef{ID: "vision-artifact", Kind: "state_artifact", Path: payload.Vision.ArtifactPath})
	}
	return payload
}

func normalizeModelDiscussPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload) capability.ExitPayload {
	if payload.Discuss == nil {
		return payload
	}
	if len(payload.Discuss.SourceRefs) == 0 {
		payload.Discuss.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = payload.Discuss.NextAction
	}
	if payload.RecommendedSuccessor == "" {
		want := workflow.PhasePlan
		if payload.Signal == capability.ExitFlagged || len(payload.Discuss.Blockers) > 0 {
			want = workflow.PhaseEnvision
		}
		if workflowSuccessorValid(effect.Request.Phase, want) {
			payload.RecommendedSuccessor = want
		}
	}
	if strings.TrimSpace(payload.Discuss.ArtifactPath) != "" {
		payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, capability.ArtifactRef{ID: "decision-artifact", Kind: "state_artifact", Path: payload.Discuss.ArtifactPath})
	}
	return payload
}

func normalizeModelResearchPayload(payload capability.ExitPayload) capability.ExitPayload {
	payload.RecommendedSuccessor = ""
	if payload.Research == nil {
		return payload
	}
	if len(payload.Research.SourceRefs) == 0 {
		payload.Research.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = payload.Research.NextAction
	}
	return payload
}

func normalizeModelProfilePayload(payload capability.ExitPayload) capability.ExitPayload {
	payload.RecommendedSuccessor = ""
	if payload.Profile == nil {
		return payload
	}
	if len(payload.Profile.SourceRefs) == 0 {
		payload.Profile.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = payload.Profile.NextAction
	}
	if strings.TrimSpace(payload.Profile.ArtifactPath) != "" {
		payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, capability.ArtifactRef{ID: "profile-artifact", Kind: "state_artifact", Path: payload.Profile.ArtifactPath})
	}
	return payload
}

func normalizeModelOrchestratePayload(effect runtime.CapabilityEffect, payload capability.ExitPayload) capability.ExitPayload {
	if payload.Orchestrate == nil {
		return payload
	}
	if len(payload.Orchestrate.SourceRefs) == 0 {
		payload.Orchestrate.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = payload.Orchestrate.FinalSummary
	}
	blocked := payload.Signal == capability.ExitFlagged || len(payload.Orchestrate.Blockers) > 0
	if blocked {
		payload.Signal = capability.ExitFlagged
	}
	if payload.RecommendedSuccessor == "" {
		want := capabilityBuildSuccessor(blocked)
		if workflowSuccessorValid(effect.Request.Phase, want) {
			payload.RecommendedSuccessor = want
		}
	}
	return payload
}

func normalizeModelBuildOwnedPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload, hasOutput bool, nextAction string) capability.ExitPayload {
	if !hasOutput {
		return payload
	}
	blocked := payload.Signal == capability.ExitFlagged
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = nextAction
	}
	if payload.RecommendedSuccessor == "" {
		want := capabilityBuildSuccessor(blocked)
		if workflowSuccessorValid(effect.Request.Phase, want) {
			payload.RecommendedSuccessor = want
		}
	}
	return payload
}

func outputNextAction(output any) string {
	switch typed := output.(type) {
	case *capability.OptimizeOutput:
		if typed != nil {
			return typed.NextAction
		}
	case *capability.DocumentOutput:
		if typed != nil {
			return typed.NextAction
		}
	case *capability.DesignOutput:
		if typed != nil {
			return typed.NextAction
		}
	}
	return ""
}

func optimizeModelArtifactRefs(output *capability.OptimizeOutput) []capability.ArtifactRef {
	if output == nil {
		return nil
	}
	var refs []capability.ArtifactRef
	if strings.TrimSpace(output.ObjectiveArtifactPath) != "" {
		refs = append(refs, capability.ArtifactRef{ID: "objective-artifact", Kind: "state_artifact", Path: output.ObjectiveArtifactPath})
	}
	if strings.TrimSpace(output.ExperimentArtifactPath) != "" {
		refs = append(refs, capability.ArtifactRef{ID: "experiment-artifact", Kind: "state_artifact", Path: output.ExperimentArtifactPath})
	}
	return refs
}

func appendMissingArtifactRefs(refs []capability.ArtifactRef, additions ...capability.ArtifactRef) []capability.ArtifactRef {
	for _, addition := range additions {
		if strings.TrimSpace(addition.Path) == "" {
			continue
		}
		found := false
		for _, existing := range refs {
			if existing.Path == addition.Path {
				found = true
				break
			}
		}
		if !found {
			refs = append(refs, addition)
		}
	}
	return refs
}

func normalizeModelPlanPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload) capability.ExitPayload {
	if payload.Plan == nil {
		return payload
	}
	if len(payload.Plan.SourceRefs) == 0 {
		payload.Plan.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if strings.TrimSpace(payload.Plan.NextAction) == "" {
		payload.Plan.NextAction = valueOr(payload.NextAction, "Review the plan, then choose the first pending item.")
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		payload.NextAction = payload.Plan.NextAction
	}
	if payload.RecommendedSuccessor == "" && workflowSuccessorValid(effect.Request.Phase, capabilityPlanSuccessor()) {
		payload.RecommendedSuccessor = capabilityPlanSuccessor()
	}
	if strings.TrimSpace(payload.Plan.ArtifactPath) != "" {
		payload.ArtifactRefs = appendMissingArtifactRefs(payload.ArtifactRefs, capability.ArtifactRef{ID: "plan-artifact", Kind: "state_artifact", Path: payload.Plan.ArtifactPath})
	}
	return payload
}

func normalizeModelBuildPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload) capability.ExitPayload {
	if payload.Build == nil {
		return payload
	}
	if len(payload.Build.SourceRefs) == 0 {
		payload.Build.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	blocked := payload.Signal == capability.ExitFlagged || len(payload.Build.Blockers) > 0 || payload.Build.Tool.Status == "failed" || payload.Build.Tool.Status == "denied"
	if blocked {
		payload.Signal = capability.ExitFlagged
	}
	if strings.TrimSpace(payload.NextAction) == "" {
		if blocked {
			payload.NextAction = "Review blockers and caveats before running another build step."
		} else {
			payload.NextAction = "Audit the build result before continuing."
		}
	}
	want := capabilityBuildSuccessor(blocked)
	if payload.RecommendedSuccessor == "" && workflowSuccessorValid(effect.Request.Phase, want) {
		payload.RecommendedSuccessor = want
	}
	return payload
}

func normalizeModelAuditPayload(effect runtime.CapabilityEffect, payload capability.ExitPayload) capability.ExitPayload {
	if payload.Audit == nil {
		return payload
	}
	if len(payload.Audit.SourceRefs) == 0 {
		payload.Audit.SourceRefs = append([]capability.SourceRef(nil), payload.SourceRefs...)
	}
	if len(payload.Audit.NextActions) == 0 && strings.TrimSpace(payload.NextAction) != "" {
		payload.Audit.NextActions = []string{payload.NextAction}
	}
	if strings.TrimSpace(payload.NextAction) == "" && len(payload.Audit.NextActions) > 0 {
		payload.NextAction = payload.Audit.NextActions[0]
	}
	if payload.RecommendedSuccessor == "" && workflowSuccessorValid(effect.Request.Phase, capabilityBuildSuccessor(true)) {
		payload.RecommendedSuccessor = capabilityBuildSuccessor(true)
	}
	return payload
}

func normalizeCapabilityBoundaryRequests(request capability.Request, payload capability.ExitPayload) []capability.BoundaryRequest {
	if len(payload.BoundaryRequests) > 0 {
		return payload.BoundaryRequests
	}
	switch payload.Capability {
	case capability.NamePlan:
		return []capability.BoundaryRequest{request.RequestStateAccess("project.current", "plan used app-supplied project state evidence"), request.RequestContextAccess("current_context", "plan used bounded context evidence")}
	case capability.NameBuild:
		return []capability.BoundaryRequest{request.RequestStateAccess("plan.current", "build used selected plan item evidence"), request.RequestPermissionCheck("workspace.mutation", "planned build target", "build reports only app-owned mutation evidence")}
	case capability.NameAudit:
		return []capability.BoundaryRequest{request.RequestStateAccess("review.current", "audit used app-owned review evidence"), request.RequestContextAccess("current_context", "audit used supplied source refs and context evidence")}
	case capability.NameBrief:
		return []capability.BoundaryRequest{request.RequestStateAccess("runtime.current", "brief used app-supplied runtime evidence"), request.RequestContextAccess("current_context", "brief used supplied context evidence")}
	case capability.NameVision:
		return []capability.BoundaryRequest{request.RequestStateAccess("project.current", "vision used app-supplied project state evidence"), request.RequestStateAccess("session.current", "vision used app-supplied session state evidence"), request.RequestContextAccess("current_context", "vision used supplied context evidence"), request.RequestArtifactAccess("vision", "vision artifact access is app-owned"), request.RequestStateWrite("vision", "vision persistence is store-mediated")}
	case capability.NameDiscuss:
		return []capability.BoundaryRequest{request.RequestStateAccess("project.current", "discuss used app-supplied project state evidence"), request.RequestContextAccess("current_context", "discuss used supplied decision context"), request.RequestArtifactAccess("decisions", "decision artifact access is app-owned"), request.RequestStateWrite("decisions", "decision persistence is store-mediated")}
	case capability.NameResearch:
		return []capability.BoundaryRequest{request.RequestStateAccess("project.current", "research used app-supplied project state evidence"), request.RequestStateAccess("session.current", "research used app-supplied session state evidence"), request.RequestContextAccess("current_context", "research folded supplied evidence into context")}
	case capability.NameOptimize:
		return []capability.BoundaryRequest{request.RequestStateAccess("metrics.current", "optimize used app-supplied metric evidence"), request.RequestContextAccess("current_context", "optimize used supplied context evidence"), request.RequestPermissionCheck("workspace.mutation", "optimization artifacts", "optimize reports only app-owned mutation evidence")}
	case capability.NameDocument:
		return []capability.BoundaryRequest{request.RequestStateAccess("docs.current", "document used app-supplied documentation evidence"), request.RequestContextAccess("current_context", "document used supplied context evidence"), request.RequestPermissionCheck("workspace.mutation", "documentation target", "document reports only app-owned mutation evidence")}
	case capability.NameDesign:
		return []capability.BoundaryRequest{request.RequestStateAccess("design.current", "design used app-supplied design evidence"), request.RequestContextAccess("current_context", "design used supplied context evidence"), request.RequestArtifactAccess("design", "design artifact access is app-owned")}
	case capability.NameProfile:
		return []capability.BoundaryRequest{request.RequestStateAccess("profile.current", "profile used app-supplied decision evidence"), request.RequestContextAccess("current_context", "profile folded supplied evidence into context"), request.RequestStateWrite("profile", "profile persistence is store-mediated")}
	case capability.NameOrchestrate:
		return []capability.BoundaryRequest{request.RequestStateAccess("plan.current", "orchestrate used app-supplied plan state evidence"), request.RequestContextAccess("current_context", "orchestrate used supplied cycle evidence"), request.RequestPermissionCheck("workspace.mutation", "child work", "orchestrate reports only app-owned child work evidence")}
	}
	return nil
}

func extractJSONObject(text string) (string, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return "", false
	}
	return text[start : end+1], true
}

func capabilityPlanSuccessor() workflow.Phase { return workflow.PhaseBuild }

func capabilityBuildSuccessor(blocked bool) workflow.Phase {
	if blocked {
		return workflow.PhaseBuild
	}
	return workflow.PhaseAudit
}

func workflowSuccessorValid(from, to workflow.Phase) bool {
	return workflow.ValidateProtocolSuccessor(from, to) == nil
}

func (runner *inputRunner) capabilityRunRequest(effect runtime.CapabilityEffect) agent.RunRequest {
	context := effect.Execution.Context
	instructions := capabilityInstructions(runner.agent.instructions, effect.Execution.Contract)
	return agent.RunRequest{
		Prompt:                   capabilityPrompt(context, effect.Execution.Contract, runner.capabilityAutonomyBoundary()),
		Instructions:             instructions,
		Provider:                 runner.agent.provider,
		Model:                    runner.agent.model,
		SessionID:                "capability-" + string(context.Capability),
		RunID:                    effect.Operation.ID,
		MaxSteps:                 runner.agent.maxSteps,
		ToolNames:                append([]string(nil), runner.agent.toolNames...),
		DispatchToolsThroughHost: true,
	}
}

func capabilityInstructions(base string, contract capability.ExecutionContract) string {
	parts := []string{
		strings.TrimSpace(base),
		"You are running one fixed built-in Aila capability. Use only the supplied scope and labeled context; do not infer unavailable project or artifact state.",
		fmt.Sprintf("Capability: %s. Output field: %s.", contract.Capability, contract.OutputField),
		"Exit expectations: return exactly one of complete, flagged, waiting, or stuck. Use waiting/stuck when required context is missing instead of hallucinating state.",
		"When producing structured capability output, return one JSON object using the ExitPayload field names and include the required typed output field when evidence supports it.",
	}
	return strings.Join(nonEmptyStrings(parts), "\n\n")
}

func capabilityPrompt(context capability.BoundedRequestContext, contract capability.ExecutionContract, autonomyBoundary string) string {
	var builder strings.Builder
	writePromptSection(&builder, "Capability request", []string{
		"run_id: " + valueOr(context.RequestID, "missing"),
		"capability: " + string(context.Capability),
		"workflow_phase: " + valueOr(context.Phase.String(), "missing"),
		"autonomy_boundary: " + valueOr(autonomyBoundary, "unknown"),
		"user_requested_scope: " + valueOr(strings.TrimSpace(context.Input), "missing"),
	})
	writePromptSection(&builder, "Fixed capability exit expectations", []string{
		"required_output_field: " + contract.OutputField,
		"complete: work can be reported from supplied evidence.",
		"flagged: work can be reported but caveats or warnings remain.",
		"waiting: user input or artifact/context facts are missing.",
		"stuck: execution cannot continue safely from supplied evidence.",
	})
	writePromptSection(&builder, "Source-backed context", boundedContextLines(context))
	writePromptSection(&builder, "Missing-context facts", missingContextLines(context))
	return strings.TrimSpace(builder.String())
}

func writePromptSection(builder *strings.Builder, title string, lines []string) {
	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
}

func boundedContextLines(context capability.BoundedRequestContext) []string {
	var lines []string
	keys := make([]string, 0, len(context.Metadata))
	for key := range context.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(context.Metadata[key])
		if value != "" {
			lines = append(lines, fmt.Sprintf("metadata:%s: %s", key, value))
		}
	}
	refs := append([]capability.SourceRef(nil), context.SourceRefs...)
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	for _, ref := range refs {
		label := strings.Join(nonEmptyStrings([]string{ref.ID, ref.Kind, ref.Path, ref.Command}), " · ")
		if label == "" {
			label = "unlabeled source ref"
		}
		lines = append(lines, fmt.Sprintf("source_ref:%s: %s", label, strings.TrimSpace(ref.Excerpt)))
	}
	if len(lines) == 0 {
		return []string{"No project or artifact summaries were supplied."}
	}
	return boundLines(lines, capabilityPromptContextBudget)
}

func missingContextLines(context capability.BoundedRequestContext) []string {
	var missing []string
	if strings.TrimSpace(context.Input) == "" {
		missing = append(missing, "user_requested_scope is missing")
	}
	if context.Phase == "" {
		missing = append(missing, "current workflow phase is missing")
	}
	if len(context.SourceRefs) == 0 {
		missing = append(missing, "source refs for project/artifact evidence are missing")
	}
	if len(context.Metadata) == 0 {
		missing = append(missing, "project/artifact summaries are missing")
	}
	if len(missing) == 0 {
		return []string{"No missing context facts were reported by prompt assembly."}
	}
	return missing
}

func boundLines(lines []string, maxBytes int) []string {
	if maxBytes <= 0 {
		return lines
	}
	var out []string
	used := 0
	for _, line := range lines {
		if used+len(line) > maxBytes {
			out = append(out, "context truncated to prompt assembly budget")
			return out
		}
		out = append(out, line)
		used += len(line)
	}
	return out
}

func (runner *inputRunner) capabilityAutonomyBoundary() string {
	if runner == nil || runner.agent == nil {
		return "unknown"
	}
	if runner.agent.autonomyBoundary != "" {
		return runner.agent.autonomyBoundary
	}
	return "unknown"
}

func capabilityArtifactRefs(refs []capability.SourceRef) []capability.ArtifactRef {
	var artifacts []capability.ArtifactRef
	for _, ref := range refs {
		if strings.TrimSpace(ref.Path) == "" {
			continue
		}
		artifacts = append(artifacts, capability.ArtifactRef{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return artifacts
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
