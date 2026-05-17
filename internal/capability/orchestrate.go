package capability

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	OrchestrateMetadataGoalID       = "orchestrate_goal_id"
	OrchestrateMetadataGoalTitle    = "orchestrate_goal_title"
	OrchestrateMetadataGoalScope    = "orchestrate_goal_scope"
	OrchestrateMetadataStatus       = "orchestrate_status"
	OrchestrateMetadataActiveCycle  = "orchestrate_active_cycle"
	OrchestrateMetadataRetryBudget  = "orchestrate_retry_budget"
	OrchestrateMetadataRetryUsed    = "orchestrate_retry_used"
	OrchestrateMetadataRetryLeft    = "orchestrate_retry_left"
	OrchestrateMetadataBlockers     = "orchestrate_blockers"
	OrchestrateMetadataCaveats      = "orchestrate_caveats"
	OrchestrateMetadataFinalSummary = "orchestrate_final_summary"
	OrchestrateMetadataNextAction   = "orchestrate_next_action"
)

// OrchestrateCapability coordinates bounded child work from app-supplied evidence.
type OrchestrateCapability struct{}

// OrchestrateOutput is the typed conductor result carried by an orchestrate exit.
type OrchestrateOutput struct {
	Goal         OrchestrateGoal
	Status       string
	ActiveCycle  string
	RetryBudget  OrchestrateRetryBudget
	Cycles       []OrchestrateCycle
	ChildWork    []OrchestrateChildWork
	Decisions    []OrchestrateDecision
	Evidence     []OrchestrateEvidence
	Blockers     []string
	Caveats      []string
	FinalSummary string
	SourceRefs   []SourceRef
}

// OrchestrateGoal records the bounded plan or task being conducted.
type OrchestrateGoal struct {
	ID    string
	Title string
	Scope string
}

// OrchestrateRetryBudget records bounded retry accounting.
type OrchestrateRetryBudget struct {
	MaxAttempts int
	Used        int
	Remaining   int
}

// OrchestrateCycle records one visible orchestration cycle.
type OrchestrateCycle struct {
	ID             string
	Capability     Name
	Status         string
	Summary        string
	Evaluation     string
	RetryDecision  string
	RetryAttempt   int
	ChildWorkIDs   []string
	EvidenceRefIDs []string
}

// OrchestrateChildWork records one supervised child-work summary.
type OrchestrateChildWork struct {
	ID             string
	Capability     Name
	Purpose        string
	Status         string
	Summary        string
	RetryAttempt   int
	EvidenceRefIDs []string
}

// OrchestrateDecision records one conductor decision.
type OrchestrateDecision struct {
	ID          string
	Kind        string
	Summary     string
	Reason      string
	Result      string
	EvidenceRef string
}

// OrchestrateEvidence records one source-backed orchestration observation.
type OrchestrateEvidence struct {
	ID      string
	Kind    string
	Summary string
	RefID   string
}

// Name returns the fixed capability identity.
func (OrchestrateCapability) Name() Name {
	return NameOrchestrate
}

// OwningPhase returns BUILD because orchestration conducts build-phase work.
func (OrchestrateCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseBuild
}

// Run emits one bounded orchestration payload. App/runtime code owns child effects.
func (OrchestrateCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run orchestrate capability: %w", err)
	}
	request = normalizeOrchestrateRequest(request)
	invocation := NewInvocation(request)
	output := orchestrateOutput(request)
	signal := ExitComplete
	if len(output.Blockers) > 0 {
		signal = ExitFlagged
	}
	successor := workflow.Phase("")
	if signal == ExitComplete && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseAudit) == nil {
		successor = workflow.PhaseAudit
	} else if signal == ExitFlagged && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseBuild) == nil {
		successor = workflow.PhaseBuild
	}
	payload := ExitPayload{
		Capability:           NameOrchestrate,
		Signal:               signal,
		Summary:              orchestrateSummary(output, signal),
		Concerns:             append(append([]string(nil), output.Blockers...), output.Caveats...),
		Attempted:            len(output.Cycles) > 0,
		NextAction:           orchestrateNextAction(request, output, signal),
		RecommendedSuccessor: successor,
		ArtifactRefs: []ArtifactRef{
			{ID: "plan-artifact", Kind: "state_artifact", Path: ".aila/artifacts/plan.md"},
			{ID: "history-log", Kind: "state_event_log", Path: ".aila/history/fake-events.jsonl"},
		},
		SourceRefs:       cloneSourceRefs(output.SourceRefs),
		BoundaryRequests: orchestrateBoundaryRequests(request),
		Orchestrate:      &output,
	}
	return invocation.Emit(payload)
}

