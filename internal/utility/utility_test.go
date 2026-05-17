package utility

import "testing"

func TestSchedulerAllowsFakeJobsOnlyWhenPrimaryRuntimeIdle(t *testing.T) {
	t.Parallel()

	request := JobRequest{ID: "status-utility", Kind: JobSuggestion}
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

func TestFakeUtilityResultIsReadOnlyEvidenceOnly(t *testing.T) {
	t.Parallel()

	result := RunFakeJob(JobRequest{ID: "status-utility", Kind: JobSuggestion, Source: Source{Caller: "app.status"}})
	if result.Status != StatusCompleted || result.Summary == "" || len(result.Suggestions) != 1 || len(result.EvidenceRefs) != 1 {
		t.Fatalf("result = %+v, want completed suggestion with evidence", result)
	}
	if result.Safety.FileMutation || result.Safety.GitMutation || result.Safety.ProjectArtifactMutation || result.Safety.PermissionApproval || result.Safety.WorkflowPhaseTransition || result.Safety.FinalJudgment {
		t.Fatalf("utility result crossed safety boundary: %+v", result.Safety)
	}
}

func TestBlockedUtilityResultPreservesDenialWithoutOutput(t *testing.T) {
	t.Parallel()

	decision := CanRun(Activity{PrimaryStatus: "active"}, JobRequest{Kind: JobSuggestion})
	result := BlockedResult(JobRequest{Kind: JobSuggestion}, decision)
	if result.Status != StatusBlocked || result.Denial.Reason != DenialPrimaryBusy || len(result.Suggestions) != 0 || len(result.EvidenceRefs) != 0 {
		t.Fatalf("blocked result = %+v", result)
	}
}
