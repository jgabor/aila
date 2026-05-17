package capability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/workflow"
)

const (
	ProfileMetadataSubject           = "profile_subject"
	ProfileMetadataContext           = "profile_context"
	ProfileMetadataDecisionSignals   = "profile_decision_signals"
	ProfileMetadataUpdateSuggestions = "profile_update_suggestions"
	ProfileMetadataEvidence          = "profile_evidence"
	ProfileMetadataConfidence        = "profile_confidence"
	ProfileMetadataCaveats           = "profile_caveats"
	ProfileMetadataNextAction        = "profile_next_action"
	ProfileMetadataContextSummary    = "profile_context_summary"
	ProfileMetadataDurable           = "profile_durable"
)

const defaultProfileArtifactPath = ".aila/artifacts/profile.md"

// ProfileCapability adapts decision-profile evidence into non-authoritative context.
type ProfileCapability struct{}

// ProfileDecisionSignal records one decision pattern available as profile context.
type ProfileDecisionSignal struct {
	ID             string
	Pattern        string
	Guidance       string
	EvidenceRefIDs []string
}

// ProfileUpdateSuggestion records one suggested profile update.
type ProfileUpdateSuggestion struct {
	ID             string
	Text           string
	Rationale      string
	EvidenceRefIDs []string
}

// ProfileEvidence records one source-backed profile observation.
type ProfileEvidence struct {
	ID          string
	Summary     string
	SourceRefID string
}

// ProfileOutput is the typed context-folding data carried by a profile capability exit.
type ProfileOutput struct {
	Subject            string
	CurrentPhase       string
	CrossCuttingStatus string
	Context            string
	DecisionSignals    []ProfileDecisionSignal
	UpdateSuggestions  []ProfileUpdateSuggestion
	Evidence           []ProfileEvidence
	Confidence         string
	Caveats            []string
	NextAction         string
	ContextSummary     string
	ArtifactPath       string
	Document           string
	SourceRefs         []SourceRef
}

// Name returns the fixed capability identity.
func (ProfileCapability) Name() Name {
	return NameProfile
}

// OwningPhase returns IDLE because profile is explicitly cross-cutting.
func (ProfileCapability) OwningPhase() workflow.Phase {
	return workflow.PhaseIdle
}

// Run emits one profile payload without fetching corpus data, executing tools, or mutating workflow phase.
func (ProfileCapability) Run(ctx context.Context, request Request) (ExitPayload, error) {
	if err := ctx.Err(); err != nil {
		return ExitPayload{}, fmt.Errorf("run profile capability: %w", err)
	}
	request = normalizeProfileRequest(request)
	invocation := NewInvocation(request)
	if profileSubject(request) == "" || !hasProfileEvidence(request) {
		payload := ExitPayload{
			Capability:       NameProfile,
			Signal:           ExitWaiting,
			Summary:          "Profile needs session evidence before it can update decision context.",
			Concerns:         []string{"profile evidence unavailable until session evidence is provided"},
			NeededInput:      "Provide session or decision evidence to profile.",
			NextAction:       "Provide profile evidence, then run profile again.",
			SourceRefs:       cloneSourceRefs(request.SourceRefs),
			BoundaryRequests: profileBoundaryRequests(request, false),
		}
		return invocation.Emit(payload)
	}

	output := buildProfileOutput(request)
	payload := ExitPayload{
		Capability:       NameProfile,
		Signal:           ExitComplete,
		Summary:          profileSummary(output),
		Concerns:         append([]string(nil), output.Caveats...),
		Attempted:        true,
		NextAction:       output.NextAction,
		ArtifactRefs:     profileArtifactRefs(output),
		SourceRefs:       cloneSourceRefs(output.SourceRefs),
		BoundaryRequests: profileBoundaryRequests(request, output.Document != ""),
		Profile:          &output,
	}
	return invocation.Emit(payload)
}

