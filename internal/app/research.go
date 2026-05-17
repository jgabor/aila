package app

import (
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/tui"
	"github.com/jgabor/aila/internal/workflow"
)

func (controller *sessionController) researchRequestFromView() capability.Request {
	topic := researchTopicEvidence(controller.view)
	contextSummary := sessionStateEvidence(controller.view)
	metadata := map[string]string{
		capability.ResearchMetadataTopic:          topic,
		capability.ResearchMetadataContext:        contextSummary,
		capability.ResearchMetadataPatterns:       "Cross-cutting helpers return context evidence without owning phase transitions|Source refs travel with condensed claims|Deterministic fixtures precede interactive terminal smoke",
		capability.ResearchMetadataEvidence:       "docs/workflow-architecture.md keeps research cross-cutting|ARCHITECTURE.md requires FSM-owned transitions|docs/tui-testing.md requires render and semantic fixtures before PTY smoke",
		capability.ResearchMetadataConfidence:     "medium",
		capability.ResearchMetadataCaveats:        "deterministic app-supplied pattern evidence only|live external fetching remains deferred",
		capability.ResearchMetadataNextAction:     "Use this research as non-authoritative context for the current workflow phase.",
		capability.ResearchMetadataContextSummary: contextSummary,
	}
	return capability.Request{
		ID:         "command-research",
		Capability: capability.NameResearch,
		Input:      topic,
		Phase:      workflowPhaseFromView(controller.view),
		SourceRefs: []capability.SourceRef{
			{ID: "research-command", Kind: "command", Command: "/research", Excerpt: "app-owned research command"},
			{ID: "research-workflow-doc", Kind: "doc", Path: "docs/workflow-architecture.md", LineStart: 264, LineEnd: 279, Excerpt: "research is cross-cutting"},
			{ID: "research-session-state", Kind: "session_state", Excerpt: contextSummary},
		},
		Metadata: metadata,
	}
}

func (controller *sessionController) openResearchView() []diagnostic.Diagnostic {
	request := controller.researchRequestFromView()
	turn := controller.runner.proposeCapability(request)
	turn.Research = researchView(controller.runner.model.LastCapability, request.Phase)
	turn.Context = researchContextView(controller.runner.model.LastCapability)
	if turn.Research != nil {
		turn.StatusDetail = "research capability status"
	}
	controller.view = tui.ApplyTranscriptTurn(controller.view, turn)
	controller.view = applyRuntimeModelToView(controller.view, controller.runner.model)
	if turn.Context != nil {
		controller.view.Context = turn.Context
		if turn.Context.Meter != "" {
			controller.view.FooterContext = turn.Context.Meter
		}
	}
	return nil
}

func researchView(payload capability.ExitPayload, current workflow.Phase) *tui.ResearchView {
	if payload.Capability != capability.NameResearch {
		return nil
	}
	var output capability.ResearchOutput
	if payload.Research != nil {
		output = *payload.Research
	}
	caveats := append([]string(nil), output.Caveats...)
	if len(caveats) == 0 && payload.Research == nil {
		caveats = append([]string(nil), payload.Concerns...)
	}
	return &tui.ResearchView{
		Source:               "app.research",
		Capability:           string(payload.Capability),
		Signal:               string(payload.Signal),
		CurrentPhase:         current.String(),
		CrossCuttingStatus:   valueOr(output.CrossCuttingStatus, "context_only"),
		Summary:              payload.Summary,
		Topic:                output.Topic,
		Context:              output.Context,
		Patterns:             researchPatternViews(output.Patterns),
		Evidence:             researchEvidenceViews(output.Evidence),
		Confidence:           output.Confidence,
		Caveats:              caveats,
		NeededInput:          payload.NeededInput,
		NextAction:           payload.NextAction,
		ContextSummary:       output.ContextSummary,
		ContextFolded:        payload.Research != nil && payload.Signal != capability.ExitWaiting,
		RecommendedSuccessor: string(payload.RecommendedSuccessor),
		TransitionClaimed:    false,
		DisplayOnly:          true,
		SourceRefs:           researchSourceRefViews(payload.SourceRefs),
		BoundaryRequests:     researchBoundaryRequestViews(payload.BoundaryRequests),
	}
}

