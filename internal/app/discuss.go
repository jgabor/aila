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

type discussArtifactPersistence struct {
	Path       string
	Status     string
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) discussRequestFromView() capability.Request {
	question := discussQuestionEvidence(controller.view)
	metadata := map[string]string{
		capability.DiscussMetadataQuestion:        question,
		capability.DiscussMetadataContext:         sessionStateEvidence(controller.view),
		capability.DiscussMetadataOptions:         "Plan the scoped next step|Revisit project vision|Proceed directly to build",
		capability.DiscussMetadataSelected:        "Plan the scoped next step",
		capability.DiscussMetadataReasoning:       "Planning keeps the next step bounded and preserves workflow authority before build work.",
		capability.DiscussMetadataConfidence:      "medium",
		capability.DiscussMetadataRecommendedNext: workflow.PhasePlan.String(),
		capability.DiscussMetadataNextAction:      "Use this decision as source material for planning.",
		capability.DiscussMetadataContextSummary:  sessionStateEvidence(controller.view),
	}
	return capability.Request{
		ID:         "command-discuss",
		Capability: capability.NameDiscuss,
		Input:      question,
		Phase:      workflow.PhaseDeliberate,
		SourceRefs: []capability.SourceRef{
			{ID: "discuss-command", Kind: "command", Command: "/discuss", Excerpt: "app-owned discuss command"},
			{ID: "discuss-project-state", Kind: "project_state", Excerpt: projectStateEvidence(controller.view)},
			{ID: "discuss-session-state", Kind: "session_state", Excerpt: metadata[capability.DiscussMetadataContextSummary]},
		},
		Metadata: metadata,
	}
}

func (controller *sessionController) openDiscussView() []diagnostic.Diagnostic {
	request := controller.discussRequestFromView()
	turn := controller.runner.proposeCapability(request)
	persistence := controller.persistDiscussPayload(controller.runner.model.LastCapability)
	turn.Discuss = discussView(controller.runner.model.LastCapability, request.Phase, persistence)
	if turn.Discuss != nil {
		turn.StatusDetail = "discuss capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model)
	if persistence.Diagnostic == nil {
		return nil
	}
	return []diagnostic.Diagnostic{*persistence.Diagnostic}
}

func (controller *sessionController) persistDiscussPayload(payload capability.ExitPayload) discussArtifactPersistence {
	if payload.Discuss == nil || strings.TrimSpace(payload.Discuss.Document) == "" {
		return discussArtifactPersistence{Status: "not_written"}
	}
	return writeDiscussArtifact(controller.ctx, controller.workspacePath, payload.Discuss.Document)
}

func writeDiscussArtifact(ctx context.Context, workspacePath string, document string) discussArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return discussArtifactPersistence{Status: "recovery_needed", Diagnostic: discussArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	artifact, err := store.WriteArtifact(ctx, state.ArtifactDecisions, state.OwnerApp, []byte(document))
	if err != nil {
		return discussArtifactPersistence{Status: "recovery_needed", Diagnostic: discussArtifactDiagnostic(err)}
	}
	return discussArtifactPersistence{Path: artifact.Path, Status: "written"}
}

func discussArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "decision artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactDecisions,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func discussView(payload capability.ExitPayload, current workflow.Phase, persistence discussArtifactPersistence) *tui.DiscussView {
	if payload.Capability != capability.NameDiscuss {
		return nil
	}
	recommendation := policy.RecommendCapabilitySuccessor(current, payload)
	artifactPath := capabilityDefaultDiscussArtifactPath()
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	var output capability.DiscussOutput
	if payload.Discuss != nil {
		output = *payload.Discuss
		artifactPath = output.ArtifactPath
	}
	if persistence.Path != "" {
		artifactPath = persistence.Path
	}
	return &tui.DiscussView{
		Source:               "app.discuss",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		Phase:                workflow.PhaseDeliberate.String(),
		Summary:              payload.Summary,
		Question:             output.Question,
		Context:              output.Context,
		Options:              discussOptionViews(output.Options),
		Selected:             output.Selected,
		Reasoning:            output.Reasoning,
		Confidence:           output.Confidence,
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
		ArtifactRefs:         discussArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           discussSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     discussBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func capabilityDefaultDiscussArtifactPath() string {
	return ".aila/artifacts/decisions.md"
}

func discussQuestionEvidence(view tui.ViewState) string {
	context := strings.TrimSpace(view.FooterContext)
	if context == "" || context == "placeholder" {
		return "Decide the next safe workflow direction for Aila."
	}
	return "Decide the next safe workflow direction for " + context + "."
}

func discussOptionViews(options []capability.DiscussOption) []tui.DiscussOptionView {
	views := make([]tui.DiscussOptionView, 0, len(options))
	for _, option := range options {
		views = append(views, tui.DiscussOptionView{ID: option.ID, Text: option.Text, Selected: option.Selected, Rationale: option.Rationale})
	}
	return views
}

func discussArtifactRefViews(refs []capability.ArtifactRef) []tui.DiscussArtifactRefView {
	views := make([]tui.DiscussArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.DiscussArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func discussSourceRefViews(refs []capability.SourceRef) []tui.DiscussSourceRefView {
	views := make([]tui.DiscussSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.DiscussSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func discussBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.DiscussBoundaryRequestView {
	views := make([]tui.DiscussBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.DiscussBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
