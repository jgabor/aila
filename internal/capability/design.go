package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	DesignMetadataGoalID               = "design_goal_id"
	DesignMetadataGoalSummary          = "design_goal_summary"
	DesignMetadataSurface              = "design_surface"
	DesignMetadataDecisions            = "design_decisions"
	DesignMetadataReviewPrompts        = "design_review_prompts"
	DesignMetadataCaveats              = "design_caveats"
	DesignMetadataNextAction           = "design_next_action"
	DesignMetadataDurable              = "design_durable"
	DesignMetadataVisualReviewRequired = "design_visual_review_required"
	DesignMetadataArtifactStatus       = "design_artifact_status"
)

const defaultDesignArtifactPath = ".aila/artifacts/design.md"

// DesignCapability adapts app-supplied visual identity evidence into BUILD-owned design output.
type DesignCapability struct{}

// DesignOutput is the typed UI-system design data carried by a design capability exit.
type DesignOutput struct {
	Goal                 DesignGoal
	Decisions            []DesignDecision
	ReviewPrompts        []DesignReviewPrompt
	Caveats              []string
	NextAction           string
	VisualReviewRequired bool
	DesignArtifactPath   string
	DesignArtifact       string
	SourceRefs           []SourceRef
}

// DesignGoal records the bounded design target.
type DesignGoal struct {
	ID      string
	Summary string
	Surface string
}

// DesignDecision records one durable design-system decision.
type DesignDecision struct {
	ID        string
	Area      string
	Decision  string
	Rationale string
}

// DesignReviewPrompt records one human-review prompt without making screenshots authoritative.
type DesignReviewPrompt struct {
	ID       string
	Question string
	Target   string
}

// Name returns the fixed capability identity.
func (DesignCapability) Name() Name {
	return NameDesign
}

// OwningPhase returns BUILD because visual identity and UI-system work is build-owned product work.
func (DesignCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseBuild
}

// Run emits one design payload. Artifact writes must be handled by app/runtime effects.
func (DesignCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run design capability: %w", err)
	}
	request = normalizeDesignRequest(request)
	invocation := NewInvocation(request)

	if !hasDesignEvidence(request) {
		payload := ExitPayload{
			Capability:       NameDesign,
			Signal:           ExitWaiting,
			Summary:          "Design needs a design goal and durable decisions before recording UI-system work.",
			Concerns:         []string{"design evidence unavailable until goal and decision records are provided"},
			NeededInput:      "Provide a design goal and durable design decisions before designing.",
			NextAction:       "Provide design evidence, then run design again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: designBoundaryRequests(request, false),
		}
		return invocation.Emit(payload)
	}

	output := buildDesignOutput(request)
	successor := workflow.Phase("")
	if workflow.ValidateProtocolSuccessor(request.Phase, workflow.PhaseAudit) == nil {
		successor = workflow.PhaseAudit
	}

	payload := ExitPayload{
		Capability:           NameDesign,
		Signal:               ExitComplete,
		Summary:              designSummary(output),
		Concerns:             append([]string(nil), output.Caveats...),
		Attempted:            len(output.Decisions) > 0,
		NextAction:           output.NextAction,
		RecommendedSuccessor: successor,
		ArtifactRefs:         designArtifactRefs(output),
		SourceRefs:           cloneSourceRefs(output.SourceRefs),
		BoundaryRequests:     designBoundaryRequests(request, output.DesignArtifact != ""),
		Design:               &output,
	}
	return invocation.Emit(payload)
}

func normalizeDesignRequest(request Request) Request {
	request.Capability = NameDesign
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseBuild
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func hasDesignEvidence(request Request) bool {
	return designMetadata(request, DesignMetadataGoalSummary, "") != "" && len(designDecisions(request)) > 0
}

func buildDesignOutput(request Request) DesignOutput {
	goal := DesignGoal{
		ID:      designMetadata(request, DesignMetadataGoalID, "aila-design-system"),
		Summary: designMetadata(request, DesignMetadataGoalSummary, "Create durable design decisions for Aila's terminal UI system."),
		Surface: designMetadata(request, DesignMetadataSurface, "terminal-ui"),
	}
	caveats := designListMetadata(request, DesignMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"deterministic app-supplied design evidence only", "screenshots are review aids, not correctness contracts"}
	}
	output := DesignOutput{
		Goal:                 goal,
		Decisions:            designDecisions(request),
		ReviewPrompts:        designReviewPrompts(request),
		Caveats:              caveats,
		NextAction:           designMetadata(request, DesignMetadataNextAction, "Audit the design-system artifact before continuing."),
		VisualReviewRequired: designBoolMetadata(request, DesignMetadataVisualReviewRequired),
		DesignArtifactPath:   defaultDesignArtifactPath,
		SourceRefs:           designSourceRefs(request),
	}
	if designBoolMetadata(request, DesignMetadataDurable) {
		output.DesignArtifact = designArtifactDocument(output)
	}
	return output
}

