package app

import (
	"os"
	"strconv"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/runtime"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

const orchestrateParentRunID = "orchestrate-command"

func (controller *sessionController) openOrchestrateView() []tui.DiagnosticView {
	request := controller.orchestrateRequestFromCurrentState(workflowPhaseFromView(controller.view))
	if os.Getenv("AILA_FAKE_ORCHESTRATE_HOLD_ACTIVE") == "1" {
		before := len(controller.runner.model.Transcript)
		controller.runner.apply(runtime.CapabilityProposed{Request: request})
		turn := transcriptTurn(controller.runner.model.Transcript[before:])
		controller.runner.applyRuntimeState(&turn)
		turn.Orchestrate = orchestrateRunningView(request)
		turn.StatusDetail = "orchestration capability status"
		controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
		return nil
	}

	controller.recordOrchestrateSubagentLifecycle(request)
	turn := controller.runner.proposeCapability(request)
	if turn.Orchestrate != nil {
		turn.StatusDetail = "orchestration capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	return nil
}

func (controller *sessionController) orchestrateRequestFromCurrentState(phase workflow.Phase) capability.Request {
	phase = normalizeOrchestratePhase(phase)
	goalID := "current-plan"
	goalTitle := "Coordinate the accepted plan"
	goalScope := "bounded existing capabilities and supervised subagents"
	if selection := selectBuildPlanItem(controller.view.Plan); selection.OK {
		goalID = defaultString(selection.ID, goalID)
		goalTitle = selection.Text
		goalScope = "plan item " + defaultString(selection.ID, "current-plan")
	}
	metadata := map[string]string{
		capability.OrchestrateMetadataGoalID:       goalID,
		capability.OrchestrateMetadataGoalTitle:    goalTitle,
		capability.OrchestrateMetadataGoalScope:    goalScope,
		capability.OrchestrateMetadataStatus:       "completed",
		capability.OrchestrateMetadataActiveCycle:  "cycle-2",
		capability.OrchestrateMetadataRetryBudget:  "1",
		capability.OrchestrateMetadataRetryUsed:    "1",
		capability.OrchestrateMetadataRetryLeft:    "0",
		capability.OrchestrateMetadataCaveats:      "deterministic app-supplied orchestration evidence only|no provider-backed child execution in this slice|orchestrate does not mutate workflow phase directly",
		capability.OrchestrateMetadataFinalSummary: "Coordinated two bounded cycles, retried one failed child, evaluated recovery, and stopped.",
		capability.OrchestrateMetadataNextAction:   "Audit the orchestration summary before human-only dogfooding.",
	}
	return capability.Request{
		ID:         "command-orchestrate",
		Capability: capability.NameOrchestrate,
		Input:      goalTitle,
		Phase:      phase,
		SourceRefs: []capability.SourceRef{
			{ID: "orchestrate-command", Kind: "command", Command: "/orchestrate", Excerpt: "app-owned orchestration command"},
			{ID: "orchestrate-workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", Excerpt: "orchestrate is BUILD-owned bounded coordination with visible evaluation"},
			{ID: "orchestrate-runtime-doc", Kind: "architecture", Path: "ARCHITECTURE.md", Excerpt: "subagents and orchestration stay behind runtime supervision and effects"},
		},
		Metadata: metadata,
	}
}

func normalizeOrchestratePhase(phase workflow.Phase) workflow.Phase {
	if phase == "" || phase == workflow.PhaseIdle {
		return workflow.PhaseBuild
	}
	return phase
}

func (controller *sessionController) recordOrchestrateSubagentLifecycle(request capability.Request) {
	controller.runner.apply(runtime.SubagentSpawnProposed{Request: orchestrateBuildSubagentRequest(request, "orchestrate-build-1", "execute the first bounded plan step", 0)})
	controller.runner.apply(runtime.SubagentFailed{
		ID:          "orchestrate-build-1",
		ParentRunID: orchestrateParentRunID,
		Purpose:     "execute the first bounded plan step",
		Failure:     runtime.FailureMetadata{Code: "verification_missing", Message: "missing verification evidence", Retryable: true},
		Summary:     "build child reported missing verification evidence",
		EvidenceLinks: []runtime.SubagentEvidenceLink{
			{ID: "cycle-1-evaluation", Kind: "evaluation", Excerpt: "first build child failed with retryable evidence gap"},
		},
	})
	controller.runner.apply(runtime.SubagentSpawnProposed{Request: orchestrateBuildSubagentRequest(request, "orchestrate-build-retry", "retry the bounded plan step with preserved evidence", 1)})
	controller.runner.apply(runtime.SubagentCompleted{ParentRunID: orchestrateParentRunID, Result: runtime.SubagentResult{
		ID:          "orchestrate-build-retry",
		ParentRunID: orchestrateParentRunID,
		Purpose:     "retry the bounded plan step with preserved evidence",
		Summary:     "retry completed after preserving evaluation evidence",
		EvidenceLinks: []runtime.SubagentEvidenceLink{
			{ID: "cycle-2-evaluation", Kind: "evaluation", Excerpt: "retry completed and audit evidence accepted"},
		},
	}})
	controller.runner.apply(runtime.SubagentSpawnProposed{Request: runtime.SubagentRequest{
		ID:          "orchestrate-audit",
		ParentRunID: orchestrateParentRunID,
		Purpose:     "evaluate the recovered build result",
		Input:       "Audit recovered orchestration evidence for " + valueOr(request.Input, "current plan"),
		Tools:       []string{string(capability.NameAudit)},
		Budget:      runtime.SubagentBudget{MaxTurns: 2, MaxTokens: 900, TimeoutMillis: 3000},
		EvidenceLinks: []runtime.SubagentEvidenceLink{
			{ID: "cycle-2-evaluation", Kind: "evaluation", Excerpt: "retry completed and audit evidence accepted"},
		},
		Source: runtime.SubagentSourceMetadata{Caller: string(capability.NameOrchestrate), RequestID: request.ID, Description: "bounded orchestration audit child work"},
	}})
	controller.runner.apply(runtime.SubagentCompleted{ParentRunID: orchestrateParentRunID, Result: runtime.SubagentResult{
		ID:          "orchestrate-audit",
		ParentRunID: orchestrateParentRunID,
		Purpose:     "evaluate the recovered build result",
		Summary:     "audit accepted the recovered result",
		EvidenceLinks: []runtime.SubagentEvidenceLink{
			{ID: "cycle-2-evaluation", Kind: "evaluation", Excerpt: "audit accepted recovered orchestration evidence"},
		},
	}})
}

func orchestrateBuildSubagentRequest(request capability.Request, id string, purpose string, retry int) runtime.SubagentRequest {
	return runtime.SubagentRequest{
		ID:          id,
		ParentRunID: orchestrateParentRunID,
		Purpose:     purpose,
		Input:       valueOr(request.Input, "current plan"),
		Tools:       []string{string(capability.NameBuild)},
		Budget:      runtime.SubagentBudget{MaxTurns: 3, MaxTokens: 1200, TimeoutMillis: 5000},
		EvidenceLinks: []runtime.SubagentEvidenceLink{
			{ID: "cycle-1-evaluation", Kind: "evaluation", Excerpt: "orchestration child work retry " + strconv.Itoa(retry)},
		},
		Source: runtime.SubagentSourceMetadata{Caller: string(capability.NameOrchestrate), RequestID: request.ID, Description: "bounded orchestration build child work"},
	}
}

func orchestrateView(payload capability.ExitPayload, current workflow.Phase) *tui.OrchestrateView {
	if payload.Capability != capability.NameOrchestrate || payload.Orchestrate == nil {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	output := payload.Orchestrate
	return &tui.OrchestrateView{
		Source:               "app.orchestrate",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		CurrentPhase:         normalizeOrchestratePhase(current).String(),
		Status:               output.Status,
		ActiveCycle:          output.ActiveCycle,
		Summary:              payload.Summary,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		Goal:                 tui.OrchestrateGoalView{ID: output.Goal.ID, Title: output.Goal.Title, Scope: output.Goal.Scope},
		RetryBudget:          tui.OrchestrateRetryBudgetView{MaxAttempts: output.RetryBudget.MaxAttempts, Used: output.RetryBudget.Used, Remaining: output.RetryBudget.Remaining},
		Cycles:               orchestrateCycleViews(output.Cycles),
		ChildWork:            orchestrateChildWorkViews(output.ChildWork),
		Decisions:            orchestrateDecisionViews(output.Decisions),
		Evidence:             orchestrateEvidenceViews(output.Evidence),
		Blockers:             append([]string(nil), output.Blockers...),
		Caveats:              append([]string(nil), output.Caveats...),
		FinalSummary:         output.FinalSummary,
		NeededInput:          payload.NeededInput,
		NextAction:           payload.NextAction,
		ArtifactRefs:         orchestrateArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           orchestrateSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     orchestrateBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func orchestrateRunningView(request capability.Request) *tui.OrchestrateView {
	return &tui.OrchestrateView{
		Source:               "app.orchestrate",
		Capability:           string(capability.NameOrchestrate),
		Signal:               string(capability.ExitWaiting),
		CurrentPhase:         normalizeOrchestratePhase(request.Phase).String(),
		Status:               "running",
		ActiveCycle:          "cycle-1",
		Summary:              "Orchestration is running bounded child work for " + valueOr(request.Metadata[capability.OrchestrateMetadataGoalID], "current-plan") + ".",
		RecommendedSuccessor: "",
		SuccessorValid:       false,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		Goal: tui.OrchestrateGoalView{
			ID:    valueOr(request.Metadata[capability.OrchestrateMetadataGoalID], "current-plan"),
			Title: valueOr(request.Metadata[capability.OrchestrateMetadataGoalTitle], "Coordinate the accepted plan"),
			Scope: valueOr(request.Metadata[capability.OrchestrateMetadataGoalScope], "bounded existing capabilities and supervised subagents"),
		},
		RetryBudget: tui.OrchestrateRetryBudgetView{MaxAttempts: 1, Used: 0, Remaining: 1},
		Cycles: []tui.OrchestrateCycleView{{
			ID:             "cycle-1",
			Capability:     string(capability.NameBuild),
			Status:         "running",
			Summary:        "dispatch build child and wait for evaluation",
			RetryAttempt:   0,
			ChildWorkIDs:   []string{"orchestrate-build-1"},
			EvidenceRefIDs: []string{"cycle-1-evaluation"},
		}},
		ChildWork: []tui.OrchestrateChildWorkView{{
			ID:             "orchestrate-build-1",
			Capability:     string(capability.NameBuild),
			Purpose:        "execute the first bounded plan step",
			Status:         "running",
			Summary:        "spawn requested: execute the first bounded plan step",
			RetryAttempt:   0,
			EvidenceRefIDs: []string{"cycle-1-evaluation"},
		}},
		Caveats:    []string{"fake runtime hold keeps orchestration visible for cancellation smoke", "no provider-backed child execution in this slice"},
		NextAction: "Cancel or wait for orchestration to finish.",
		SourceRefs: []tui.OrchestrateSourceRefView{{ID: "orchestrate-command", Kind: "command", Command: "/orchestrate", Excerpt: "app-owned orchestration command"}},
	}
}

func orchestrateCycleViews(cycles []capability.OrchestrateCycle) []tui.OrchestrateCycleView {
	views := make([]tui.OrchestrateCycleView, 0, len(cycles))
	for _, cycle := range cycles {
		views = append(views, tui.OrchestrateCycleView{ID: cycle.ID, Capability: string(cycle.Capability), Status: cycle.Status, Summary: cycle.Summary, Evaluation: cycle.Evaluation, RetryDecision: cycle.RetryDecision, RetryAttempt: cycle.RetryAttempt, ChildWorkIDs: append([]string(nil), cycle.ChildWorkIDs...), EvidenceRefIDs: append([]string(nil), cycle.EvidenceRefIDs...)})
	}
	return views
}

func orchestrateChildWorkViews(children []capability.OrchestrateChildWork) []tui.OrchestrateChildWorkView {
	views := make([]tui.OrchestrateChildWorkView, 0, len(children))
	for _, child := range children {
		views = append(views, tui.OrchestrateChildWorkView{ID: child.ID, Capability: string(child.Capability), Purpose: child.Purpose, Status: child.Status, Summary: child.Summary, RetryAttempt: child.RetryAttempt, EvidenceRefIDs: append([]string(nil), child.EvidenceRefIDs...)})
	}
	return views
}

func orchestrateDecisionViews(decisions []capability.OrchestrateDecision) []tui.OrchestrateDecisionView {
	views := make([]tui.OrchestrateDecisionView, 0, len(decisions))
	for _, decision := range decisions {
		views = append(views, tui.OrchestrateDecisionView{ID: decision.ID, Kind: decision.Kind, Summary: decision.Summary, Reason: decision.Reason, Result: decision.Result, EvidenceRef: decision.EvidenceRef})
	}
	return views
}

func orchestrateEvidenceViews(evidence []capability.OrchestrateEvidence) []tui.OrchestrateEvidenceView {
	views := make([]tui.OrchestrateEvidenceView, 0, len(evidence))
	for _, item := range evidence {
		views = append(views, tui.OrchestrateEvidenceView{ID: item.ID, Kind: item.Kind, Summary: item.Summary, RefID: item.RefID})
	}
	return views
}

func orchestrateArtifactRefViews(refs []capability.ArtifactRef) []tui.OrchestrateArtifactRefView {
	views := make([]tui.OrchestrateArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.OrchestrateArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func orchestrateSourceRefViews(refs []capability.SourceRef) []tui.OrchestrateSourceRefView {
	views := make([]tui.OrchestrateSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.OrchestrateSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func orchestrateBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.OrchestrateBoundaryRequestView {
	views := make([]tui.OrchestrateBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.OrchestrateBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
