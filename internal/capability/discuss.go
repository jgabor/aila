package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	DiscussMetadataQuestion        = "discuss_question"
	DiscussMetadataContext         = "discuss_context"
	DiscussMetadataOptions         = "discuss_options"
	DiscussMetadataSelected        = "discuss_selected"
	DiscussMetadataReasoning       = "discuss_reasoning"
	DiscussMetadataConfidence      = "discuss_confidence"
	DiscussMetadataBlockers        = "discuss_blockers"
	DiscussMetadataArtifactPath    = "discuss_artifact_path"
	DiscussMetadataRecommendedNext = "discuss_recommended_successor"
	DiscussMetadataNextAction      = "discuss_next_action"
	DiscussMetadataContextSummary  = "discuss_context_summary"
)

const defaultDiscussArtifactPath = ".aila/artifacts/decisions.md"

// DiscussCapability records structured deliberation from app-supplied decision evidence.
type DiscussCapability struct{}

// DiscussOption records one decision option considered by discuss.
type DiscussOption struct {
	ID        string
	Text      string
	Selected  bool
	Rationale string
}

// DiscussOutput is the typed deliberation data carried by a discuss capability exit.
type DiscussOutput struct {
	Question     string
	Context      string
	Options      []DiscussOption
	Selected     string
	Reasoning    string
	Confidence   string
	Blockers     []string
	ArtifactPath string
	NextAction   string
	Document     string
	SourceRefs   []SourceRef
}

// Name returns the fixed capability identity.
func (DiscussCapability) Name() Name {
	return NameDiscuss
}

// OwningPhase returns DELIBERATE because the capability captures consequential decisions.
func (DiscussCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseDeliberate
}

// Run emits one discussion payload without reading files, calling providers, or mutating workflow phase.
func (DiscussCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run discuss capability: %w", err)
	}
	request = normalizeDiscussRequest(request)
	invocation := NewInvocation(request)
	if discussQuestion(request) == "" {
		payload := ExitPayload{
			Capability:       NameDiscuss,
			Signal:           ExitWaiting,
			Summary:          "Discuss needs a consequential decision before it can deliberate.",
			NeededInput:      "Describe the consequential decision to deliberate.",
			NextAction:       "Provide a decision question, then run discuss again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: discussBoundaryRequests(request),
		}
		return invocation.Emit(payload)
	}

	discussion := buildDiscussOutput(request)
	signal := ExitComplete
	if len(discussion.Blockers) > 0 {
		signal = ExitFlagged
	}
	successor := discussRecommendedSuccessor(request, signal)
	payload := ExitPayload{
		Capability:           NameDiscuss,
		Signal:               signal,
		Summary:              discussSummary(discussion, signal),
		Concerns:             append([]string(nil), discussion.Blockers...),
		Attempted:            true,
		NextAction:           discussion.NextAction,
		RecommendedSuccessor: successor,
		ArtifactRefs: []ArtifactRef{{
			ID:   "decision-artifact",
			Kind: "state_artifact",
			Path: discussion.ArtifactPath,
		}},
		SourceRefs:       cloneSourceRefs(discussion.SourceRefs),
		BoundaryRequests: discussBoundaryRequests(request),
		Discuss:          &discussion,
	}
	return invocation.Emit(payload)
}

