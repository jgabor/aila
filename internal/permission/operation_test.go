package permission

import "testing"

func TestReadOperationClassifiesAsSafeReadOnly(t *testing.T) {
	t.Parallel()

	for _, operation := range []ProposedOperation{
		NewReadOperation("notes.txt"),
		NewFindOperation("**/*.go"),
		NewGrepOperation("TODO", "**/*.go"),
	} {
		operation := operation
		t.Run(operation.Tool, func(t *testing.T) {
			t.Parallel()

			if operation.Kind != OperationRead || operation.Tool == "" || operation.TargetPath == "" {
				t.Fatalf("read-only operation = %#v, want tool target", operation)
			}
			if operation.ExpectedEffect == "" || !operation.Reversible {
				t.Fatalf("operation safety metadata = %#v", operation)
			}
			if len(operation.Command) != 0 || operation.DiffPreview != "" || operation.WorkingDir != "" || operation.TargetVersion != "" {
				t.Fatalf("operation reused mutation, shell, or version fields: %#v", operation)
			}
		})
	}
}

func TestDecideAllowsReadOnlyAutomaticallyAtReadOrHigher(t *testing.T) {
	t.Parallel()

	operations := []ProposedOperation{NewReadOperation("notes.txt"), NewFindOperation("**/*.go"), NewGrepOperation("TODO", "**/*.go")}
	for _, level := range []AutonomyLevel{AutonomyRead, AutonomyWrite, AutonomyYolo} {
		for _, operation := range operations {
			decision := Decide(level, operation)
			if !decision.Allowed || !decision.Automatic || decision.Reason == "" {
				t.Fatalf("Decide(%q, %#v) = %#v, want automatic allow", level, operation, decision)
			}
		}
	}
}

func TestDecideDoesNotAutoApproveReadsWhenAutonomyOff(t *testing.T) {
	t.Parallel()

	for _, operation := range []ProposedOperation{NewReadOperation("notes.txt"), NewFindOperation("*.go"), NewGrepOperation("TODO", "*.go")} {
		decision := Decide(AutonomyOff, operation)
		if decision.Allowed || decision.Automatic || decision.Reason == "" {
			t.Fatalf("Decide(off, %#v) = %#v, want denied pending approval", operation, decision)
		}
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
