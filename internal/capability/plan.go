package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	PlanMetadataTitle        = "plan_title"
	PlanMetadataScope        = "plan_scope"
	PlanMetadataProjectState = "project_state"
	PlanMetadataSessionState = "session_state"
	PlanMetadataArtifactPath = "plan_artifact_path"
	PlanMetadataNextAction   = "next_action"
)

const defaultPlanArtifactPath = ".aila/artifacts/plan.md"

// PlanCapability creates scoped work without executing build or audit behavior.
type PlanCapability struct{}

// PlanOutput is the typed plan data carried by a plan capability exit.
type PlanOutput struct {
	Title        string
	Scope        string
	ArtifactPath string
	Items        []PlanItem
	Blockers     []string
	NextAction   string
	Document     string
	SourceRefs   []SourceRef
}

// PlanItem records one scoped plan step and its behavioral acceptance criteria.
type PlanItem struct {
	ID           string
	Text         string
	Status       string
	Done         bool
	Acceptance   []string
	SourceRefIDs []string
}

// Name returns the fixed capability identity.
func (PlanCapability) Name() Name {
	return NamePlan
}

// OwningPhase returns PLAN because the capability creates scoped work.
func (PlanCapability) OwningPhase() workflow.Phase {
	return workflow.PhasePlan
}

// Run emits one plan payload without reading files, executing tools, or changing workflow phase.
func (PlanCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run plan capability: %w", err)
	}
	request = normalizePlanRequest(request)
	invocation := NewInvocation(request)

	if planScope(request) == "" {
		payload := ExitPayload{
			Capability:       NamePlan,
			Signal:           ExitWaiting,
			Summary:          "Plan needs a scope before it can create scoped work.",
			NeededInput:      "Provide the work scope or accepted milestone to plan.",
			NextAction:       "Provide the plan scope, then run plan again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: planBoundaryRequests(request),
		}
		return invocation.Emit(payload)
	}

	plan := buildPlanOutput(request)
	signal := ExitComplete
	if len(plan.Blockers) > 0 {
		signal = ExitFlagged
	}

	var successor workflow.Phase
	if signal == ExitComplete && workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhasePlan) == nil {
		successor = workflow.PhasePlan
	}

	payload := ExitPayload{
		Capability:           NamePlan,
		Signal:               signal,
		Summary:              planSummary(plan, signal),
		Concerns:             append([]string(nil), plan.Blockers...),
		Attempted:            true,
		NextAction:           plan.NextAction,
		RecommendedSuccessor: successor,
		ArtifactRefs: []ArtifactRef{{
			ID:   "plan-artifact",
			Kind: "state_artifact",
			Path: plan.ArtifactPath,
		}},
		SourceRefs:       cloneSourceRefs(plan.SourceRefs),
		BoundaryRequests: planBoundaryRequests(request),
		Plan:             &plan,
	}
	return invocation.Emit(payload)
}

