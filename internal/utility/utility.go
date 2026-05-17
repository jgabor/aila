// Package utility schedules idle-only, read-only utility jobs.
package utility

import "strings"

// JobKind names a fixed utility worker job family.
type JobKind string

const (
	JobContextPrep JobKind = "context_prep"
	JobSuggestion  JobKind = "suggestion"
)

// Status describes utility worker state independently from the primary runtime.
type Status string

const (
	StatusIdle      Status = "idle"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusBlocked   Status = "blocked"
)

// Source records caller-visible provenance for fake utility jobs.
type Source struct {
	Caller      string
	RequestID   string
	Description string
}

// JobRequest records a deterministic fake utility job request.
type JobRequest struct {
	ID     string
	Kind   JobKind
	Model  string
	Source Source
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

// EvidenceRef records exact evidence backing a fake utility suggestion.
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

// SafetyBoundary proves utility output did not perform consequential actions.
type SafetyBoundary struct {
	FileMutation            bool
	GitMutation             bool
	ProjectArtifactMutation bool
	PermissionApproval      bool
	WorkflowPhaseTransition bool
	FinalJudgment           bool
}

// JobResult is an immutable fake utility result for display and tests.
type JobResult struct {
	Request      JobRequest
	Status       Status
	Summary      string
	Suggestions  []Suggestion
	EvidenceRefs []EvidenceRef
	Caveats      []string
	Denial       Decision
	Safety       SafetyBoundary
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
	return request
}

// CanRun returns whether a fake utility job may run in the current activity.
func CanRun(activity Activity, request JobRequest) Decision {
	request = NormalizeJobRequest(request)
	if request.Kind != JobContextPrep && request.Kind != JobSuggestion {
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
	return JobResult{Request: request, Status: StatusRunning, Summary: "fake utility job running", Safety: SafetyBoundary{}}
}

// BlockedResult records why a fake utility job did not start.
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

// RunFakeJob returns deterministic read-only utility output.
func RunFakeJob(request JobRequest) JobResult {
	request = NormalizeJobRequest(request)
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
