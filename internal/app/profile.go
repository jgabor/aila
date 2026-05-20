package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/state"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

type profileArtifactPersistence struct {
	Path       string
	Status     string
	Diagnostic *diagnostic.Diagnostic
}

func (controller *sessionController) profileRequestFromView() capability.Request {
	contextSummary := sessionStateEvidence(controller.view)
	metadata := map[string]string{
		capability.ProfileMetadataSubject:           "Aila decision profile",
		capability.ProfileMetadataContext:           contextSummary,
		capability.ProfileMetadataDecisionSignals:   "Prefer bounded roadmap slices before broad refactors|Keep validation evidence close to the completed task|Prefer behavior-named tests over milestone numbers",
		capability.ProfileMetadataUpdateSuggestions: "Keep capability validation evidence near the closeout artifact|Rename touched milestone-numbered tests when behavior names communicate the invariant",
		capability.ProfileMetadataEvidence:          "Recent roadmap work used planera before implementation|Validation gates caught stale milestone-numbered test names|Cross-cutting capability outputs fold into context without owning phase transitions",
		capability.ProfileMetadataConfidence:        "medium",
		capability.ProfileMetadataCaveats:           "deterministic app-supplied session evidence only|provider-backed corpus analysis deferred",
		capability.ProfileMetadataNextAction:        "Use this profile as non-authoritative context for the current workflow phase.",
		capability.ProfileMetadataContextSummary:    contextSummary,
		capability.ProfileMetadataDurable:           "true",
	}
	return capability.Request{
		ID:         "command-profile",
		Capability: capability.NameProfile,
		Input:      metadata[capability.ProfileMetadataSubject],
		Phase:      workflowPhaseFromView(controller.view),
		SourceRefs: []capability.SourceRef{
			{ID: "profile-command", Kind: "command", Command: "/profile", Excerpt: "app-owned profile command"},
			{ID: "profile-workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", LineStart: 463, LineEnd: 472, Excerpt: "profile is cross-cutting and never triggers phase transitions by itself"},
			{ID: "profile-session-state", Kind: "session_state", Excerpt: contextSummary},
		},
		Metadata: metadata,
	}
}

func (controller *sessionController) openProfileView() []diagnostic.Diagnostic {
	request := controller.profileRequestFromView()
	turn := controller.runner.proposeCapability(request)
	persistence := controller.persistProfilePayload(controller.runner.model.LastCapability)
	turn.Profile = profileView(controller.runner.model.LastCapability, request.Phase, persistence)
	turn.Context = profileContextView(controller.runner.model.LastCapability)
	if turn.Profile != nil {
		turn.StatusDetail = "profile capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model, controller.workspacePath)
	if turn.Context != nil {
		controller.view.Context = turn.Context
		if turn.Context.Meter != "" {
			controller.view.FooterContext = turn.Context.Meter
		}
	}
	if turn.Profile != nil {
		controller.view.Profile = turn.Profile
	}
	if persistence.Diagnostic == nil {
		return nil
	}
	return []diagnostic.Diagnostic{*persistence.Diagnostic}
}

func (controller *sessionController) persistProfilePayload(payload capability.ExitPayload) profileArtifactPersistence {
	if payload.Profile == nil || strings.TrimSpace(payload.Profile.Document) == "" {
		return profileArtifactPersistence{Status: "not_written"}
	}
	return writeProfileArtifact(controller.ctx, controller.workspacePath, payload.Profile.Document)
}

func writeProfileArtifact(ctx context.Context, workspacePath string, document string) profileArtifactPersistence {
	store, err := state.OpenProjectStore(ctx, workspacePath)
	if err != nil {
		return profileArtifactPersistence{Status: "recovery_needed", Diagnostic: profileArtifactDiagnostic(fmt.Errorf("open project store: %w", err))}
	}
	artifact, err := store.WriteArtifact(ctx, state.ArtifactProfile, state.OwnerApp, []byte(document))
	if err != nil {
		return profileArtifactPersistence{Status: "recovery_needed", Diagnostic: profileArtifactDiagnostic(err)}
	}
	return profileArtifactPersistence{Path: artifact.Path, Status: "written"}
}

func profileArtifactDiagnostic(err error) *diagnostic.Diagnostic {
	message := "profile artifact write failed"
	if err != nil {
		message += ": " + boundedStoreError(err)
	}
	diagnostic := diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryState,
		Source:           diagnostic.SourceStateSnapshot,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactProfile,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})
	return &diagnostic
}