func researchContextView(payload capability.ExitPayload) *tui.ContextView {
	if payload.Capability != capability.NameResearch || payload.Research == nil {
		return nil
	}
	output := payload.Research
	blocks := []tui.ContextBlockView{{
		ID:           "research-summary",
		Kind:         "research",
		Title:        output.Topic,
		Text:         output.ContextSummary,
		SourceRefIDs: researchSourceRefIDs(output.SourceRefs),
	}}
	claims := make([]tui.ContextClaimView, 0, len(output.Patterns)+len(output.Evidence))
	for _, pattern := range output.Patterns {
		claims = append(claims, tui.ContextClaimView{Text: "research pattern: " + pattern.Concept, SourceRefIDs: append([]string(nil), pattern.EvidenceRefIDs...)})
	}
	for _, evidence := range output.Evidence {
		refs := []string{}
		if evidence.SourceRefID != "" {
			refs = []string{evidence.SourceRefID}
		}
		claims = append(claims, tui.ContextClaimView{Text: "research evidence: " + evidence.Summary, SourceRefIDs: refs})
	}
	return &tui.ContextView{
		Source:     "app.research.context",
		Status:     "folded",
		Meter:      fmt.Sprintf("research refs: %d", len(output.SourceRefs)),
		Blocks:     blocks,
		Claims:     claims,
		SourceRefs: researchContextSourceRefViews(output.SourceRefs),
		Warnings:   append([]string(nil), output.Caveats...),
	}
}

func researchTopicEvidence(view tui.ViewState) string {
	context := strings.TrimSpace(view.FooterContext)
	if context == "" || context == "placeholder" {
		return "Research external patterns for Aila."
	}
	return "Research external patterns for " + context + "."
}

func researchPatternViews(patterns []capability.ResearchPattern) []tui.ResearchPatternView {
	views := make([]tui.ResearchPatternView, 0, len(patterns))
	for _, pattern := range patterns {
		views = append(views, tui.ResearchPatternView{ID: pattern.ID, Concept: pattern.Concept, Applicability: pattern.Applicability, EvidenceRefIDs: append([]string(nil), pattern.EvidenceRefIDs...)})
	}
	return views
}

func researchEvidenceViews(evidence []capability.ResearchEvidence) []tui.ResearchEvidenceView {
	views := make([]tui.ResearchEvidenceView, 0, len(evidence))
	for _, item := range evidence {
		views = append(views, tui.ResearchEvidenceView{ID: item.ID, Summary: item.Summary, SourceRefID: item.SourceRefID})
	}
	return views
}

func researchSourceRefViews(refs []capability.SourceRef) []tui.ResearchSourceRefView {
	views := make([]tui.ResearchSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.ResearchSourceRefView{ID: ref.ID, Kind: ref.Kind, Path: ref.Path, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func researchContextSourceRefViews(refs []capability.SourceRef) []tui.ContextSourceRefView {
	views := make([]tui.ContextSourceRefView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, tui.ContextSourceRefView{ID: ref.ID, Kind: ref.Kind, Label: ref.Kind, Path: ref.Path, LineStart: ref.LineStart, LineEnd: ref.LineEnd, Command: ref.Command, Excerpt: ref.Excerpt})
	}
	return views
}

func researchSourceRefIDs(refs []capability.SourceRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func researchBoundaryRequestViews(requests []capability.BoundaryRequest) []tui.ResearchBoundaryRequestView {
	views := make([]tui.ResearchBoundaryRequestView, 0, len(requests))
	for _, request := range requests {
		views = append(views, tui.ResearchBoundaryRequestView{Kind: string(request.Kind), Operation: request.Operation, Target: request.Target, Reason: request.Reason})
	}
	return views
}
