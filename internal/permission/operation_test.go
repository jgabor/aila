package permission

import "testing"

func TestReadOperationClassifiesAsSafeReadOnly(t *testing.T) {
	t.Parallel()

	for _, operation := range []ProposedOperation{
		NewReadOperation("notes.txt"),
		NewFindOperation("**/*.go"),
		NewGrepOperation("TODO", "**/*.go"),
		NewFetchOperation("https://example.com/docs"),
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

	operations := []ProposedOperation{NewReadOperation("notes.txt"), NewFindOperation("**/*.go"), NewGrepOperation("TODO", "**/*.go"), NewFetchOperation("https://example.com/docs")}
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

	for _, operation := range []ProposedOperation{NewReadOperation("notes.txt"), NewFindOperation("*.go"), NewGrepOperation("TODO", "*.go"), NewFetchOperation("https://example.com/docs")} {
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

func TestBashInspectionOperationIsReadOnly(t *testing.T) {
	t.Parallel()

	operation := NewBashInspectionOperation([]string{"git", "status", "--short"}, ".", "inspect git working tree status")
	if operation.Kind != OperationRead || operation.Tool != "bash" || operation.WorkingDir != "." || operation.ExpectedEffect == "" || !operation.Reversible {
		t.Fatalf("bash inspection operation = %+v", operation)
	}
	if got := Decide(AutonomyRead, operation); !got.Allowed || !got.Automatic {
		t.Fatalf("read autonomy decision = %+v, want allowed", got)
	}
	if got := Decide(AutonomyOff, operation); got.Allowed || got.Automatic {
		t.Fatalf("off autonomy decision = %+v, want denied", got)
	}
}

func TestFetchOperationIsReadOnlyNetworkRead(t *testing.T) {
	t.Parallel()

	operation := NewFetchOperation("https://example.com/docs")
	if operation.Kind != OperationRead || operation.Tool != "fetch" || operation.TargetPath != "https://example.com/docs" || operation.ExpectedEffect == "" || !operation.Reversible {
		t.Fatalf("fetch operation = %+v", operation)
	}
	if len(operation.Command) != 0 || operation.WorkingDir != "" || operation.DiffPreview != "" || operation.TargetVersion != "" {
		t.Fatalf("fetch operation reused mutation or shell fields: %+v", operation)
	}
	if got := Decide(AutonomyRead, operation); !got.Allowed || !got.Automatic {
		t.Fatalf("read autonomy decision = %+v, want allowed", got)
	}
	if got := Decide(AutonomyOff, operation); got.Allowed || got.Automatic {
		t.Fatalf("off autonomy decision = %+v, want denied", got)
	}
}

func TestDecisionRecordCopiesReadOnlyPolicyEvidence(t *testing.T) {
	t.Parallel()

	operation := NewBashInspectionOperation([]string{"git", "status", "--short"}, ".", "inspect git working tree status")
	operation.RunID = "run-1"
	operation.Capability = "inspect"

	record := DecideRecord(AutonomyRead, operation)
	if record.Autonomy != AutonomyRead || record.Source != decisionSourceAutonomyPolicy || !record.Allowed || !record.Automatic || record.ApprovalRequired || record.Reason == "" {
		t.Fatalf("allowed record = %+v, want automatic read allow evidence", record)
	}
	if record.OperationKind != OperationRead || record.Tool != "bash" || record.WorkingDir != "." || record.ExpectedEffect == "" || !record.Reversible || record.RunID != "run-1" || record.Capability != "inspect" {
		t.Fatalf("operation evidence = %+v", record)
	}
	operation.Command[0] = "mutated"
	if record.Command[0] != "git" {
		t.Fatalf("record command changed after operation mutation: %+v", record.Command)
	}
}

func TestDecisionRecordMarksOffReadAsApprovalRequiredWithoutApproving(t *testing.T) {
	t.Parallel()

	record := DecideRecord(AutonomyOff, NewReadOperation("notes.txt"))
	if record.Allowed || record.Automatic || !record.ApprovalRequired || record.Reason != "autonomy off requires approval" {
		t.Fatalf("off read record = %+v, want blocked approval-required read evidence", record)
	}
	if record.OperationKind != OperationRead || record.Tool != "read" || record.TargetPath != "notes.txt" || record.ExpectedEffect == "" || !record.Reversible {
		t.Fatalf("off read operation evidence = %+v", record)
	}
}

func TestDecisionRecordDeniesNonReadWithoutApprovalPrompt(t *testing.T) {
	t.Parallel()

	record := DecideRecord(AutonomyRead, ProposedOperation{Kind: OperationMutation, Tool: "edit", TargetPath: "notes.txt", DiffPreview: "-old\n+new"})
	if record.Allowed || record.Automatic || record.ApprovalRequired || record.Reason == "" {
		t.Fatalf("non-read record = %+v, want automatic denial without approval requirement", record)
	}
	if record.OperationKind != OperationMutation || record.Tool != "edit" {
		t.Fatalf("non-read operation evidence = %+v", record)
	}
}
