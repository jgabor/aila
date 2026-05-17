package capability

import (
	"context"
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestOrchestrateCapabilityRecordsCyclesRetriesDecisionsAndEvidence(t *testing.T) {
	t.Parallel()

	payload, err := OrchestrateCapability{}.Run(context.Background(), Request{
		ID:         "orchestrate-1",
		Capability: NameOrchestrate,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			OrchestrateMetadataGoalID:      "m56-plan",
			OrchestrateMetadataGoalTitle:   "Coordinate M56 implementation",
			OrchestrateMetadataGoalScope:   "bounded orchestration over existing capabilities",
			OrchestrateMetadataRetryBudget: "1",
		},
		SourceRefs: []SourceRef{{ID: "orchestrate-command", Kind: "command", Command: "/orchestrate", Excerpt: "app-owned orchestrate command"}},
	})
	if err != nil {
		t.Fatalf("Run orchestrate capability: %v", err)
	}
	if payload.Capability != NameOrchestrate || payload.Signal != ExitComplete || payload.Orchestrate == nil {
		t.Fatalf("orchestrate payload identity = %+v", payload)
	}
	output := payload.Orchestrate
	if output.Goal.ID != "m56-plan" || output.Status != "completed" || output.ActiveCycle != "cycle-2" {
		t.Fatalf("orchestrate goal/status = %+v", output)
	}
	if len(output.Cycles) != 2 || output.Cycles[0].Status != "retrying" || output.Cycles[0].RetryDecision == "" || output.Cycles[1].Status != "completed" {
		t.Fatalf("cycles = %+v", output.Cycles)
	}
	if len(output.ChildWork) != 3 || output.ChildWork[0].Status != "failed" || output.ChildWork[1].RetryAttempt != 1 {
		t.Fatalf("child work = %+v", output.ChildWork)
	}
	if output.RetryBudget.MaxAttempts != 1 || output.RetryBudget.Used != 1 || output.RetryBudget.Remaining != 0 {
		t.Fatalf("retry budget = %+v", output.RetryBudget)
	}
	if len(output.Decisions) != 2 || output.Decisions[0].Kind != "retry" || len(output.Evidence) != 2 {
		t.Fatalf("decisions/evidence = %+v / %+v", output.Decisions, output.Evidence)
	}
	if payload.RecommendedSuccessor != workflow.PhaseAudit || payload.NextAction == "" {
		t.Fatalf("successor/next = %q %q", payload.RecommendedSuccessor, payload.NextAction)
	}
	if !hasBoundaryRequest(payload.BoundaryRequests, BoundaryToolExecution, "fixed built-in capabilities") || !hasBoundaryRequest(payload.BoundaryRequests, BoundaryPermissionCheck, "supervised subagents") {
		t.Fatalf("boundary requests = %+v", payload.BoundaryRequests)
	}
}

func TestOrchestrateCapabilityFlagsBlockersWithoutLosingRetryEvidence(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{
		Capability: NameOrchestrate,
		Phase:      workflow.PhaseBuild,
		Metadata: map[string]string{
			OrchestrateMetadataBlockers: "audit evidence missing",
		},
	})
	if err != nil {
		t.Fatalf("RunBuiltIn orchestrate: %v", err)
	}
	if payload.Signal != ExitFlagged || payload.RecommendedSuccessor != workflow.PhaseBuild || payload.Orchestrate == nil {
		t.Fatalf("flagged orchestrate payload = %+v", payload)
	}
	if len(payload.Orchestrate.Cycles) != 2 || payload.Orchestrate.Cycles[0].RetryDecision == "" || len(payload.Orchestrate.ChildWork) != 3 {
		t.Fatalf("retry evidence lost: %+v", payload.Orchestrate)
	}
}

func TestRunBuiltInDispatchesOrchestrateCapability(t *testing.T) {
	t.Parallel()

	payload, err := RunBuiltIn(context.Background(), Request{Capability: NameOrchestrate, Phase: workflow.PhaseBuild})
	if err != nil {
		t.Fatalf("RunBuiltIn orchestrate: %v", err)
	}
	if payload.Capability != NameOrchestrate || payload.Orchestrate == nil {
		t.Fatalf("RunBuiltIn payload = %+v", payload)
	}
}
