package permission

import "testing"

func TestReadOperationClassifiesAsSafeReadOnly(t *testing.T) {
	t.Parallel()

	operation := NewReadOperation("notes.txt")

	if operation.Kind != OperationRead || operation.Tool != "read" || operation.TargetPath != "notes.txt" {
		t.Fatalf("read operation = %#v, want read tool target", operation)
	}
	if operation.ExpectedEffect != "bounded workspace file preview" || !operation.Reversible {
		t.Fatalf("read operation safety metadata = %#v", operation)
	}
	if len(operation.Command) != 0 || operation.DiffPreview != "" || operation.WorkingDir != "" || operation.TargetVersion != "" {
		t.Fatalf("read operation reused mutation, shell, or version fields: %#v", operation)
	}
}

func TestDecideAllowsReadOnlyAutomaticallyAtReadOrHigher(t *testing.T) {
	t.Parallel()

	operation := NewReadOperation("notes.txt")
	for _, level := range []AutonomyLevel{AutonomyRead, AutonomyWrite, AutonomyYolo} {
		decision := Decide(level, operation)
		if !decision.Allowed || !decision.Automatic || decision.Reason == "" {
			t.Fatalf("Decide(%q, read) = %#v, want automatic allow", level, decision)
		}
	}
}

func TestDecideDoesNotAutoApproveReadsWhenAutonomyOff(t *testing.T) {
	t.Parallel()

	decision := Decide(AutonomyOff, NewReadOperation("notes.txt"))

	if decision.Allowed || decision.Automatic || decision.Reason == "" {
		t.Fatalf("Decide(off, read) = %#v, want denied pending approval", decision)
	}
}

func TestDecideDoesNotClassifyMutationOrExecAsRead(t *testing.T) {
	t.Parallel()

	for _, operation := range []ProposedOperation{
		{Kind: OperationMutation, Tool: "edit", TargetPath: "notes.txt", DiffPreview: "-old\n+new"},
		{Kind: OperationExec, Tool: "bash", Command: []string{"git", "status"}},
	} {
		decision := Decide(AutonomyRead, operation)
		if decision.Allowed || decision.Automatic {
			t.Fatalf("Decide(read, %#v) = %#v, want denied non-read operation", operation, decision)
		}
	}
}
