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

type designArtifactPersistence struct {
	Path       string
	Status     string
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) openDesignView() []tui.DiagnosticView {
	request := controller.designRequestFromCurrentState(workflowPhaseFromView(controller.view))
	turn := controller.runner.proposeCapability(request)
	persistence := controller.persistDesignPayload(controller.runner.model.LastCapability)
	turn.Design = designView(controller.runner.model.LastCapability, request.Phase, persistence)
	if turn.Design != nil {
		turn.StatusDetail = "design capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	if persistence.Diagnostic != nil {
		return diagnosticViews([]diagnostic.Diagnostic{*persistence.Diagnostic})
	}
	return nil
}

func (controller *sessionController) designRequestFromCurrentState(phase workflow.Phase) capability.Request {
	metadata := map[string]string{
		capability.DesignMetadataGoalID:         "aila-terminal-design-system",
		capability.DesignMetadataGoalSummary:    "Keep Aila's terminal UI design system durable for agents and humans.",
		capability.DesignMetadataSurface:        "terminal-ui",
		capability.DesignMetadataDecisions:      "phase-hierarchy::information architecture::Keep active phase and capability identity visible before detailed evidence.::Users need orientation before inspecting dense output.|review-prompts::visual review::Record explicit visual review prompts beside design decisions.::Screenshots can support human judgment but must not become the correctness contract.|artifact-boundary::state store::Persist durable design decisions through the project artifact store.::The TUI displays app-owned evidence and does not own persistence.",
		capability.DesignMetadataReviewPrompts:  "desktop-hierarchy::Does the wide layout preserve content and session hierarchy?::docs/mockup-desktop.png|narrow-clarity::Can 80x24 still show phase, prompt, caveats, and next action?::docs/mockup-mobile.png",
		capability.DesignMetadataCaveats:        "deterministic app-supplied design evidence only|screenshots are review aids, not correctness contracts|no major visual language change in this slice",
		capability.DesignMetadataNextAction:     "Audit the design-system artifact before continuing.",
		capability.DesignMetadataDurable:        "true",
		capability.DesignMetadataArtifactStatus: "planned",
	}
	return capability.Request{
		ID:         "command-design",
		Capability: capability.NameDesign,
		Input:      metadata[capability.DesignMetadataGoalSummary],
		Phase:      normalizeDesignPhase(phase),
		SourceRefs: []capability.SourceRef{
			{ID: "design-command", Kind: "command", Command: "/design", Excerpt: "app-owned design command"},
			{ID: "design-workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", LineStart: 264, LineEnd: 276, Excerpt: "design is BUILD-owned visual identity and UI-system work"},
			{ID: "design-tui-testing-doc", Kind: "doc", Path: "docs/tui-testing.md", LineStart: 330, LineEnd: 365, Excerpt: "visual review complements deterministic snapshots"},
		},
		Metadata: metadata,
	}
}

func normalizeDesignPhase(phase workflow.Phase) workflow.Phase {
	if phase == "" || phase == workflow.PhaseIdle {
		return workflow.PhaseBuild
	}
	return phase
}

func (controller *sessionController) persistDesignPayload(payload capability.ExitPayload) designArtifactPersistence {
	if payload.Design == nil || strings.TrimSpace(payload.Design.DesignArtifact) == "" {
		return designArtifactPersistence{Status: "not_written"}
	}
	return writeDesignArtifact(controller.ctx, controller.workspacePath, payload.Design.DesignArtifact)
}

func writeDesignArtifact(ctx context.Context, workspacePath string, document string) designArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return designArtifactPersistence{Status: "recovery_needed", Diagnostic: designArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	artifact, err := store.WriteArtifact(ctx, state.ArtifactDesign, state.OwnerApp, []byte(document))
	if err != nil {
		return designArtifactPersistence{Status: "recovery_needed", Diagnostic: designArtifactDiagnostic(err)}
	}
	return designArtifactPersistence{Path: artifact.Path, Status: "written"}
}

func designArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "design artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactDesign,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func designView(payload capability.ExitPayload, current workflow.Phase, persistence designArtifactPersistence) *tui.DesignView {
	if payload.Capability != capability.NameDesign {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	var output capability.DesignOutput
	if payload.Design != nil {
		output = *payload.Design
	}
	artifactPath := valueOr(output.DesignArtifactPath, ".aila/artifacts/design.md")
	if persistence.Path != "" {
		artifactPath = persistence.Path
	}
	caveats := append([]string(nil), output.Caveats...)
	if len(caveats) == 0 && payload.Design == nil {
		caveats = append([]string(nil), payload.Concerns...)
	}
	return &tui.DesignView{
		Source:               "app.design",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		CurrentPhase:         current.String(),
		Summary:              payload.Summary,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		Goal:                 tui.DesignGoalView{ID: output.Goal.ID, Summary: output.Goal.Summary, Surface: output.Goal.Surface},
		Decisions:            designDecisionViews(output.Decisions),
		ReviewPrompts:        designReviewPromptViews(output.ReviewPrompts),
		Caveats:              caveats,
		NeededInput:          payload.NeededInput,
		NextAction:           payload.NextAction,
		VisualReviewRequired: output.VisualReviewRequired,
		DesignArtifactPath:   artifactPath,
		ArtifactStatus:       artifactStatus,
		ArtifactRefs:         designArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           designSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     designBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func designDecisionViews(decisions []capability.DesignDecision) []tui.DesignDecisionView {
	views := make([]tui.DesignDecisionView, 0, len(decisions))
	for _, decision := range decisions {
		views = append(views, tui.DesignDecisionView{ID: decision.ID, Area: decision.Area, Decision: decision.Decision, Rationale: decision.Rationale})
	}
	return views
}

func designReviewPromptViews(prompts []capability.DesignReviewPrompt) []tui.DesignReviewPromptView {
	views := make([]tui.DesignReviewPromptView, 0, len(prompts))
	for _, prompt := range prompts {
		views = append(views, tui.DesignReviewPromptView{ID: prompt.ID, Question: prompt.Question, Target: prompt.Target})
	}
	return views
}

func designArtifactRefViews(refs []capability.ArtifactRef) []tui.DesignArtifactRefView {
	views := make([]tui.DesignArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.DesignArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func designSourceRefViews(refs []capability.SourceRef) []tui.DesignSourceRefView {
	views := make([]tui.DesignSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.DesignSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func designBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.DesignBoundaryRequestView {
	views := make([]tui.DesignBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.DesignBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