func normalizeDiscussRequest(request Request) Request {
	request.Capability = NameDiscuss
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseDeliberate
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func buildDiscussOutput(request Request) DiscussOutput {
	question := discussQuestion(request)
	options := discussOptions(request)
	selected := discussSelected(request, options)
	for index := range options {
		options[index].Selected = options[index].Text == selected || options[index].ID == selected
	}
	blockers := discussListMetadata(request, DiscussMetadataBlockers)
	discussion := DiscussOutput{
		Question:     question,
		Context:      discussMetadata(request, DiscussMetadataContext, discussMetadata(request, DiscussMetadataContextSummary, "")),
		Options:      options,
		Selected:     selected,
		Reasoning:    discussMetadata(request, DiscussMetadataReasoning, defaultDiscussReasoning(selected)),
		Confidence:   discussMetadata(request, DiscussMetadataConfidence, "medium"),
		Blockers:     blockers,
		ArtifactPath: discussMetadata(request, DiscussMetadataArtifactPath, defaultDiscussArtifactPath),
		NextAction:   discussMetadata(request, DiscussMetadataNextAction, defaultDiscussNextAction(blockers)),
		SourceRefs:   discussSourceRefs(request),
	}
	discussion.Document = renderDiscussDocument(discussion)
	return discussion
}

func discussQuestion(request Request) string {
	if value := discussMetadata(request, DiscussMetadataQuestion, ""); value != "" {
		return value
	}
	return strings.TrimSpace(request.Input)
}

func discussOptions(request Request) []DiscussOption {
	items := discussListMetadata(request, DiscussMetadataOptions)
	if len(items) == 0 {
		items = []string{"Plan the scoped next step", "Revisit project vision", "Proceed directly to build"}
	}
	options := make([]DiscussOption, 0, len(items))
	for index, item := range items {
		options = append(options, DiscussOption{
			ID:        fmt.Sprintf("option-%d", index+1),
			Text:      item,
			Rationale: "considered for the decision record",
		})
	}
	return options
}

func discussSelected(request Request, options []DiscussOption) string {
	selected := discussMetadata(request, DiscussMetadataSelected, "")
	if selected != "" {
		return selected
	}
	if len(options) == 0 {
		return ""
	}
	return options[0].Text
}

func discussRecommendedSuccessor(request Request, signal ExitSignal) workflow.Phase {
	preferred := workflow.Phase(discussMetadata(request, DiscussMetadataRecommendedNext, ""))
	if preferred == "" {
		if signal == ExitFlagged {
			preferred = workflow.PhaseEnvision
		} else {
			preferred = workflow.PhasePlan
		}
	}
	if workflow.ValidateProtocolSuccessor(request.Phase, preferred) == nil {
		return preferred
	}
	return ""
}

func defaultDiscussReasoning(selected string) string {
	if selected == "" {
		return "No option was selected."
	}
	return selected + " keeps the next step bounded and preserves workflow authority."
}

func defaultDiscussNextAction(blockers []string) string {
	if len(blockers) > 0 {
		return "Resolve the decision blockers before changing workflow direction."
	}
	return "Use this decision as source material for planning."
}

func discussSummary(discussion DiscussOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return fmt.Sprintf("Discuss recorded a decision with %d blocker(s).", len(discussion.Blockers))
	}
	return "Discuss recorded a consequential decision."
}

func renderDiscussDocument(discussion DiscussOutput) string {
	var builder strings.Builder
	builder.WriteString("# Decision\n\n")
	builder.WriteString("Question: ")
	builder.WriteString(discussion.Question)
	builder.WriteString("\n\n")
	if discussion.Context != "" {
		builder.WriteString("Context: ")
		builder.WriteString(discussion.Context)
		builder.WriteString("\n\n")
	}
	builder.WriteString("## Options\n\n")
	for _, option := range discussion.Options {
		marker := " "
		if option.Selected {
			marker = "x"
		}
		builder.WriteString("- [")
		builder.WriteString(marker)
		builder.WriteString("] ")
		builder.WriteString(option.Text)
		if option.Rationale != "" {
			builder.WriteString(" -- ")
			builder.WriteString(option.Rationale)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nChoice: ")
	builder.WriteString(discussion.Selected)
	builder.WriteString("\n\nReasoning: ")
	builder.WriteString(discussion.Reasoning)
	builder.WriteString("\n\nConfidence: ")
	builder.WriteString(discussion.Confidence)
	builder.WriteString("\n")
	if len(discussion.Blockers) > 0 {
		builder.WriteString("\n## Blockers\n\n")
		for _, blocker := range discussion.Blockers {
			builder.WriteString("- ")
			builder.WriteString(blocker)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nNext action: ")
	builder.WriteString(discussion.NextAction)
	builder.WriteString("\n")
	return builder.String()
}

func discussBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("project.current", "discuss requires app-supplied project state evidence"),
		request.RequestStateAccess("session.current", "discuss requires app-supplied session state evidence"),
		request.RequestContextAccess("current_context", "discuss uses supplied context evidence"),
		request.RequestArtifactAccess("decisions", "decision artifact path must be resolved through the state store"),
		request.RequestStateWrite("decisions", "decision artifact persistence must be app-owned and store-mediated"),
	}
}

func discussSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	if len(refs) == 0 {
		refs = append(refs, SourceRef{ID: "discuss-input", Kind: "prompt", Excerpt: discussQuestion(request)})
	}
	contextSummary := strings.TrimSpace(request.Metadata[DiscussMetadataContextSummary])
	if contextSummary != "" {
		refs = append(refs, SourceRef{ID: "discuss-context", Kind: "context", Excerpt: contextSummary})
	}
	return refs
}

func discussMetadata(request Request, key string, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func discussListMetadata(request Request, key string) []string {
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
