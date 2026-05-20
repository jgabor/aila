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

type visionArtifactPersistence struct {
	Path       string
	Status     string
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) visionRequestFromView() capability.Request {
	northStar := visionNorthStarEvidence(controller.view)
	metadata := map[string]string{
		capability.VisionMetadataNorthStar:       northStar,
		capability.VisionMetadataContextSummary:  sessionStateEvidence(controller.view),
		capability.VisionMetadataRecommendedNext: workflow.PhasePlan.String(),
		capability.VisionMetadataNextAction:      "Use this vision as source material for planning.",
	}
	return capability.Request{
		ID:         "command-vision",
		Capability: capability.NameVision,
		Input:      northStar,
		Phase:      workflow.PhaseEnvision,
		SourceRefs: []capability.SourceRef{
			{ID: "vision-command", Kind: "command", Command: "/vision", Excerpt: "app-owned vision command"},
			{ID: "vision-project-state", Kind: "project_state", Excerpt: projectStateEvidence(controller.view)},
			{ID: "vision-session-state", Kind: "session_state", Excerpt: metadata[capability.VisionMetadataContextSummary]},
		},
		Metadata: metadata,
	}
}

func (controller *sessionController) openVisionView() []diagnostic.Diagnostic {
	request := controller.visionRequestFromView()
	turn := controller.runner.proposeCapability(request)
	persistence := controller.persistVisionPayload(controller.runner.model.LastCapability)
	turn.Vision = visionView(controller.runner.model.LastCapability, request.Phase, persistence)
	if turn.Vision != nil {
		turn.StatusDetail = "vision capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model, controller.workspacePath)
	if persistence.Diagnostic == nil {
		return nil
	}
	return []diagnostic.Diagnostic{*persistence.Diagnostic}
}

func (controller *sessionController) persistVisionPayload(payload capability.ExitPayload) visionArtifactPersistence {
	if payload.Vision == nil || strings.TrimSpace(payload.Vision.Document) == "" {
		return visionArtifactPersistence{Status: "not_written"}
	}
	return writeVisionArtifact(controller.ctx, controller.workspacePath, payload.Vision.Document)
}

func writeVisionArtifact(ctx context.Context, workspacePath string, document string) visionArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return visionArtifactPersistence{Status: "recovery_needed", Diagnostic: visionArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	artifact, err := store.WriteArtifact(ctx, state.ArtifactVision, state.OwnerApp, []byte(document))
	if err != nil {
		return visionArtifactPersistence{Status: "recovery_needed", Diagnostic: visionArtifactDiagnostic(err)}
	}
	return visionArtifactPersistence{Path: artifact.Path, Status: "written"}
}

func visionArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "vision artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactVision,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func visionView(payload capability.ExitPayload, current workflow.Phase, persistence visionArtifactPersistence) *tui.VisionView {
	if payload.Capability != capability.NameVision {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	artifactPath := capabilityDefaultVisionArtifactPath()
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	var output capability.VisionOutput
	if payload.Vision != nil {
		output = *payload.Vision
		artifactPath = output.ArtifactPath
	}
	if persistence.Path != "" {
		artifactPath = persistence.Path
	}
	return &tui.VisionView{
		Source:               "app.vision",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		Phase:                workflow.PhaseEnvision.String(),
		Summary:              payload.Summary,
		NorthStar:            output.NorthStar,
		Principles:           append([]string(nil), output.Principles...),
		LongTermGoals:        append([]string(nil), output.LongTermGoals...),
		Blockers:             append([]string(nil), output.Blockers...),
		NeededInput:          payload.NeededInput,
		NextAction:           payload.NextAction,
		ArtifactPath:         artifactPath,
		ArtifactStatus:       artifactStatus,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		SuccessorValid:       recommendation.SuccessorValid,
		SuccessorRejected:    recommendation.SuccessorRejected,
		SuccessorReason:      recommendation.SuccessorReason,
		TransitionClaimed:    false,
		DisplayOnly:          true,
		ArtifactRefs:         visionArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           visionSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     visionBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func capabilityDefaultVisionArtifactPath() string {
	return ".aila/artifacts/vision.md"
}

func visionNorthStarEvidence(view tui.ViewState) string {
	context := strings.TrimSpace(view.FooterContext)
	if context == "" || context == "placeholder" {
		return "Shape Aila's project direction before planning broad work."
	}
	return "Shape Aila's project direction for " + context + "."
}

func visionArtifactRefViews(refs []capability.ArtifactRef) []tui.VisionArtifactRefView {
	views := make([]tui.VisionArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.VisionArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func visionSourceRefViews(refs []capability.SourceRef) []tui.VisionSourceRefView {
	views := make([]tui.VisionSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.VisionSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func visionBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.VisionBoundaryRequestView {
	views := make([]tui.VisionBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.VisionBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