func normalizeProfileRequest(request Request) Request {
	request.Capability = NameProfile
	if request.Phase == "" {
		request.Phase = workflow.PhaseIdle
	}
	request.Metadata = cloneMap(request.Metadata)
	return request
}

func buildProfileOutput(request Request) ProfileOutput {
	sourceRefs := profileSourceRefs(request)
	evidence := profileEvidence(request, sourceRefs)
	signals := profileDecisionSignals(request, evidence)
	suggestions := profileUpdateSuggestions(request, evidence)
	caveats := profileListMetadata(request, ProfileMetadataCaveats)
	if len(caveats) == 0 {
		caveats = []string{"profile used app-supplied session evidence only"}
	}
	output := ProfileOutput{
		Subject:            profileSubject(request),
		CurrentPhase:       request.Phase.String(),
		CrossCuttingStatus: "context_only",
		Context:            profileMetadata(request, ProfileMetadataContext, profileMetadata(request, ProfileMetadataContextSummary, "")),
		DecisionSignals:    signals,
		UpdateSuggestions:  suggestions,
		Evidence:           evidence,
		Confidence:         profileMetadata(request, ProfileMetadataConfidence, "medium"),
		Caveats:            caveats,
		NextAction:         profileMetadata(request, ProfileMetadataNextAction, "Use this profile as non-authoritative context for the current workflow phase."),
		ContextSummary:     profileContextSummary(request, signals, suggestions, evidence, caveats),
		SourceRefs:         sourceRefs,
	}
	if profileDurable(request) {
		output.ArtifactPath = defaultProfileArtifactPath
		output.Document = profileDocument(output)
	}
	return output
}

func profileSubject(request Request) string {
	if subject := profileMetadata(request, ProfileMetadataSubject, ""); subject != "" {
		return subject
	}
	return strings.TrimSpace(request.Input)
}

func hasProfileEvidence(request Request) bool {
	return len(profileListMetadata(request, ProfileMetadataDecisionSignals)) > 0 || len(profileListMetadata(request, ProfileMetadataUpdateSuggestions)) > 0 || len(profileListMetadata(request, ProfileMetadataEvidence)) > 0
}

func profileDecisionSignals(request Request, evidence []ProfileEvidence) []ProfileDecisionSignal {
	items := profileListMetadata(request, ProfileMetadataDecisionSignals)
	signals := make([]ProfileDecisionSignal, 0, len(items))
	for index, item := range items {
		refID := ""
		if len(evidence) > 0 {
			refID = evidence[index%len(evidence)].SourceRefID
		}
		signal := ProfileDecisionSignal{
			ID:       fmt.Sprintf("signal-%d", index+1),
			Pattern:  item,
			Guidance: "use as context, not workflow authority",
		}
		if refID != "" {
			signal.EvidenceRefIDs = []string{refID}
		}
		signals = append(signals, signal)
	}
	return signals
}

