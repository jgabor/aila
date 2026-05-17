// Package utility schedules idle-only, read-only utility jobs.
package utility

import (
	"fmt"
	"strings"
)

// JobKind names a fixed utility worker job family.
type JobKind string

const (
	JobContextPrep       JobKind = "context_prep"
	JobStaleContextCheck JobKind = "stale_context_check"
	JobSummaryRefresh    JobKind = "summary_refresh"
	JobSuggestion        JobKind = "suggestion"
)

// Status describes utility worker state independently from the primary runtime.
type Status string

const (
	StatusIdle      Status = "idle"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusBlocked   Status = "blocked"
)

// Source records caller-visible provenance for utility jobs.
type Source struct {
	Caller      string
	RequestID   string
	Description string
}

// StaleContextInput records bounded context freshness facts for pure checks.
type StaleContextInput struct {
	SavedFingerprint   string
	CurrentFingerprint string
	SavedLabel         string
	CurrentLabel       string
}

// SummaryRefreshInput records bounded summary facts for pure refresh checks.
type SummaryRefreshInput struct {
	OriginalSummary string
	RequiredDetails []string
	SourceRefIDs    []string
	ConfidenceHint  string
}

// JobRequest records a deterministic utility job request.
type JobRequest struct {
	ID             string
	Kind           JobKind
	Model          string
	Source         Source
	StaleContext   StaleContextInput
	SummaryRefresh SummaryRefreshInput
}

// Activity records the primary runtime facts needed for idle-only scheduling.
type Activity struct {
	PrimaryStatus       string
	ActiveOperationKind string
	ApprovalPending     bool
	QueuedCount         int
}

// DenialReason names why a utility job did not start.
type DenialReason string

const (
	DenialNone               DenialReason = "none"
	DenialPrimaryBusy        DenialReason = "primary_runtime_busy"
	DenialActiveOperation    DenialReason = "active_operation"
	DenialApprovalPending    DenialReason = "approval_pending"
	DenialQueuedUserInput    DenialReason = "queued_user_input"
	DenialUnsupportedJobKind DenialReason = "unsupported_job_kind"
)

// Decision records a pure scheduling decision.
type Decision struct {
	Allowed bool
	Reason  DenialReason
	Detail  string
}

// EvidenceRef records exact evidence backing utility output.
type EvidenceRef struct {
	ID     string
	Kind   string
	Source string
	Detail string
}

// Suggestion is display-only utility output.
type Suggestion struct {
	Text           string
	EvidenceRefIDs []string
}

// PreparedContext is display-only context prep output. It is non-authoritative;
// foreground work decides whether to use it.
type PreparedContext struct {
	Summary          string
	EvidenceRefIDs   []string
	Caveats          []string
	NonAuthoritative bool
}

// StaleContextStatus describes whether saved context should be trusted.
type StaleContextStatus string

const (
	StaleContextFresh   StaleContextStatus = "fresh"
	StaleContextStale   StaleContextStatus = "stale"
	StaleContextUnknown StaleContextStatus = "unknown"
)

// StaleContextCheck is display-only freshness output. It never refreshes,
// rewrites, or compacts context.
type StaleContextCheck struct {
	Status              StaleContextStatus
	Summary             string
	EvidenceRefIDs      []string
	Caveats             []string
	SuggestedNextAction string
}

// SummaryRefreshStatus names the display-only summary refresh outcome.
type SummaryRefreshStatus string

const (
	SummaryRefreshRefreshed     SummaryRefreshStatus = "refreshed"
	SummaryRefreshCurrent       SummaryRefreshStatus = "current"
	SummaryRefreshLowConfidence SummaryRefreshStatus = "low_confidence"
)

// SummaryRefresh is display-only refreshed summary output. Foreground work
// decides whether to use it.
type SummaryRefresh struct {
	Status           SummaryRefreshStatus
	OriginalSummary  string
	RefreshedSummary string
	SourceRefIDs     []string
	ExactDetails     []string
	Confidence       string
	Caveats          []string
}

// SafetyBoundary proves utility output did not perform consequential actions.
type SafetyBoundary struct {
	FileMutation            bool
	GitMutation             bool
	ProjectArtifactMutation bool
	PermissionApproval      bool
	WorkflowPhaseTransition bool
	FinalJudgment           bool
	ContextRefresh          bool
	ContextCompaction       bool
	ContextRewrite          bool
}

