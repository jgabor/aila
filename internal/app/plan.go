package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/policy"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

type planArtifactPersistence struct {
	Path       string
	Status     string
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) planRequestFromView() capability.Request {
	phase := workflowPhaseFromView(controller.view)
	if phase == workflow.PhaseIdle {
		phase = workflow.PhaseBuild
	}
	scope := strings.TrimSpace(controller.view.FooterContext)
	if scope == "" || scope == "placeholder" {
		scope = "current session work"
	}
	metadata := map[string]string{
		capability.PlanMetadataTitle:        "Current Session Plan",
		capability.PlanMetadataScope:        scope,
		capability.PlanMetadataProjectState: projectStateEvidence(controller.view),
		capability.PlanMetadataSessionState: sessionStateEvidence(controller.view),
		capability.PlanMetadataNextAction:   "Review the plan artifact, then choose the first pending item.",
	}
	return capability.Request{
		ID:         "command-plan",
		Capability: capability.NamePlan,
		Input:      scope,
		Phase:      phase,
		SourceRefs: []capability.SourceRef{
			{ID: "plan-scope", Kind: "context", Excerpt: scope},
			{ID: "plan-project-state", Kind: "project_state", Excerpt: metadata[capability.PlanMetadataProjectState]},
			{ID: "plan-session-state", Kind: "session_state", Excerpt: metadata[capability.PlanMetadataSessionState]},
		},
		Metadata: metadata,
	}
}

func (controller *sessionController) openPlanView() []diagnostic.Diagnostic {
	request := controller.planRequestFromView()
	turn := controller.runner.proposeCapability(request)
	persistence := controller.persistPlanPayload(controller.runner.model.LastCapability)
	turn.Plan = planView(controller.runner.model.LastCapability, request.Phase, persistence)
	if turn.Plan != nil {
		turn.StatusDetail = "plan capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model, controller.workspacePath)
	if persistence.Diagnostic == nil {
		return nil
	}
	return []diagnostic.Diagnostic{*persistence.Diagnostic}
}

func (controller *sessionController) persistPlanPayload(payload capability.ExitPayload) planArtifactPersistence {
	if payload.Plan == nil || strings.TrimSpace(payload.Plan.Document) == "" {
		return planArtifactPersistence{Status: "not_written"}
	}
	return writePlanArtifact(controller.ctx, controller.workspacePath, payload.Plan.Document)
}

func writePlanArtifact(ctx context.Context, workspacePath string, document string) planArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return planArtifactPersistence{Status: "recovery_needed", Diagnostic: planArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	artifact, err := store.WriteArtifact(ctx, state.ArtifactPlan, state.OwnerApp, []byte(document))
	if err != nil {
		return planArtifactPersistence{Status: "recovery_needed", Diagnostic: planArtifactDiagnostic(err)}
	}
	return planArtifactPersistence{Path: artifact.Path, Status: "written"}
}

func planArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "plan artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactPlan,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func planView(payload capability.ExitPayload, current workflow.Phase, persistence planArtifactPersistence) *tui.PlanView {
	if payload.Capability != capability.NamePlan || payload.Plan == nil {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	plan := payload.Plan
	artifactPath := plan.ArtifactPath
	if persistence.Path != "" {
		artifactPath = persistence.Path
	}
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	return &tui.PlanView{
		Source:               "app.plan",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		Title:                plan.Title,
		Scope:                plan.Scope,
		Summary:              payload.Summary,
		ArtifactPath:         artifactPath,
		ArtifactStatus:       artifactStatus,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		Items:                planItemViews(plan.Items),
		Blockers:             append([]string(nil), plan.Blockers...),
		NextAction:           payload.NextAction,
		ArtifactRefs:         planArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           planSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     planBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func projectStateEvidence(view tui.ViewState) string {
	return strings.Join(nonEmptyParts(valueOr(view.ProjectStoreStatus, "unknown"), valueOr(view.ProjectStoreDetail, "no detail")), " ")
}

func sessionStateEvidence(view tui.ViewState) string {
	return strings.Join(nonEmptyParts("runtime="+valueOr(view.RuntimeStatus, "unknown"), "phase="+valueOr(view.PhaseSource, "unknown"), "context="+valueOr(view.FooterContext, "unknown")), " ")
}

func planItemViews(items []capability.PlanItem) []tui.PlanItemView {
	views := make([]tui.PlanItemView, 0, len(items))
	for _, item := range items {
		views = append(views, tui.PlanItemView{ID: item.ID, Text: item.Text, Status: item.Status, Done: item.Done, Acceptance: append([]string(nil), item.Acceptance...), SourceRefIDs: append([]string(nil), item.SourceRefIDs...)})
	}
	return views
}

func planArtifactRefViews(refs []capability.ArtifactRef) []tui.PlanArtifactRefView {
	views := make([]tui.PlanArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.PlanArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func planSourceRefViews(refs []capability.SourceRef) []tui.PlanSourceRefView {
	views := make([]tui.PlanSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.PlanSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func planBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.PlanBoundaryRequestView {
	views := make([]tui.PlanBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.PlanBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}

func planStatusLines(plan *tui.PlanView) []string {
	if plan == nil {
		return nil
	}
	lines := []string{
		"plan capability: " + valueOr(plan.Capability, "plan") + " " + valueOr(plan.Signal, "complete"),
		"plan source: " + valueOr(plan.Source, "app.plan"),
		"plan title: " + valueOr(plan.Title, "unknown"),
		"plan scope: " + valueOr(plan.Scope, "unknown"),
		"plan artifact: " + valueOr(plan.ArtifactPath, "unknown"),
		"plan artifact status: " + valueOr(plan.ArtifactStatus, "unknown"),
		"plan successor valid: " + boolText(plan.SuccessorValid),
		"plan transition claimed: " + boolText(plan.TransitionClaimed),
		"plan display-only: " + boolText(plan.DisplayOnly),
	}
	for _, item := range plan.Items {
		lines = append(lines, "plan item: "+item.ID+" status="+valueOr(item.Status, "pending")+" done="+boolText(item.Done)+" text="+item.Text)
	}
	for _, blocker := range plan.Blockers {
		lines = append(lines, "plan blocker: "+blocker)
	}
	if plan.NextAction != "" {
		lines = append(lines, "plan next action: "+plan.NextAction)
	}
	for _, ref := range plan.ArtifactRefs {
		lines = append(lines, "plan artifact ref: "+ref.ID+" kind="+ref.Kind+" path="+ref.Path)
	}
	return lines
}