func profileUpdateSuggestions(request Request, evidence []ProfileEvidence) []ProfileUpdateSuggestion {
	items := profileListMetadata(request, ProfileMetadataUpdateSuggestions)
	suggestions := make([]ProfileUpdateSuggestion, 0, len(items))
	for index, item := range items {
		refID := ""
		if len(evidence) > 0 {
			refID = evidence[index%len(evidence)].SourceRefID
		}
		suggestion := ProfileUpdateSuggestion{
			ID:        fmt.Sprintf("suggestion-%d", index+1),
			Text:      item,
			Rationale: "apply only when current task matches the profile evidence",
		}
		if refID != "" {
			suggestion.EvidenceRefIDs = []string{refID}
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions
}

func profileEvidence(request Request, sourceRefs []SourceRef) []ProfileEvidence {
	items := profileListMetadata(request, ProfileMetadataEvidence)
	evidence := make([]ProfileEvidence, 0, len(items))
	for index, item := range items {
		refID := fmt.Sprintf("profile-source-%d", index+1)
		if len(sourceRefs) > 0 {
			refID = sourceRefs[index%len(sourceRefs)].ID
		}
		evidence = append(evidence, ProfileEvidence{
			ID:          fmt.Sprintf("evidence-%d", index+1),
			Summary:     item,
			SourceRefID: refID,
		})
	}
	return evidence
}

func profileContextSummary(request Request, signals []ProfileDecisionSignal, suggestions []ProfileUpdateSuggestion, evidence []ProfileEvidence, caveats []string) string {
	if summary := profileMetadata(request, ProfileMetadataContextSummary, ""); summary != "" {
		return summary
	}
	return fmt.Sprintf("Profile for %s produced %d signal(s), %d update suggestion(s), %d evidence item(s), and %d caveat(s).", profileSubject(request), len(signals), len(suggestions), len(evidence), len(caveats))
}

func profileSummary(output ProfileOutput) string {
	return fmt.Sprintf("Profile folded %d decision signal(s) into context for %s.", len(output.DecisionSignals), output.Subject)
}

func profileArtifactRefs(output ProfileOutput) []ArtifactRef {
	if strings.TrimSpace(output.Document) == "" {
		return nil
	}
	return []ArtifactRef{{ID: "profile-artifact", Kind: "profile", Path: output.ArtifactPath}}
}

func profileBoundaryRequests(request Request, durable bool) []BoundaryRequest {
	requests := []BoundaryRequest{
		request.RequestStateAccess("session.current", "profile uses app-supplied session evidence"),
		request.RequestContextAccess("current_context", "profile folds results into current context"),
	}
	if durable {
		requests = append(requests,
			request.RequestArtifactAccess("profile", "state store resolves durable profile artifact"),
			request.RequestStateWrite("profile", "state store records durable profile output"),
		)
	}
	return requests
}

func profileSourceRefs(request Request) []SourceRef {
	refs := cloneSourceRefs(request.SourceRefs)
	contextSummary := strings.TrimSpace(request.Metadata[ProfileMetadataContextSummary])
	if contextSummary != "" {
		refs = append(refs, SourceRef{ID: "profile-context", Kind: "context", Excerpt: contextSummary})
	}
	return refs
}

func profileDocument(output ProfileOutput) string {
	var b strings.Builder
	b.WriteString("# Profile\n\n")
	b.WriteString("Subject: " + output.Subject + "\n")
	if output.Context != "" {
		b.WriteString("Context: " + output.Context + "\n")
	}
	if output.Confidence != "" {
		b.WriteString("Confidence: " + output.Confidence + "\n")
	}
	b.WriteString("\n## Decision Signals\n")
	for _, signal := range output.DecisionSignals {
		b.WriteString("- " + signal.Pattern + " (" + signal.Guidance + ")\n")
	}
	b.WriteString("\n## Update Suggestions\n")
	for _, suggestion := range output.UpdateSuggestions {
		b.WriteString("- " + suggestion.Text + "\n")
	}
	b.WriteString("\n## Evidence\n")
	for _, evidence := range output.Evidence {
		b.WriteString("- " + evidence.Summary + "\n")
	}
	b.WriteString("\n## Caveats\n")
	for _, caveat := range output.Caveats {
		b.WriteString("- " + caveat + "\n")
	}
	if output.NextAction != "" {
		b.WriteString("\nNext action: " + output.NextAction + "\n")
	}
	return b.String()
}

func profileDurable(request Request) bool {
	value := strings.ToLower(profileMetadata(request, ProfileMetadataDurable, ""))
	return value == "true" || value == "yes" || value == "1"
}

func profileMetadata(request Request, key string, fallback string) string {
	if request.Metadata == nil {
		return fallback
	}
	value := strings.TrimSpace(request.Metadata[key])
	if value == "" {
		return fallback
	}
	return value
}

func profileListMetadata(request Request, key string) []string {
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