func normalizePlanRequest(request Request) Request {
	request.Capability = NamePlan
	if request.Phase == "" {
		request.Phase = workflow.PhaseBuild
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func buildPlanOutput(request Request) PlanOutput {
	scope := planScope(request)
	title := planMetadata(request, PlanMetadataTitle, "Plan: "+scope)
	artifactPath := planMetadata(request, PlanMetadataArtifactPath, defaultPlanArtifactPath)
	projectState := planMetadata(request, PlanMetadataProjectState, "")
	sessionState := planMetadata(request, PlanMetadataSessionState, "")

	blockers := planBlockers(projectState, sessionState)
	items := planItems(scope, len(blockers) == 0)
	nextAction := planMetadata(request, PlanMetadataNextAction, defaultPlanNextAction(blockers))
	sourceRefs := planSourceRefs(request)
	plan := PlanOutput{
		Title:        title,
		Scope:        scope,
		ArtifactPath: artifactPath,
		Items:        items,
		Blockers:     blockers,
		NextAction:   nextAction,
		SourceRefs:   sourceRefs,
	}
	plan.Document = renderPlanDocument(plan)
	return plan
}

func planScope(request Request) string {
	if value := planMetadata(request, PlanMetadataScope, ""); value != "" {
		return value
	}
	return strings.TrimSpace(request.Input)
}

func planItems(scope string, evidenceReady bool) []PlanItem {
	items := []PlanItem{
		{
			ID:     "scope",
			Text:   "Confirm the scoped work and evidence for " + scope,
			Status: "pending",
			Acceptance: []string{
				"GIVEN the work is planned WHEN the plan is displayed THEN scope, source refs, blockers, and next action are visible before execution.",
				"GIVEN the plan artifact is written WHEN state persistence succeeds THEN the artifact is store-resolved and app-owned.",
			},
			SourceRefIDs: []string{"plan-scope", "plan-project-state"},
		},
		{
			ID:     "implement",
			Text:   "Implement only the scoped plan behavior",
			Status: "pending",
			Acceptance: []string{
				"GIVEN implementation starts WHEN code changes are made THEN build execution, audit behavior, provider calls, and tool execution remain out of scope.",
				"GIVEN capability output completes WHEN the runtime records it THEN exactly one exit payload is emitted.",
			},
			SourceRefIDs: []string{"plan-scope"},
		},
		{
			ID:     "validate",
			Text:   "Validate display and semantic evidence before execution",
			Status: "pending",
			Acceptance: []string{
				"GIVEN validation runs WHEN fixtures and semantic snapshots are inspected THEN plan items, done state, blockers, artifact refs, and source refs are machine-readable.",
				"GIVEN the plan recommends a successor WHEN the FSM validator checks it THEN only a valid PLAN recommendation is surfaced.",
			},
			SourceRefIDs: []string{"plan-session-state"},
		},
	}
	if evidenceReady {
		items[0].Status = "done"
		items[0].Done = true
	}
	return items
}

func planBlockers(projectState, sessionState string) []string {
	var blockers []string
	if strings.TrimSpace(projectState) == "" {
		blockers = append(blockers, "project state evidence missing")
	}
	if strings.TrimSpace(sessionState) == "" {
		blockers = append(blockers, "session state evidence missing")
	}
	return blockers
}

func defaultPlanNextAction(blockers []string) string {
	if len(blockers) > 0 {
		return "Provide missing project/session evidence before executing the plan."
	}
	return "Review the plan artifact, then choose the first pending item."
}

func planSummary(plan PlanOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return fmt.Sprintf("Plan: %s has %d blocker(s) before execution.", plan.Scope, len(plan.Blockers))
	}
	return fmt.Sprintf("Plan: %s with %d item(s) ready for review.", plan.Scope, len(plan.Items))
}

func renderPlanDocument(plan PlanOutput) string {
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(plan.Title)
	builder.WriteString("\n\n")
	builder.WriteString("Scope: ")
	builder.WriteString(plan.Scope)
	builder.WriteString("\n\n")
	if len(plan.Blockers) > 0 {
		builder.WriteString("## Blockers\n")
		for _, blocker := range plan.Blockers {
			builder.WriteString("- ")
			builder.WriteString(blocker)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## Items\n")
	for _, item := range plan.Items {
		marker := "[ ]"
		if item.Done {
			marker = "[x]"
		}
		builder.WriteString("- ")
		builder.WriteString(marker)
		builder.WriteString(" ")
		builder.WriteString(item.Text)
		builder.WriteString("\n")
		for _, acceptance := range item.Acceptance {
			builder.WriteString("  - ")
			builder.WriteString(acceptance)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nNext action: ")
	builder.WriteString(plan.NextAction)
	builder.WriteString("\n")
	return builder.String()
}

func planBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("project.current", "plan requires app-supplied project state evidence"),
		request.RequestStateAccess("session.current", "plan requires app-supplied session state evidence"),
		request.RequestContextAccess("current_context", "plan uses bounded context evidence supplied by the app"),
		request.RequestArtifactAccess("plan", "plan artifact path must be resolved through the state store"),
		request.RequestStateWrite("plan", "plan artifact persistence must be app-owned and store-mediated"),
	}
}

func planSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	if len(refs) == 0 {
		refs = append(refs, SourceRef{ID: "plan-scope", Kind: "prompt", Excerpt: planScope(request)})
	}
	ensureRef := func(id, kind, excerpt string) {
		if strings.TrimSpace(excerpt) == "" || hasSourceRef(refs, id) {
			return
		}
		refs = append(refs, SourceRef{ID: id, Kind: kind, Excerpt: excerpt})
	}
	ensureRef("plan-project-state", "project_state", request.Metadata[PlanMetadataProjectState])
	ensureRef("plan-session-state", "session_state", request.Metadata[PlanMetadataSessionState])
	return refs
}

func hasSourceRef(refs []SourceRef, id string) bool {
	for _, ref := range refs {
		if ref.ID == id {
			return true
		}
	}
	return false
}

func planMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func cloneSourceRefs(refs []SourceRef) []SourceRef {
	return append([]SourceRef(nil), refs...)
}
