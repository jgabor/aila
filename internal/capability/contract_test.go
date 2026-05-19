package capability

import (
	"testing"

	"github.com/jgabor/aila/internal/workflow"
)

func TestBuiltInExecutionContractsCoverRegistryWithModelBackedPath(t *testing.T) {
	t.Parallel()

	contracts := BuiltInExecutionContracts()
	byName := make(map[Name]ExecutionContract, len(contracts))
	for _, contract := range contracts {
		byName[contract.Capability] = contract
	}

	for _, definition := range BuiltInRegistry().All() {
		contract, ok := byName[definition.Name]
		if !ok {
			t.Fatalf("missing execution contract for %q", definition.Name)
		}
		if contract.Path != ExecutionPathModelBacked {
			t.Fatalf("contract path for %q = %q, want %q", definition.Name, contract.Path, ExecutionPathModelBacked)
		}
		if contract.OutputField == "" {
			t.Fatalf("contract output field for %q is empty", definition.Name)
		}
	}
}

func TestPrepareModelExecutionPreservesBoundedRequestContext(t *testing.T) {
	t.Parallel()

	request := Request{
		ID:         "build-001",
		Capability: NameBuild,
		Input:      "Implement the selected task.",
		Phase:      workflow.PhaseBuild,
		SourceRefs: []SourceRef{{ID: "plan-1", Kind: "plan", Path: ".agentera/plan.yaml", LineStart: 10, LineEnd: 14, Excerpt: "Task 1"}},
		Metadata:   map[string]string{"plan_task": "Task 1", "acceptance": "model path typed"},
	}

	prepared := PrepareModelExecution(request)
	if prepared.Path != ExecutionPathModelBacked || prepared.ModelCall == nil || prepared.Waiting != nil || prepared.Stuck != nil {
		t.Fatalf("prepared execution = %+v", prepared)
	}
	if prepared.ModelCall.Kind != BoundaryModelCall || prepared.ModelCall.RequestID != request.ID || prepared.ModelCall.Capability != request.Capability {
		t.Fatalf("model call boundary = %+v", prepared.ModelCall)
	}
	if prepared.Context.RequestID != request.ID || prepared.Context.Capability != request.Capability || prepared.Context.Input != request.Input || prepared.Context.Phase != request.Phase {
		t.Fatalf("context scalar fields = %+v", prepared.Context)
	}
	if len(prepared.Context.SourceRefs) != 1 || prepared.Context.SourceRefs[0].ID != "plan-1" {
		t.Fatalf("source refs = %+v", prepared.Context.SourceRefs)
	}
	if prepared.Context.Metadata["plan_task"] != "Task 1" || prepared.Context.Metadata["acceptance"] != "model path typed" {
		t.Fatalf("metadata = %+v", prepared.Context.Metadata)
	}

	request.SourceRefs[0].ID = "mutated"
	request.Metadata["plan_task"] = "mutated"
	if prepared.Context.SourceRefs[0].ID != "plan-1" || prepared.Context.Metadata["plan_task"] != "Task 1" {
		t.Fatalf("prepared context aliases request: %+v", prepared.Context)
	}
}

func TestPrepareModelExecutionReturnsTypedWaitingReasonForMissingRequiredContext(t *testing.T) {
	t.Parallel()

	prepared := PrepareModelExecution(Request{ID: "plan-001", Capability: NamePlan, Phase: workflow.PhasePlan})
	if prepared.Path != ExecutionPathWaiting || prepared.MissingContextReason != MissingContextInput || prepared.Waiting == nil {
		t.Fatalf("prepared execution = %+v", prepared)
	}
	if prepared.Waiting.Signal != ExitWaiting || prepared.Waiting.NeededInput != string(MissingContextInput) || prepared.Waiting.RecommendedSuccessor != "" {
		t.Fatalf("waiting payload = %+v", prepared.Waiting)
	}
}

func TestPrepareModelExecutionReturnsTypedStuckReasonForUnknownCapability(t *testing.T) {
	t.Parallel()

	prepared := PrepareModelExecution(Request{ID: "unknown-001", Capability: Name("unknown"), Phase: workflow.PhaseBuild, Input: "run"})
	if prepared.Path != ExecutionPathStuck || prepared.MissingContextReason != MissingContextUnknownCapability || prepared.Stuck == nil {
		t.Fatalf("prepared execution = %+v", prepared)
	}
	if prepared.Stuck.Signal != ExitStuck || prepared.Stuck.Blocker == "" || prepared.Stuck.Attempted {
		t.Fatalf("stuck payload = %+v", prepared.Stuck)
	}
}
