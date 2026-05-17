package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	ResearchMetadataTopic          = "research_topic"
	ResearchMetadataContext        = "research_context"
	ResearchMetadataPatterns       = "research_patterns"
	ResearchMetadataEvidence       = "research_evidence"
	ResearchMetadataConfidence     = "research_confidence"
	ResearchMetadataCaveats        = "research_caveats"
	ResearchMetadataNextAction     = "research_next_action"
	ResearchMetadataContextSummary = "research_context_summary"
)

// ResearchCapability adapts external pattern evidence into non-authoritative context.
type ResearchCapability struct{}

// ResearchPattern records one pattern or concept found by research.
type ResearchPattern struct {
	ID             string
	Concept        string
	Applicability  string
	EvidenceRefIDs []string
}

// ResearchEvidence records one bounded source-backed research observation.
type ResearchEvidence struct {
	ID          string
	Summary     string
	SourceRefID string
}

// ResearchOutput is the typed context-folding data carried by a research capability exit.
type ResearchOutput struct {
	Topic              string
	CurrentPhase       string
	CrossCuttingStatus string
	Context            string
	Patterns           []ResearchPattern
	Evidence           []ResearchEvidence
	Confidence         string
	Caveats            []string
	NextAction         string
	ContextSummary     string
	SourceRefs         []SourceRef
}

// Name returns the fixed capability identity.
func (ResearchCapability) Name() Name {
	return NameResearch
}

// OwningPhase returns IDLE because research is explicitly cross-cutting.
func (ResearchCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseIdle
}

// Run emits one research payload without fetching content, writing artifacts, or mutating workflow phase.
func (ResearchCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run research capability: %w", err)
	}
	request = normalizeResearchRequest(request)
	invocation := NewInvocation(request)
	if researchTopic(request) == "" {
		payload := ExitPayload{
			Capability:       NameResearch,
			Signal:           ExitWaiting,
			Summary:          "Research needs a topic before it can adapt external patterns.",
			Concerns:         []string{"research evidence unavailable until a topic is provided"},
			NeededInput:      "Describe the pattern, concept, library, or solution space to research.",
			NextAction:       "Provide a research topic, then run research again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: researchBoundaryRequests(request),
		}
		return invocation.Emit(payload)
	}

	output := buildResearchOutput(request)
	payload := ExitPayload{
		Capability:       NameResearch,
		Signal:           ExitComplete,
		Summary:          researchSummary(output),
		Concerns:         append([]string(nil), output.Caveats...),
		Attempted:        true,
		NextAction:       output.NextAction,
		SourceRefs:       cloneSourceRefs(output.SourceRefs),
		BoundaryRequests: researchBoundaryRequests(request),
		Research:         &output,
	}
	return invocation.Emit(payload)
}

func normalizeResearchRequest(request Request) Request {
	request.Capability = NameResearch
	if request.Phase == "" {
		request.Phase = workflow.PhaseIdle
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func buildResearchOutput(request Request) ResearchOutput {
	sourceRefs := researchSourceRefs(request)
	evidence := researchEvidence(request, sourceRefs)
	patterns := researchPatterns(request, evidence)
	caveats := researchListMetadata(request, ResearchMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"research used app-supplied pattern evidence only"}
	}
	return ResearchOutput{
		Topic:              researchTopic(request),
		CurrentPhase:       request.Phase.String(),
		CrossCuttingStatus: "context_only",
		Context:            researchMetadata(request, ResearchMetadataContext, researchMetadata(request, ResearchMetadataContextSummary, "")),
		Patterns:           patterns,
		Evidence:           evidence,
		Confidence:         researchMetadata(request, ResearchMetadataConfidence, "medium"),
		Caveats:            caveats,
		NextAction:         researchMetadata(request, ResearchMetadataNextAction, "Use this research as non-authoritative context for the current workflow phase."),
		ContextSummary:     researchContextSummary(request, patterns, evidence, caveats),
		SourceRefs:         sourceRefs,
	}
}

func researchTopic(request Request) string {
	if topic := researchMetadata(request, ResearchMetadataTopic, ""); topic != "" {
		return topic
	}
	return strings.TrimSpace(request.Input)
}

func researchPatterns(request Request, evidence []ResearchEvidence) []ResearchPattern {
	items := researchListMetadata(request, ResearchMetadataPatterns)
	if len(items) == 0 {
		items = []string{"Prefer proven external patterns before custom workflow machinery"}
	}
	patterns := make([]ResearchPattern, 0, len(items))
	for index, item := range items {
		refID := ""
		if len(evidence) > 0 {
			refID = evidence[index%len(evidence)].SourceRefID
		}
		pattern := ResearchPattern{
			ID:            fmt.Sprintf("pattern-%d", index+1),
			Concept:       item,
			Applicability: "adapt as context, not workflow authority",
		}
		if refID != "" {
			pattern.EvidenceRefIDs = []string{refID}
		}
		patterns = append(patterns, pattern)
	}
	return patterns
}

func researchEvidence(request Request, sourceRefs []SourceRef) []ResearchEvidence {
	items := researchListMetadata(request, ResearchMetadataEvidence)
	if len(items) == 0 {
		items = []string{"Current app context supplied the research evidence."}
	}
	evidence := make([]ResearchEvidence, 0, len(items))
	for index, item := range items {
		refID := fmt.Sprintf("research-source-%d", index+1)
		if len(sourceRefs) > 0 {
			refID = sourceRefs[index%len(sourceRefs)].ID
		}
		evidence = append(evidence, ResearchEvidence{
			ID:          fmt.Sprintf("evidence-%d", index+1),
			Summary:     item,
			SourceRefID: refID,
		})
	}
	return evidence
}

func researchContextSummary(request Request, patterns []ResearchPattern, evidence []ResearchEvidence, caveats []string) string {
	if summary := researchMetadata(request, ResearchMetadataContextSummary, ""); summary != "" {
		return summary
	}
	return fmt.Sprintf("Research on %s produced %d pattern(s), %d evidence item(s), and %d caveat(s).", researchTopic(request), len(patterns), len(evidence), len(caveats))
}

func researchSummary(output ResearchOutput) string {
	return fmt.Sprintf("Research folded %d pattern(s) into context for %s.", len(output.Patterns), output.Topic)
}

func researchBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("project.current", "research uses app-supplied project state evidence"),
		request.RequestStateAccess("session.current", "research uses app-supplied session state evidence"),
		request.RequestContextAccess("current_context", "research folds results into current context"),
	}
}

func researchSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	if len(refs) == 0 {
		refs = append(refs, SourceRef{ID: "research-input", Kind: "prompt", Excerpt: researchTopic(request)})
	}
	contextSummary := strings.TrimSpace(request.Metadata[ResearchMetadataContextSummary])
	if contextSummary != "" {
		refs = append(refs, SourceRef{ID: "research-context", Kind: "context", Excerpt: contextSummary})
	}
	return refs
}

func researchMetadata(request Request, key string, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func researchListMetadata(request Request, key string) []string {
	if request.Metadata == nil {
		return nil
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return nil
	}
	fields := strings.Split(value, "|")
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}