func profileView(payload capability.ExitPayload, current workflow.Phase, persistence profileArtifactPersistence) *tui.ProfileView {
	if payload.Capability != capability.NameProfile {
		return nil
	}
	artifactPath := capabilityDefaultProfileArtifactPath()
	artifactStatus := persistence.Status
	if artifactStatus == "" {
		artifactStatus = "available"
	}
	var output capability.ProfileOutput
	if payload.Profile != nil {
		output = *payload.Profile
		artifactPath = output.ArtifactPath
	}
	if persistence.Path != "" {
		artifactPath = persistence.Path
	}
	caveats := append([]string(nil), output.Caveats...)
	if len(caveats) == 0 && payload.Profile == nil {
		caveats = append([]string(nil), payload.Concerns...)
	}
	return &tui.ProfileView{
		Source:               "app.profile",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		CurrentPhase:         current.String(),
		CrossCuttingStatus:   valueOr(output.CrossCuttingStatus, "context_only"),
		Summary:              payload.Summary,
		Subject:              output.Subject,
		Context:              output.Context,
		DecisionSignals:      profileDecisionSignalViews(output.DecisionSignals),
		UpdateSuggestions:    profileUpdateSuggestionViews(output.UpdateSuggestions),
		Evidence:             profileEvidenceViews(output.Evidence),
		Confidence:           output.Confidence,
		Caveats:              caveats,
		NeededInput:          payload.NeededInput,
		NextAction:           payload.NextAction,
		ContextSummary:       output.ContextSummary,
		ArtifactPath:         artifactPath,
		ArtifactStatus:       artifactStatus,
		ContextFolded:        payload.Profile != nil && payload.Signal != capability.ExitWaiting,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		TransitionClaimed:    false,
		DisplayOnly:          true,
		ArtifactRefs:         profileArtifactRefViews(payload.ArtifactRefs),
		SourceRefs:           profileSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     profileBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func profileContextView(payload capability.ExitPayload) *tui.ContextView {
	if payload.Capability != capability.NameProfile || payload.Profile == nil {
		return nil
	}
	output := payload.Profile
	blocks := []tui.ContextBlockView{{
		ID:           "profile-summary",
		Kind:         "profile",
		Title:        output.Subject,
		Text:         output.ContextSummary,
		SourceRefIDs: profileSourceRefIDs(output.SourceRefs),
	}}
	claims := make([]tui.ContextClaimView, 0, len(output.DecisionSignals)+len(output.UpdateSuggestions)+len(output.Evidence))
	for _, signal := range output.DecisionSignals {
		claims = append(claims, tui.ContextClaimView{Text: "profile signal: " + signal.Pattern, SourceRefIDs: append([]string(nil), signal.EvidenceRefIDs...)})
	}
	for _, suggestion := range output.UpdateSuggestions {
		claims = append(claims, tui.ContextClaimView{Text: "profile update: " + suggestion.Text, SourceRefIDs: append([]string(nil), suggestion.EvidenceRefIDs...)})
	}
	for _, evidence := range output.Evidence {
		refs := []string{}
		if evidence.SourceRefID != "" {
			refs = []string{evidence.SourceRefID}
		}
		claims = append(claims, tui.ContextClaimView{Text: "profile evidence: " + evidence.Summary, SourceRefIDs: refs})
	}
	return &tui.ContextView{
		Source:     "app.profile.context",
		Status:     "folded",
		Meter:      fmt.Sprintf("profile refs: %d", len(output.SourceRefs)),
		Blocks:     blocks,
		Claims:     claims,
		SourceRefs: profileContextSourceRefViews(output.SourceRefs),
		Warnings:   append([]string(nil), output.Caveats...),
	}
}

func capabilityDefaultProfileArtifactPath() string {
	return ".aila/artifacts/profile.md"
}

func profileDecisionSignalViews(signals []capability.ProfileDecisionSignal) []tui.ProfileDecisionSignalView {
	views := make([]tui.ProfileDecisionSignalView, 0, len(signals))
	for _, signal := range signals {
		views = append(views, tui.ProfileDecisionSignalView{ID: signal.ID, Pattern: signal.Pattern, Guidance: signal.Guidance, EvidenceRefIDs: append([]string(nil), signal.EvidenceRefIDs...)})
	}
	return views
}

func profileUpdateSuggestionViews(suggestions []capability.ProfileUpdateSuggestion) []tui.ProfileUpdateSuggestionView {
	views := make([]tui.ProfileUpdateSuggestionView, 0, len(suggestions))
	for _, suggestion := range suggestions {
		views = append(views, tui.ProfileUpdateSuggestionView{ID: suggestion.ID, Text: suggestion.Text, Rationale: suggestion.Rationale, EvidenceRefIDs: append([]string(nil), suggestion.EvidenceRefIDs...)})
	}
	return views
}

func profileEvidenceViews(evidence []capability.ProfileEvidence) []tui.ProfileEvidenceView {
	views := make([]tui.ProfileEvidenceView, 0, len(evidence))
	for _, item := range evidence {
		views = append(views, tui.ProfileEvidenceView{ID: item.ID, Summary: item.Summary, SourceRefID: item.SourceRefID})
	}
	return views
}

func profileArtifactRefViews(refs []capability.ArtifactRef) []tui.ProfileArtifactRefView {
	views := make([]tui.ProfileArtifactRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.ProfileArtifactRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path})
	}
	return views
}

func profileSourceRefViews(refs []capability.SourceRef) []tui.ProfileSourceRefView {
	views := make([]tui.ProfileSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.ProfileSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func profileContextSourceRefViews(refs []capability.SourceRef) []tui.ContextSourceRefView {
	views := make([]tui.ContextSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.ContextSourceRefView{ID: ref.ID, Kind: ref.Kind, Label: ref.Kind, Path: ref.Path, LineStart: ref.LineStart, LineEnd: ref.LineEnd, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func profileSourceRefIDs(refs []capability.SourceRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func profileBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.ProfileBoundaryRequestView {
	views := make([]tui.ProfileBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.ProfileBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