// JobResult is an immutable utility result for display and tests.
type JobResult struct {
	Request         JobRequest
	Status          Status
	Summary         string
	PreparedContext PreparedContext
	StaleContext    StaleContextCheck
	SummaryRefresh  SummaryRefresh
	Suggestions     []Suggestion
	EvidenceRefs    []EvidenceRef
	Caveats         []string
	Denial          Decision
	Safety          SafetyBoundary
}

// NormalizeJobRequest fills stable defaults without performing IO.
func NormalizeJobRequest(request JobRequest) JobRequest {
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		request.ID = "utility-fake-job"
	}
	if request.Kind == "" {
		request.Kind = JobSuggestion
	}
	request.Model = strings.TrimSpace(request.Model)
	if request.Model == "" {
		request.Model = "utility"
	}
	request.Source.Caller = strings.TrimSpace(request.Source.Caller)
	if request.Source.Caller == "" {
		request.Source.Caller = "app.utility"
	}
	request.Source.RequestID = strings.TrimSpace(request.Source.RequestID)
	if request.Source.RequestID == "" {
		request.Source.RequestID = request.ID
	}
	request.Source.Description = strings.TrimSpace(request.Source.Description)
	request.StaleContext.SavedFingerprint = strings.TrimSpace(request.StaleContext.SavedFingerprint)
	request.StaleContext.CurrentFingerprint = strings.TrimSpace(request.StaleContext.CurrentFingerprint)
	request.StaleContext.SavedLabel = strings.TrimSpace(request.StaleContext.SavedLabel)
	request.StaleContext.CurrentLabel = strings.TrimSpace(request.StaleContext.CurrentLabel)
	if request.StaleContext.SavedLabel == "" {
		request.StaleContext.SavedLabel = "saved context"
	}
	if request.StaleContext.CurrentLabel == "" {
		request.StaleContext.CurrentLabel = "current context"
	}
	request.SummaryRefresh.OriginalSummary = strings.TrimSpace(request.SummaryRefresh.OriginalSummary)
	request.SummaryRefresh.RequiredDetails = cleanStringSlice(request.SummaryRefresh.RequiredDetails)
	request.SummaryRefresh.SourceRefIDs = cleanStringSlice(request.SummaryRefresh.SourceRefIDs)
	request.SummaryRefresh.ConfidenceHint = normalizeSummaryRefreshConfidence(request.SummaryRefresh.ConfidenceHint)
	return request
}

// CanRun returns whether a utility job may run in the current activity.
func CanRun(activity Activity, request JobRequest) Decision {
	request = NormalizeJobRequest(request)
	if request.Kind != JobContextPrep && request.Kind != JobStaleContextCheck && request.Kind != JobSummaryRefresh && request.Kind != JobSuggestion {
		return Decision{Reason: DenialUnsupportedJobKind, Detail: "unsupported utility job kind"}
	}
	status := strings.TrimSpace(activity.PrimaryStatus)
	if status != "" && status != string(StatusIdle) {
		return Decision{Reason: DenialPrimaryBusy, Detail: "primary runtime is " + status}
	}
	if strings.TrimSpace(activity.ActiveOperationKind) != "" {
		return Decision{Reason: DenialActiveOperation, Detail: "active operation is " + activity.ActiveOperationKind}
	}
	if activity.ApprovalPending {
		return Decision{Reason: DenialApprovalPending, Detail: "approval is pending"}
	}
	if activity.QueuedCount > 0 {
		return Decision{Reason: DenialQueuedUserInput, Detail: "user input is queued"}
	}
	return Decision{Allowed: true, Reason: DenialNone, Detail: "primary runtime is idle"}
}

// RunningResult records visible utility running state.
func RunningResult(request JobRequest) JobResult {
	request = NormalizeJobRequest(request)
	summary := "utility job running"
	if request.Kind == JobContextPrep {
		summary = "utility context prep running"
	}
	if request.Kind == JobStaleContextCheck {
		summary = "utility stale-context check running"
	}
	if request.Kind == JobSummaryRefresh {
		summary = "utility summary refresh running"
	}
	return JobResult{Request: request, Status: StatusRunning, Summary: summary, Safety: SafetyBoundary{}}
}

// BlockedResult records why a utility job did not start.
func BlockedResult(request JobRequest, decision Decision) JobResult {
	request = NormalizeJobRequest(request)
	if decision.Reason == "" {
		decision.Reason = DenialPrimaryBusy
	}
	return JobResult{
		Request: request,
		Status:  StatusBlocked,
		Summary: "utility job blocked: " + decision.Detail,
		Caveats: []string{decision.Detail},
		Denial:  decision,
		Safety:  SafetyBoundary{},
	}
}

