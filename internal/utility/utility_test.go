package utility

import "testing"

func TestSchedulerAllowsUtilityJobsOnlyWhenPrimaryRuntimeIdle(t *testing.T) {
	t.Parallel()

	request := JobRequest{ID: "status-stale-context-check", Kind: JobStaleContextCheck}
	decision := CanRun(Activity{PrimaryStatus: "idle"}, request)
	if !decision.Allowed || decision.Reason != DenialNone {
		t.Fatalf("idle decision = %+v, want allowed", decision)
	}

	cases := []struct {
		name     string
		activity Activity
		want     DenialReason
	}{
		{name: "primary busy", activity: Activity{PrimaryStatus: "active"}, want: DenialPrimaryBusy},
		{name: "active operation", activity: Activity{PrimaryStatus: "idle", ActiveOperationKind: "bash"}, want: DenialActiveOperation},
		{name: "approval pending", activity: Activity{PrimaryStatus: "idle", ApprovalPending: true}, want: DenialApprovalPending},
		{name: "queued input", activity: Activity{PrimaryStatus: "idle", QueuedCount: 1}, want: DenialQueuedUserInput},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := CanRun(tc.activity, request)
			if got.Allowed || got.Reason != tc.want || got.Detail == "" {
				t.Fatalf("decision = %+v, want denied %s with detail", got, tc.want)
			}
		})
	}
}

func TestSuggestionUtilityResultIsReadOnlyEvidenceOnly(t *testing.T) {
	t.Parallel()

	result := RunJob(JobRequest{ID: "status-utility", Kind: JobSuggestion, Source: Source{Caller: "app.status"}})
	if result.Status != StatusCompleted || result.Summary == "" || len(result.Suggestions) != 1 || len(result.EvidenceRefs) != 1 {
		t.Fatalf("result = %+v, want completed suggestion with evidence", result)
	}
	assertReadOnlySafety(t, result.Safety)
}

func TestContextPrepResultIsReadOnlyNonAuthoritativeEvidence(t *testing.T) {
	t.Parallel()

	result := RunJob(JobRequest{ID: "status-context-prep", Kind: JobContextPrep, Source: Source{Caller: "app.status"}})
	if result.Status != StatusCompleted || result.Summary != "prepared context ready" {
		t.Fatalf("context prep status = %+v", result)
	}
	if result.PreparedContext.Summary == "" || !result.PreparedContext.NonAuthoritative || len(result.PreparedContext.EvidenceRefIDs) != 2 || len(result.PreparedContext.Caveats) != 1 {
		t.Fatalf("prepared context = %+v, want summary/evidence/caveat/non-authoritative", result.PreparedContext)
	}
	if len(result.Suggestions) != 1 || len(result.EvidenceRefs) != 2 || len(result.Caveats) != 1 {
		t.Fatalf("context prep output = suggestions:%d evidence:%d caveats:%d", len(result.Suggestions), len(result.EvidenceRefs), len(result.Caveats))
	}
	assertReadOnlySafety(t, result.Safety)
}

func TestStaleContextResultReportsFreshStaleAndUnknownWithoutMutation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   StaleContextInput
		want    StaleContextStatus
		caveats bool
	}{
		{name: "fresh", input: StaleContextInput{SavedFingerprint: "ctx-1", CurrentFingerprint: "ctx-1"}, want: StaleContextFresh},
		{name: "stale", input: StaleContextInput{SavedFingerprint: "ctx-1", CurrentFingerprint: "ctx-2"}, want: StaleContextStale, caveats: true},
		{name: "unknown", input: StaleContextInput{SavedFingerprint: "ctx-1"}, want: StaleContextUnknown, caveats: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := RunJob(JobRequest{ID: "status-stale-context-check", Kind: JobStaleContextCheck, Source: Source{Caller: "app.status"}, StaleContext: tc.input})
			if result.Status != StatusCompleted || result.StaleContext.Status != tc.want || result.StaleContext.Summary == "" || result.StaleContext.SuggestedNextAction == "" || len(result.StaleContext.EvidenceRefIDs) == 0 || len(result.EvidenceRefs) == 0 {
				t.Fatalf("stale context result = %+v, want %s with summary/evidence/action", result, tc.want)
			}
			if (len(result.StaleContext.Caveats) > 0) != tc.caveats {
				t.Fatalf("caveats = %#v, want caveats=%v", result.StaleContext.Caveats, tc.caveats)
			}
			assertReadOnlySafety(t, result.Safety)
		})
	}
}

func TestBlockedUtilityResultPreservesDenialWithoutContextOutput(t *testing.T) {
	t.Parallel()

	decision := CanRun(Activity{PrimaryStatus: "active"}, JobRequest{Kind: JobStaleContextCheck})
	result := BlockedResult(JobRequest{Kind: JobStaleContextCheck}, decision)
	if result.Status != StatusBlocked || result.Denial.Reason != DenialPrimaryBusy || len(result.Suggestions) != 0 || len(result.EvidenceRefs) != 0 || result.PreparedContext.Summary != "" || result.StaleContext.Status != "" {
		t.Fatalf("blocked result = %+v", result)
	}
}

func assertReadOnlySafety(t *testing.T, safety SafetyBoundary) {
	t.Helper()

	if safety.FileMutation || safety.GitMutation || safety.ProjectArtifactMutation || safety.PermissionApproval || safety.WorkflowPhaseTransition || safety.FinalJudgment || safety.ContextRefresh || safety.ContextCompaction || safety.ContextRewrite {
		t.Fatalf("utility result crossed safety boundary: %+v", safety)
	}
}
