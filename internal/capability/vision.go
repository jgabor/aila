package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	VisionMetadataNorthStar       = "vision_north_star"
	VisionMetadataPrinciples      = "vision_principles"
	VisionMetadataLongTermGoals   = "vision_long_term_goals"
	VisionMetadataBlockers        = "vision_blockers"
	VisionMetadataArtifactPath    = "vision_artifact_path"
	VisionMetadataRecommendedNext = "vision_recommended_successor"
	VisionMetadataNextAction      = "vision_next_action"
	VisionMetadataContextSummary  = "vision_context_summary"
)

const defaultVisionArtifactPath = ".aila/artifacts/vision.md"

// VisionCapability shapes project direction from app-supplied goal evidence.
type VisionCapability struct{}

// VisionOutput is the typed vision data carried by a vision capability exit.
type VisionOutput struct {
	NorthStar     string
	Principles    []string
	LongTermGoals []string
	Blockers      []string
	ArtifactPath  string
	NextAction    string
	Document      string
	SourceRefs    []SourceRef
}

// Name returns the fixed capability identity.
func (VisionCapability) Name() Name {
	return NameVision
}

// OwningPhase returns ENVISION because the capability shapes project direction.
func (VisionCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseEnvision
}

// Run emits one vision payload without reading files, calling providers, or mutating workflow phase.
func (VisionCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run vision capability: %w", err)
	}
	request = normalizeVisionRequest(request)
	invocation := NewInvocation(request)
	if visionDirection(request) == "" {
		payload := ExitPayload{
			Capability:       NameVision,
			Signal:           ExitWaiting,
			Summary:          "Vision needs project direction before it can shape goals.",
			NeededInput:      "Describe the project direction or long-term goal to shape.",
			NextAction:       "Provide direction, then run vision again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: visionBoundaryRequests(request),
		}
		return invocation.Emit(payload)
	}

	vision := buildVisionOutput(request)
	signal := ExitComplete
	if len(vision.Blockers) > 0 {
		signal = ExitFlagged
	}
	successor := visionRecommendedSuccessor(request, signal)
	payload := ExitPayload{
		Capability:           NameVision,
		Signal:               signal,
		Summary:              visionSummary(vision, signal),
		Concerns:             append([]string(nil), vision.Blockers...),
		Attempted:            true,
		NextAction:           vision.NextAction,
		RecommendedSuccessor: successor,
		ArtifactRefs: []ArtifactRef{{
			ID:   "vision-artifact",
			Kind: "state_artifact",
			Path: vision.ArtifactPath,
		}},
		SourceRefs:       cloneSourceRefs(vision.SourceRefs),
		BoundaryRequests: visionBoundaryRequests(request),
		Vision:           &vision,
	}
	return invocation.Emit(payload)
}

func normalizeVisionRequest(request Request) Request {
	request.Capability = NameVision
	if request.Phase == "" || request.Phase == workflow.PhaseIdle {
		request.Phase = workflow.PhaseEnvision
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func buildVisionOutput(request Request) VisionOutput {
	northStar := visionDirection(request)
	principles := visionListMetadata(request, VisionMetadataPrinciples)
	if len(principles) == 0 {
		principles = []string{
			"Keep Aila a fixed terminal coding agent rather than a plugin host.",
			"Preserve statechart-MVU boundaries and explicit effects.",
			"Prefer visible evidence and reversible workflow steps.",
		}
	}
	goals := visionListMetadata(request, VisionMetadataLongTermGoals)
	if len(goals) == 0 {
		goals = []string{
			"Shape goals before planning or building broad changes.",
			"Use persisted vision as source material for later plan and build work.",
		}
	}
	blockers := visionListMetadata(request, VisionMetadataBlockers)
	nextAction := visionMetadata(request, VisionMetadataNextAction, defaultVisionNextAction(blockers))
	vision := VisionOutput{
		NorthStar:     northStar,
		Principles:    principles,
		LongTermGoals: goals,
		Blockers:      blockers,
		ArtifactPath:  visionMetadata(request, VisionMetadataArtifactPath, defaultVisionArtifactPath),
		NextAction:    nextAction,
		SourceRefs:    visionSourceRefs(request),
	}
	vision.Document = renderVisionDocument(vision)
	return vision
}

func visionDirection(request Request) string {
	if value := visionMetadata(request, VisionMetadataNorthStar, ""); value != "" {
		return value
	}
	return strings.TrimSpace(request.Input)
}

func visionRecommendedSuccessor(request Request, signal ExitSignal) workflow.Phase {
	preferred := workflow.Phase(visionMetadata(request, VisionMetadataRecommendedNext, ""))
	if preferred == "" {
		if signal == ExitFlagged {
			preferred = workflow.PhaseDeliberate
		} else {
			preferred = workflow.PhasePlan
		}
	}
	if workflow.ValidateProtocolSuccessor(request.Phase, preferred) == nil {
		return preferred
	}
	return ""
}

func defaultVisionNextAction(blockers []string) string {
	if len(blockers) > 0 {
		return "Resolve the vision blockers or deliberate before planning."
	}
	return "Use this vision as source material for planning."
}

func visionSummary(vision VisionOutput, signal ExitSignal) string {
	if signal == ExitFlagged {
		return fmt.Sprintf("Vision shaped goals with %d blocker(s) before planning.", len(vision.Blockers))
	}
	return "Vision shaped project direction and long-term goals."
}

func renderVisionDocument(vision VisionOutput) string {
	var builder strings.Builder
	builder.WriteString("# Vision\n\n")
	builder.WriteString("North star: ")
	builder.WriteString(vision.NorthStar)
	builder.WriteString("\n\n")
	builder.WriteString("## Principles\n\n")
	for _, principle := range vision.Principles {
		builder.WriteString("- ")
		builder.WriteString(principle)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Long-term goals\n\n")
	for _, goal := range vision.LongTermGoals {
		builder.WriteString("- ")
		builder.WriteString(goal)
		builder.WriteString("\n")
	}
	if len(vision.Blockers) > 0 {
		builder.WriteString("\n## Blockers\n\n")
		for _, blocker := range vision.Blockers {
			builder.WriteString("- ")
			builder.WriteString(blocker)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nNext action: ")
	builder.WriteString(vision.NextAction)
	builder.WriteString("\n")
	return builder.String()
}

func visionBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("project.current", "vision requires app-supplied project state evidence"),
		request.RequestStateAccess("session.current", "vision requires app-supplied session state evidence"),
		request.RequestContextAccess("current_context", "vision uses supplied context evidence"),
		request.RequestArtifactAccess("vision", "vision artifact path must be resolved through the state store"),
		request.RequestStateWrite("vision", "vision artifact persistence must be app-owned and store-mediated"),
	}
}

func visionSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	if len(refs) == 0 {
		refs = append(refs, SourceRef{ID: "vision-input", Kind: "prompt", Excerpt: visionDirection(request)})
	}
	contextSummary := strings.TrimSpace(request.Metadata[VisionMetadataContextSummary])
	if contextSummary != "" {
		refs = append(refs, SourceRef{ID: "vision-context", Kind: "context", Excerpt: contextSummary})
	}
	return refs
}

func visionMetadata(request Request, key string, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func visionListMetadata(request Request, key string) []string {
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