// RunJob returns deterministic read-only utility output.
func RunJob(request JobRequest) JobResult {
	request = NormalizeJobRequest(request)
	if request.Kind == JobContextPrep {
		return contextPrepResult(request)
	}
	if request.Kind == JobStaleContextCheck {
		return staleContextResult(request)
	}
	if request.Kind == JobSummaryRefresh {
		return summaryRefreshResult(request)
	}
	return JobResult{
		Request: request,
		Status:  StatusCompleted,
		Summary: "fake utility suggestion ready",
		Suggestions: []Suggestion{{
			Text:           "Review current status before starting new background utility work.",
			EvidenceRefIDs: []string{"utility-evidence-1"},
		}},
		EvidenceRefs: []EvidenceRef{{
			ID:     "utility-evidence-1",
			Kind:   "runtime_state",
			Source: request.Source.Caller,
			Detail: "primary runtime idle; fake utility job only",
		}},
		Safety: SafetyBoundary{},
	}
}

func summaryRefreshResult(request JobRequest) JobResult {
	input := request.SummaryRefresh
	original := strings.TrimSpace(input.OriginalSummary)
	details := cleanStringSlice(input.RequiredDetails)
	sourceRefIDs := cleanStringSlice(input.SourceRefIDs)
	confidence := normalizeSummaryRefreshConfidence(input.ConfidenceHint)
	if confidence == "" {
		confidence = "medium"
	}

	missing := missingSummaryDetails(original, details)
	refreshed := original
	status := SummaryRefreshCurrent
	summary := "summary already includes required details"
	if len(missing) > 0 {
		status = SummaryRefreshRefreshed
		summary = "summary refreshed with source-backed details"
		refreshed = appendSummaryDetails(original, missing)
	}

	var caveats []string
	if confidence == "low" || len(details) == 0 || len(sourceRefIDs) == 0 {
		status = SummaryRefreshLowConfidence
		summary = "summary refresh confidence low"
		if len(details) == 0 {
			caveats = append(caveats, "exact details missing; refreshed summary is advisory")
		}
		if len(sourceRefIDs) == 0 {
			caveats = append(caveats, "source refs missing; refreshed summary must not replace source checks")
		}
		if confidence == "low" {
			caveats = append(caveats, "refresh confidence is low; foreground work must check source refs before using refreshed summary")
		}
	}

	refresh := SummaryRefresh{
		Status:           status,
		OriginalSummary:  original,
		RefreshedSummary: refreshed,
		SourceRefIDs:     sourceRefIDs,
		ExactDetails:     details,
		Confidence:       confidence,
		Caveats:          append([]string(nil), caveats...),
	}
	evidence := summaryRefreshEvidence(request.Source.Caller, sourceRefIDs, details)
	suggestion := summaryRefreshSuggestion(refresh, evidence)
	return JobResult{
		Request:        request,
		Status:         StatusCompleted,
		Summary:        summary,
		SummaryRefresh: refresh,
		Suggestions:    []Suggestion{suggestion},
		EvidenceRefs:   evidence,
		Caveats:        append([]string(nil), caveats...),
		Safety:         SafetyBoundary{},
	}
}

func summaryRefreshSuggestion(refresh SummaryRefresh, evidence []EvidenceRef) Suggestion {
	text := "Keep the current summary; supplied details are already represented."
	if refresh.Status == SummaryRefreshRefreshed {
		text = "Use the refreshed summary only with its preserved source refs."
	}
	if refresh.Status == SummaryRefreshLowConfidence {
		text = "Review preserved source refs before using the refreshed summary."
	}
	return Suggestion{Text: text, EvidenceRefIDs: evidenceIDs(evidence)}
}

func summaryRefreshEvidence(source string, sourceRefIDs []string, details []string) []EvidenceRef {
	var evidence []EvidenceRef
	for index, refID := range sourceRefIDs {
		evidence = append(evidence, EvidenceRef{ID: numberedID("summary-refresh-source", index+1), Kind: "source_ref", Source: source, Detail: "source_ref=" + refID})
	}
	for index, detail := range details {
		evidence = append(evidence, EvidenceRef{ID: numberedID("summary-refresh-detail", index+1), Kind: "exact_detail", Source: source, Detail: detail})
	}
	return evidence
}