func designDecisions(request Request) []DesignDecision {
	entries := designListMetadata(request, DesignMetadataDecisions)
	decisions := make([]DesignDecision, 0, len(entries))
	for _, entry := range entries {
		parts := splitDesignFields(entry, 4)
		if parts[0] == "" && parts[2] == "" {
			continue
		}
		decisions = append(decisions, DesignDecision{
			ID:        defaultString(parts[0], fmt.Sprintf("design-decision-%d", len(decisions)+1)),
			Area:      parts[1],
			Decision:  parts[2],
			Rationale: parts[3],
		})
	}
	return decisions
}

func designReviewPrompts(request Request) []DesignReviewPrompt {
	entries := designListMetadata(request, DesignMetadataReviewPrompts)
	prompts := make([]DesignReviewPrompt, 0, len(entries))
	for _, entry := range entries {
		parts := splitDesignFields(entry, 3)
		if parts[0] == "" && parts[1] == "" {
			continue
		}
		prompts = append(prompts, DesignReviewPrompt{
			ID:       defaultString(parts[0], fmt.Sprintf("design-review-%d", len(prompts)+1)),
			Question: parts[1],
			Target:   parts[2],
		})
	}
	return prompts
}

func splitDesignFields(entry string, count int) []string {
	parts := strings.Split(entry, "::")
	fields := make([]string, count)
	for index := range fields {
		if index < len(parts) {
			fields[index] = strings.TrimSpace(parts[index])
		}
	}
	return fields
}

func designSummary(output DesignOutput) string {
	return fmt.Sprintf("Design recorded %d decisions for %s.", len(output.Decisions), defaultString(output.Goal.ID, "current design goal"))
}

func designArtifactRefs(output DesignOutput) []ArtifactRef {
	refs := []ArtifactRef{{ID: "design-artifact", Kind: "design", Path: output.DesignArtifactPath}}
	if output.Goal.Surface != "" {
		refs = append(refs, ArtifactRef{ID: "design-surface", Kind: "ui_surface", Path: output.Goal.Surface})
	}
	return refs
}

func designSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	if len(refs) == 0 {
		refs = append(refs, SourceRef{ID: "design-input", Kind: "capability_input", Excerpt: designMetadata(request, DesignMetadataGoalSummary, request.Input)})
	}
	return refs
}

func designBoundaryRequests(request Request, durable bool) []BoundaryRequest {
	requests := []BoundaryRequest{
		request.RequestStateAccess("design.current", "design uses app-supplied visual identity and UI-system evidence"),
		request.RequestArtifactAccess("design", "state store resolves durable design-system output"),
	}
	if durable {
		requests = append(requests, request.RequestStateWrite("design", "state store records durable design-system output"))
	}
	return requests
}

func designArtifactDocument(output DesignOutput) string {
	var builder strings.Builder
	builder.WriteString("# Design System\n\n")
	builder.WriteString("Goal: ")
	builder.WriteString(output.Goal.Summary)
	builder.WriteString("\n")
	if output.Goal.Surface != "" {
		builder.WriteString("Surface: ")
		builder.WriteString(output.Goal.Surface)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Decisions\n")
	for _, decision := range output.Decisions {
		builder.WriteString("- ")
		builder.WriteString(decision.ID)
		if decision.Area != "" {
			builder.WriteString(" [")
			builder.WriteString(decision.Area)
			builder.WriteString("]")
		}
		builder.WriteString(": ")
		builder.WriteString(decision.Decision)
		if decision.Rationale != "" {
			builder.WriteString(" - ")
			builder.WriteString(decision.Rationale)
		}
		builder.WriteString("\n")
	}
	if len(output.ReviewPrompts) > 0 {
		builder.WriteString("\n## Visual Review Prompts\n")
		for _, prompt := range output.ReviewPrompts {
			builder.WriteString("- ")
			builder.WriteString(prompt.ID)
			builder.WriteString(": ")
			builder.WriteString(prompt.Question)
			if prompt.Target != "" {
				builder.WriteString(" (")
				builder.WriteString(prompt.Target)
				builder.WriteString(")")
			}
			builder.WriteString("\n")
		}
	}
	if len(output.Caveats) > 0 {
		builder.WriteString("\n## Caveats\n")
		for _, caveat := range output.Caveats {
			builder.WriteString("- ")
			builder.WriteString(caveat)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nNext action: ")
	builder.WriteString(output.NextAction)
	builder.WriteString("\n")
	return builder.String()
}

func designMetadata(request Request, key string, fallback string) string {
	if request.Metadata == nil {
		return strings.TrimSpace(fallback)
	}
	if value := strings.TrimSpace(request.Metadata[key]); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func designListMetadata(request Request, key string) []string {
	value := designMetadata(request, key, "")
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

func designBoolMetadata(request Request, key string) bool {
	value := strings.ToLower(designMetadata(request, key, ""))
	return value == "true" || value == "yes" || value == "1"
}
