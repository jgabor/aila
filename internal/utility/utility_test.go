package utility

import "testing"

func TestSchedulerAllowsUtilityJobsOnlyWhenPrimaryRuntimeIdle(t *testing.T) {
	t.Parallel()

	request := JobRequest{ID: "status-context-prep", Kind: JobContextPrep}
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

func TestBlockedUtilityResultPreservesDenialWithoutPreparedContext(t *testing.T) {
	t.Parallel()

	decision := CanRun(Activity{PrimaryStatus: "active"}, JobRequest{Kind: JobContextPrep})
	result := BlockedResult(JobRequest{Kind: JobContextPrep}, decision)
	if result.Status != StatusBlocked || result.Denial.Reason != DenialPrimaryBusy || len(result.Suggestions) != 0 || len(result.EvidenceRefs) != 0 || result.PreparedContext.Summary != "" {
		t.Fatalf("blocked result = %+v", result)
	}
}

func assertReadOnlySafety(t *testing.T, safety SafetyBoundary) {
	t.Helper()

	if safety.FileMutation || safety.GitMutation || safety.ProjectArtifactMutation || safety.PermissionApproval || safety.WorkflowPhaseTransition || safety.FinalJudgment {
		t.Fatalf("utility result crossed safety boundary: %+v", safety)
	}
}