func normalizeOrchestrateRequest(request Request) Request {
	request.Capability = NameOrchestrate
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseBuild
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func orchestrateOutput(request Request) OrchestrateOutput {
	goal := OrchestrateGoal{
		ID:    orchestrateMetadata(request, OrchestrateMetadataGoalID, "current-plan"),
		Title: orchestrateMetadata(request, OrchestrateMetadataGoalTitle, "Coordinate the accepted plan"),
		Scope: orchestrateMetadata(request, OrchestrateMetadataGoalScope, "bounded existing capabilities and supervised subagents"),
	}
	budget := OrchestrateRetryBudget{
		MaxAttempts: orchestrateIntMetadata(request, OrchestrateMetadataRetryBudget, 1),
		Used:        orchestrateIntMetadata(request, OrchestrateMetadataRetryUsed, 1),
		Remaining:   orchestrateIntMetadata(request, OrchestrateMetadataRetryLeft, 0),
	}
	evidence := []OrchestrateEvidence{
		{ID: "cycle-1-evaluation", Kind: "evaluation", Summary: "first build child failed with retryable evidence gap", RefID: "orchestrate-build-1"},
		{ID: "cycle-2-evaluation", Kind: "evaluation", Summary: "retry completed and audit evidence accepted", RefID: "orchestrate-build-retry"},
	}
	childWork := []OrchestrateChildWork{
		{ID: "orchestrate-build-1", Capability: NameBuild, Purpose: "execute the first bounded plan step", Status: "failed", Summary: "build child reported missing verification evidence", RetryAttempt: 0, EvidenceRefIDs: []string{"cycle-1-evaluation"}},
		{ID: "orchestrate-build-retry", Capability: NameBuild, Purpose: "retry the bounded plan step with preserved evidence", Status: "completed", Summary: "retry completed after preserving evaluation evidence", RetryAttempt: 1, EvidenceRefIDs: []string{"cycle-2-evaluation"}},
		{ID: "orchestrate-audit", Capability: NameAudit, Purpose: "evaluate the recovered build result", Status: "completed", Summary: "audit accepted the recovered result", RetryAttempt: 0, EvidenceRefIDs: []string{"cycle-2-evaluation"}},
	}
	cycles := []OrchestrateCycle{
		{ID: "cycle-1", Capability: NameBuild, Status: "retrying", Summary: "dispatch build child and evaluate retryable failure", Evaluation: "missing verification evidence", RetryDecision: "retry build with preserved evidence", RetryAttempt: 0, ChildWorkIDs: []string{"orchestrate-build-1"}, EvidenceRefIDs: []string{"cycle-1-evaluation"}},
		{ID: "cycle-2", Capability: NameBuild, Status: "completed", Summary: "retry build child and request audit evaluation", Evaluation: "recovered result accepted", RetryDecision: "stop retries and summarize", RetryAttempt: 1, ChildWorkIDs: []string{"orchestrate-build-retry", "orchestrate-audit"}, EvidenceRefIDs: []string{"cycle-2-evaluation"}},
	}
	decisions := []OrchestrateDecision{
		{ID: "retry-build", Kind: "retry", Summary: "Retry the failed build child once.", Reason: "failure was retryable and retry budget remained", Result: "retry dispatched", EvidenceRef: "cycle-1-evaluation"},
		{ID: "final-audit", Kind: "evaluation", Summary: "Request audit evaluation after recovered build evidence.", Reason: "orchestrate must evaluate before final summary", Result: "audit accepted", EvidenceRef: "cycle-2-evaluation"},
	}
	blockers := orchestrateListMetadata(request, OrchestrateMetadataBlockers)
	caveats := orchestrateListMetadata(request, OrchestrateMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"deterministic app-supplied orchestration evidence only", "no provider-backed child execution in this slice"}
	}
	finalSummary := orchestrateMetadata(request, OrchestrateMetadataFinalSummary, "Coordinated two bounded cycles, retried one failed child, evaluated recovery, and stopped.")
	return OrchestrateOutput{
		Goal:         goal,
		Status:       orchestrateMetadata(request, OrchestrateMetadataStatus, "completed"),
		ActiveCycle:  orchestrateMetadata(request, OrchestrateMetadataActiveCycle, "cycle-2"),
		RetryBudget:  budget,
		Cycles:       cycles,
		ChildWork:    childWork,
		Decisions:    decisions,
		Evidence:     evidence,
		Blockers:     blockers,
		Caveats:      caveats,
		FinalSummary: finalSummary,
		SourceRefs:   cloneSourceRefs(request.SourceRefs),
	}
}

func orchestrateSummary(output OrchestrateOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return "Orchestration stopped with blockers for " + output.Goal.ID + "."
	}
	return fmt.Sprintf("Orchestration completed %d cycles for %s with %d retry used.", len(output.Cycles), output.Goal.ID, output.RetryBudget.Used)
}

func orchestrateNextAction(request Request, output OrchestrateOutput, signal ExitSignal) string {
	if next := orchestrateMetadata(request, OrchestrateMetadataNextAction, ""); next != "" {
		return next
	}
	if signal == ExitFlagged {
		return "Resolve orchestration blockers before another build cycle."
	}
	if output.FinalSummary != "" {
		return "Audit the orchestration summary before human-only dogfooding."
	}
	return "Review orchestration evidence."
}

func orchestrateBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("orchestration.current", "orchestrate requires app-supplied runtime and plan state"),
		request.RequestArtifactAccess("plan", "state resolver owns plan artifact access"),
		request.RequestToolExecution("capability.dispatch", "fixed built-in capabilities", "orchestrate dispatches only through existing capability/tool paths"),
		request.RequestPermissionCheck("child_work.allowed", "supervised subagents", "child work inherits explicit tool and permission constraints"),
		request.RequestContextAccess("current_context", "orchestrate uses app-supplied context evidence"),
	}
}

func orchestrateMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func orchestrateIntMetadata(request Request, key string, fallback int) int {
	value := orchestrateMetadata(request, key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func orchestrateListMetadata(request Request, key string) []string {
	value := orchestrateMetadata(request, key, "")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
