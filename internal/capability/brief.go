package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	BriefMetadataRuntimeStatus       = "runtime_status"
	BriefMetadataProjectStoreStatus  = "project_store_status"
	BriefMetadataHistoryState        = "history_state"
	BriefMetadataHistoryEvents       = "history_events"
	BriefMetadataLatestHistory       = "latest_history"
	BriefMetadataContextStatus       = "context_status"
	BriefMetadataContextSummary      = "context_summary"
	BriefMetadataHealthStatus        = "health_status"
	BriefMetadataSuggestedNextAction = "suggested_next_action"
)

// BriefCapability gives a bounded orientation over app-supplied state evidence.
type BriefCapability struct{}

// Name returns the fixed capability identity.
func (BriefCapability) Name() Name {
	return NameBrief
}

// OwningPhase returns IDLE because brief is cross-cutting and non-transitioning.
func (BriefCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseIdle
}

// Run emits one brief payload without reading state or mutating workflow phase.
func (BriefCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run brief capability: %w", err)
	}
	request = normalizeBriefRequest(request)
	invocation := NewInvocation(request)
	payload := ExitPayload{
		Capability:       NameBrief,
		Signal:           ExitComplete,
		Summary:          briefSummary(request),
		Concerns:         briefKnownGaps(request),
		NextAction:       briefNextAction(request),
		SourceRefs:       append([]SourceRef(nil), request.SourceRefs...),
		BoundaryRequests: briefBoundaryRequests(request),
	}
	return invocation.Emit(payload)
}

// RunBuiltIn runs a fixed built-in capability by name.
func RunBuiltIn(ctx context.Context, request Request) (ExitPayload, error) {
	if request.Capability == "" {
		request.Capability = NameBrief
	}
	switch request.Capability {
	case NameBrief:
		return BriefCapability{}.Run(ctx, request)
	case NameVision:
		return VisionCapability{}.Run(ctx, request)
	case NameDiscuss:
		return DiscussCapability{}.Run(ctx, request)
	case NameResearch:
		return ResearchCapability{}.Run(ctx, request)
	case NameProfile:
		return ProfileCapability{}.Run(ctx, request)
	case NamePlan:
		return PlanCapability{}.Run(ctx, request)
	case NameBuild:
		return BuildCapability{}.Run(ctx, request)
	case NameOptimize:
		return OptimizeCapability{}.Run(ctx, request)
	case NameAudit:
		return AuditCapability{}.Run(ctx, request)
	default:
		return ExitPayload{}, fmt.Errorf("unsupported built-in capability %q", request.Capability)
	}
}

func normalizeBriefRequest(request Request) Request {
	request.Capability = NameBrief
	if request.Phase == "" {
		request.Phase = workflow.PhaseIdle
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func briefSummary(request Request) string {
	return "Brief: phase " + request.Phase.String() +
		", runtime " + briefMetadata(request, BriefMetadataRuntimeStatus, "unknown") +
		", store " + briefMetadata(request, BriefMetadataProjectStoreStatus, "unknown") +
		", history " + briefHistoryLabel(request) +
		", context " + briefContextLabel(request) +
		", health " + briefMetadata(request, BriefMetadataHealthStatus, "unavailable") + "."
}

func briefHistoryLabel(request Request) string {
	state := briefMetadata(request, BriefMetadataHistoryState, "unavailable")
	events := strings.TrimSpace(request.Metadata[BriefMetadataHistoryEvents])
	if events == "" {
		return state
	}
	return state + " (" + events + " events)"
}

func briefContextLabel(request Request) string {
	status := briefMetadata(request, BriefMetadataContextStatus, "unavailable")
	summary := strings.TrimSpace(request.Metadata[BriefMetadataContextSummary])
	if summary == "" {
		return status
	}
	return status + " (" + summary + ")"
}

func briefKnownGaps(request Request) []string {
	checks := []struct {
		key   string
		label string
	}{
		{BriefMetadataRuntimeStatus, "runtime status unavailable"},
		{BriefMetadataProjectStoreStatus, "project store status unavailable"},
		{BriefMetadataHistoryState, "history unavailable"},
		{BriefMetadataContextStatus, "context unavailable"},
		{BriefMetadataHealthStatus, "health unavailable"},
	}
	var gaps []string
	for _, check := range checks {
		value := strings.ToLower(strings.TrimSpace(request.Metadata[check.key]))
		if value == "" || value == "unknown" || value == "unavailable" || value == "recovery_needed" {
			gaps = append(gaps, check.label)
		}
	}
	return gaps
}

func briefNextAction(request Request) string {
	if next := strings.TrimSpace(request.Metadata[BriefMetadataSuggestedNextAction]); next != "" {
		return next
	}
	status := strings.ToLower(strings.TrimSpace(request.Metadata[BriefMetadataRuntimeStatus]))
	if status != "" && status != "idle" {
		return "Let the current runtime work finish or interrupt it before starting another capability."
	}
	if len(briefKnownGaps(request)) > 0 {
		return "Review the missing evidence, then choose the next capability."
	}
	return "Choose the next capability from the current goal."
}

func briefBoundaryRequests(request Request) []BoundaryRequest {
	return []BoundaryRequest{
		request.RequestStateAccess("runtime.current", "brief requires the current runtime and workflow phase from the runtime boundary"),
		request.RequestArtifactAccess("current_session_snapshot", "brief uses store-resolved session state when available"),
		request.RequestArtifactAccess("fake_history", "brief uses store-resolved recent history when available"),
		request.RequestContextAccess("current_context", "brief uses app-supplied context evidence when available"),
		request.RequestArtifactAccess("health", "brief uses store-resolved health evidence when available"),
	}
}

func briefMetadata(request Request, key, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}
