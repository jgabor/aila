package policy

import (
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/workflow"
)

// CapabilityRouteSource identifies the policy input family behind a candidate.
type CapabilityRouteSource string

const (
	CapabilityRouteExplicitSlash   CapabilityRouteSource = "explicit_slash"
	CapabilityRouteNaturalLanguage CapabilityRouteSource = "natural_language"
	CapabilityRouteWaiting         CapabilityRouteSource = "waiting"
	CapabilityRouteSuccessorCheck  CapabilityRouteSource = "successor_validation"
)

// CapabilityRecommendation is a pure policy recommendation, not a phase change.
type CapabilityRecommendation struct {
	Source               CapabilityRouteSource
	Input                string
	Candidate            capability.Name
	Confidence           int
	Reason               string
	NeededInput          string
	CurrentPhase         workflow.Phase
	RuntimeStatus        workflow.RuntimeStatus
	RecommendedSuccessor workflow.Phase
	SuccessorValid       bool
	SuccessorRejected    bool
	SuccessorReason      string
	SourceRefs           []capability.SourceRef
	BoundaryRequests     []capability.BoundaryRequest
	TransitionClaimed    bool
}

var explicitCapabilityRoutes = map[string]capability.Name{
	"/brief":       capability.NameBrief,
	"/vision":      capability.NameVision,
	"/discuss":     capability.NameDiscuss,
	"/research":    capability.NameResearch,
	"/plan":        capability.NamePlan,
	"/build":       capability.NameBuild,
	"/optimize":    capability.NameOptimize,
	"/document":    capability.NameDocument,
	"/design":      capability.NameDesign,
	"/audit":       capability.NameAudit,
	"/profile":     capability.NameProfile,
	"/orchestrate": capability.NameOrchestrate,
}

// RecommendCapability maps user intent to a typed capability candidate only.
func RecommendCapability(input string, current workflow.Phase) (CapabilityRecommendation, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return CapabilityRecommendation{}, false
	}
	if candidate, ok := explicitCapabilityRoutes[trimmed]; ok {
		return CapabilityRecommendation{
			Source:       CapabilityRouteExplicitSlash,
			Input:        trimmed,
			Candidate:    candidate,
			Confidence:   100,
			Reason:       "exact capability slash route",
			CurrentPhase: current,
			SourceRefs:   sourceRefsForInput("policy-explicit-route", trimmed),
		}, true
	}
	if strings.HasPrefix(trimmed, "/") {
		return CapabilityRecommendation{}, false
	}

	lower := strings.ToLower(trimmed)
	for _, route := range []struct {
		candidate  capability.Name
		confidence int
		reason     string
		needles    []string
	}{
		{candidate: capability.NamePlan, confidence: 88, reason: "planning intent matched", needles: []string{"plan", "acceptance", "milestone", "roadmap"}},
		{candidate: capability.NameBuild, confidence: 88, reason: "implementation intent matched", needles: []string{"build", "implement", "fix", "code"}},
		{candidate: capability.NameAudit, confidence: 86, reason: "audit intent matched", needles: []string{"audit", "review", "risk", "quality"}},
		{candidate: capability.NameDiscuss, confidence: 84, reason: "deliberation intent matched", needles: []string{"decide", "discuss", "tradeoff", "should we"}},
		{candidate: capability.NameBrief, confidence: 78, reason: "status briefing intent matched", needles: []string{"status", "where are we", "next action", "summary"}},
	} {
		if containsCapabilityNeedle(lower, route.needles) {
			return CapabilityRecommendation{
				Source:       CapabilityRouteNaturalLanguage,
				Input:        trimmed,
				Candidate:    route.candidate,
				Confidence:   route.confidence,
				Reason:       route.reason,
				CurrentPhase: current,
				SourceRefs:   sourceRefsForInput("policy-natural-language-route", trimmed),
			}, true
		}
	}

	return CapabilityRecommendation{
		Source:        CapabilityRouteWaiting,
		Input:         trimmed,
		Candidate:     capability.NameBrief,
		Confidence:    42,
		Reason:        "low confidence capability route",
		NeededInput:   "Clarify whether you want a brief, discussion, plan, build, or audit.",
		CurrentPhase:  current,
		RuntimeStatus: workflow.RuntimeStatusWaiting,
		SourceRefs:    sourceRefsForInput("policy-low-confidence-route", trimmed),
	}, true
}

// RecommendCapabilitySuccessor validates a successor recommendation without changing phase.
func RecommendCapabilitySuccessor(current workflow.Phase, payload capability.ExitPayload) CapabilityRecommendation {
	recommendation := CapabilityRecommendation{
		Source:               CapabilityRouteSuccessorCheck,
		Candidate:            payload.Capability,
		Confidence:           100,
		Reason:               "capability exit successor requires workflow FSM validation",
		CurrentPhase:         current,
		RecommendedSuccessor: payload.RecommendedSuccessor,
		SourceRefs:           cloneSourceRefs(payload.SourceRefs),
		BoundaryRequests:     cloneBoundaryRequests(payload.BoundaryRequests),
	}
	if payload.RecommendedSuccessor == "" {
		recommendation.SuccessorReason = "no successor recommended"
		return recommendation
	}
	if err := workflow.ValidateProtocolSuccessor(current, payload.RecommendedSuccessor); err != nil {
		recommendation.SuccessorRejected = true
		recommendation.SuccessorReason = err.Error()
		return recommendation
	}
	recommendation.SuccessorValid = true
	recommendation.SuccessorReason = "workflow FSM accepted recommended successor"
	return recommendation
}

func containsCapabilityNeedle(input string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(input, needle) {
			return true
		}
	}
	return false
}

func sourceRefsForInput(id string, input string) []capability.SourceRef {
	return []capability.SourceRef{{ID: id, Kind: "prompt", Excerpt: input}}
}

func cloneSourceRefs(refs []capability.SourceRef) []capability.SourceRef {
	return append([]capability.SourceRef(nil), refs...)
}

func cloneBoundaryRequests(requests []capability.BoundaryRequest) []capability.BoundaryRequest {
	clone := append([]capability.BoundaryRequest(nil), requests...)
	for index := range clone {
		if len(clone[index].Metadata) == 0 {
			continue
		}
		metadata := make(map[string]string, len(clone[index].Metadata))
		for key, value := range clone[index].Metadata {
			metadata[key] = value
		}
		clone[index].Metadata = metadata
	}
	return clone
}