func missingSummaryDetails(summary string, details []string) []string {
	lowerSummary := strings.ToLower(summary)
	var missing []string
	for _, detail := range details {
		if !strings.Contains(lowerSummary, strings.ToLower(detail)) {
			missing = append(missing, detail)
		}
	}
	return missing
}

func appendSummaryDetails(summary string, details []string) string {
	joined := "Important details: " + strings.Join(details, "; ")
	if strings.TrimSpace(summary) == "" {
		return joined
	}
	return strings.TrimSpace(summary) + " " + joined
}

func normalizeSummaryRefreshConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func cleanStringSlice(values []string) []string {
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func evidenceIDs(evidence []EvidenceRef) []string {
	ids := make([]string, 0, len(evidence))
	for _, ref := range evidence {
		ids = append(ids, ref.ID)
	}
	return ids
}

func numberedID(prefix string, number int) string {
	return fmt.Sprintf("%s-%d", prefix, number)
}

func contextPrepResult(request JobRequest) JobResult {
	return JobResult{
		Request: request,
		Status:  StatusCompleted,
		Summary: "prepared context ready",
		PreparedContext: PreparedContext{
			Summary:          "Likely next context: roadmap M42 scope, current utility worker state, and recent status evidence.",
			EvidenceRefIDs:   []string{"context-prep-roadmap", "context-prep-runtime"},
			Caveats:          []string{"prepared context is non-authoritative; foreground work must re-check source refs before acting"},
			NonAuthoritative: true,
		},
		Suggestions: []Suggestion{{
			Text:           "Use prepared context only as a starting point for the next foreground step.",
			EvidenceRefIDs: []string{"context-prep-roadmap", "context-prep-runtime"},
		}},
		EvidenceRefs: []EvidenceRef{
			{
				ID:     "context-prep-roadmap",
				Kind:   "roadmap",
				Source: "ROADMAP.md",
				Detail: "Milestone 42 requires visible non-authoritative utility context prep",
			},
			{
				ID:     "context-prep-runtime",
				Kind:   "runtime_state",
				Source: request.Source.Caller,
				Detail: "primary runtime idle; context prep allowed by utility scheduler",
			},
		},
		Caveats: []string{"prepared context is non-authoritative; foreground capability decides whether to use it"},
		Safety:  SafetyBoundary{},
	}
}

func staleContextResult(request JobRequest) JobResult {
	input := request.StaleContext
	check := StaleContextCheck{
		Status:              StaleContextUnknown,
		Summary:             "saved context freshness unknown",
		EvidenceRefIDs:      []string{"stale-context-input"},
		Caveats:             []string{"saved or current context fingerprint missing; no context was refreshed, compacted, or rewritten"},
		SuggestedNextAction: "Rebuild foreground context before relying on saved context.",
	}
	evidence := []EvidenceRef{{
		ID:     "stale-context-input",
		Kind:   "context_fingerprint",
		Source: request.Source.Caller,
		Detail: "saved or current context fingerprint missing",
	}}
	if input.SavedFingerprint != "" && input.CurrentFingerprint != "" {
		check.Status = StaleContextFresh
		check.Summary = "saved context appears fresh"
		check.EvidenceRefIDs = []string{"stale-context-saved", "stale-context-current"}
		check.Caveats = nil
		check.SuggestedNextAction = "Continue with normal source-ref checks."
		if input.SavedFingerprint != input.CurrentFingerprint {
			check.Status = StaleContextStale
			check.Summary = "saved context appears stale"
			check.Caveats = []string{"stale status is advisory; no context was refreshed, compacted, or rewritten"}
			check.SuggestedNextAction = "Rebuild foreground context before relying on saved context."
		}
		evidence = []EvidenceRef{
			{
				ID:     "stale-context-saved",
				Kind:   "context_fingerprint",
				Source: input.SavedLabel,
				Detail: "saved=" + input.SavedFingerprint,
			},
			{
				ID:     "stale-context-current",
				Kind:   "context_fingerprint",
				Source: input.CurrentLabel,
				Detail: "current=" + input.CurrentFingerprint,
			},
		}
	}
	return JobResult{
		Request:      request,
		Status:       StatusCompleted,
		Summary:      check.Summary,
		StaleContext: check,
		Suggestions: []Suggestion{{
			Text:           check.SuggestedNextAction,
			EvidenceRefIDs: append([]string(nil), check.EvidenceRefIDs...),
		}},
		EvidenceRefs: evidence,
		Caveats:      append([]string(nil), check.Caveats...),
		Safety:       SafetyBoundary{},
	}
}
