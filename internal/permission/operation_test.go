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

func TestWriteShapedOperationConstructorsPreserveEvidence(t *testing.T) {
	t.Parallel()

	edit := NewEditOperation("internal/demo.txt", "sha256:old", "-old\n+new", "replace demo contents")
	if edit.Kind != OperationMutation || edit.Tool != "edit" || edit.TargetPath != "internal/demo.txt" || edit.TargetVersion != "sha256:old" || edit.DiffPreview != "-old\n+new" || edit.ExpectedEffect == "" || !edit.Reversible {
		t.Fatalf("edit operation = %+v", edit)
	}

	write := NewWriteOperation("internal/generated.txt", "missing", "+created", "create generated file")
	if write.Kind != OperationMutation || write.Tool != "write" || write.TargetPath != "internal/generated.txt" || write.TargetVersion != "missing" || write.DiffPreview != "+created" || write.ExpectedEffect == "" || write.Reversible {
		t.Fatalf("write operation = %+v", write)
	}

	recovery := NewRecoveryOperation("undo", "internal/generated.txt", "sha256:new", "delete created file")
	if recovery.Kind != OperationMutation || recovery.Tool != "undo" || recovery.TargetPath != "internal/generated.txt" || recovery.TargetVersion != "sha256:new" || recovery.ExpectedEffect == "" || !recovery.Reversible {
		t.Fatalf("recovery operation = %+v", recovery)
	}

	command := []string{"sh", "-c", "printf updated > internal/demo.txt"}
	mutatingBash := NewMutatingBashOperation(command, ".", "update demo file")
	if mutatingBash.Kind != OperationExec || mutatingBash.Tool != "bash" || mutatingBash.WorkingDir != "." || mutatingBash.ExpectedEffect == "" || mutatingBash.Reversible {
		t.Fatalf("mutating bash operation = %+v", mutatingBash)
	}
	command[0] = "mutated"
	if mutatingBash.Command[0] != "sh" {
		t.Fatalf("mutating bash command aliases caller slice: %+v", mutatingBash.Command)
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

func TestDecideAutonomyMatrixForWriteShapedOperations(t *testing.T) {
	t.Parallel()

	operations := []ProposedOperation{
		NewReadOperation("notes.txt"),
		NewEditOperation("internal/demo.txt", "sha256:old", "-old\n+new", "replace demo contents"),
		NewWriteOperation("internal/generated.txt", "missing", "+created", "create generated file"),
		NewMutatingBashOperation([]string{"sh", "-c", "printf updated > internal/demo.txt"}, ".", "update demo file"),
	}
	for _, tc := range []struct {
		level            AutonomyLevel
		kind             OperationKind
		allowed          bool
		automatic        bool
		approvalRequired bool
		reason           string
	}{
		{level: AutonomyOff, kind: OperationRead, allowed: false, automatic: false, approvalRequired: true, reason: "autonomy off requires approval"},
		{level: AutonomyOff, kind: OperationMutation, allowed: false, automatic: false, approvalRequired: true, reason: "autonomy off requires approval"},
		{level: AutonomyOff, kind: OperationExec, allowed: false, automatic: false, approvalRequired: true, reason: "autonomy off requires approval"},
		{level: AutonomyRead, kind: OperationRead, allowed: true, automatic: true, approvalRequired: false, reason: "safe read-only operation"},
		{level: AutonomyRead, kind: OperationMutation, allowed: false, automatic: false, approvalRequired: true, reason: "read autonomy requires approval for write-shaped operation"},
		{level: AutonomyRead, kind: OperationExec, allowed: false, automatic: false, approvalRequired: true, reason: "read autonomy requires approval for write-shaped operation"},
		{level: AutonomyWrite, kind: OperationRead, allowed: true, automatic: true, approvalRequired: false, reason: "write autonomy allows classified operation"},
		{level: AutonomyWrite, kind: OperationMutation, allowed: true, automatic: true, approvalRequired: false, reason: "write autonomy allows classified operation"},
		{level: AutonomyWrite, kind: OperationExec, allowed: true, automatic: true, approvalRequired: false, reason: "write autonomy allows classified operation"},
		{level: AutonomyYolo, kind: OperationRead, allowed: true, automatic: true, approvalRequired: false, reason: "yolo autonomy grants classified operation"},
		{level: AutonomyYolo, kind: OperationMutation, allowed: true, automatic: true, approvalRequired: false, reason: "yolo autonomy grants classified operation"},
		{level: AutonomyYolo, kind: OperationExec, allowed: true, automatic: true, approvalRequired: false, reason: "yolo autonomy grants classified operation"},
	} {
		tc := tc
		t.Run(string(tc.level)+"/"+string(tc.kind), func(t *testing.T) {
			t.Parallel()

			for _, operation := range operations {
				if operation.Kind != tc.kind {
					continue
				}
				decision := Decide(tc.level, operation)
				if decision.Allowed != tc.allowed || decision.Automatic != tc.automatic || decision.Reason != tc.reason {
					t.Fatalf("Decide(%q, %#v) = %+v", tc.level, operation, decision)
				}
				record := RecordDecision(tc.level, operation, decision)
				if record.Source != decisionSourceAutonomyPolicy || record.ApprovalRequired != tc.approvalRequired || record.Allowed != tc.allowed || record.Automatic != tc.automatic || record.Reason != tc.reason {
					t.Fatalf("record = %+v, want allowed=%v automatic=%v approvalRequired=%v", record, tc.allowed, tc.automatic, tc.approvalRequired)
				}
			}
		})
	}
}

func TestDecideDeniesUnclassifiedOperationsWithoutApprovalPrompt(t *testing.T) {
	t.Parallel()

	operation := ProposedOperation{Kind: OperationKind("network_write"), Tool: "future"}
	for _, level := range []AutonomyLevel{AutonomyOff, AutonomyRead, AutonomyWrite, AutonomyYolo, AutonomyLevel("admin")} {
		decision := Decide(level, operation)
		if decision.Allowed || decision.Automatic || decision.Reason != "operation is not classified" {
			t.Fatalf("Decide(%q, unclassified) = %+v", level, decision)
		}
		record := RecordDecision(level, operation, decision)
		if record.ApprovalRequired {
			t.Fatalf("unclassified record requested approval: %+v", record)
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

func TestDecisionRecordCopiesPolicyEvidence(t *testing.T) {
	t.Parallel()

	operation := NewBashInspectionOperation([]string{"git", "status", "--short"}, ".", "inspect git working tree status")
	operation.RunID = "run-1"
	operation.Capability = "inspect"

	record := DecideRecord(AutonomyRead, operation)
	if record.Autonomy != AutonomyRead || record.Source != decisionSourceAutonomyPolicy || !record.Allowed || !record.Automatic || record.ApprovalRequired || record.Reason == "" || record.ManualAction != "" {
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

func TestDecisionRecordMarksReadBlockedWriteAsApprovalRequired(t *testing.T) {
	t.Parallel()

	record := DecideRecord(AutonomyRead, NewEditOperation("notes.txt", "sha256:old", "-old\n+new", "replace notes"))
	if record.Allowed || record.Automatic || !record.ApprovalRequired || record.Reason != "read autonomy requires approval for write-shaped operation" {
		t.Fatalf("read write record = %+v, want approval-required denial", record)
	}
	if record.OperationKind != OperationMutation || record.Tool != "edit" || record.TargetPath != "notes.txt" {
		t.Fatalf("write operation evidence should omit diff body from record fields not modeled here: %+v", record)
	}
}

func TestManualDecisionRecordCopiesExactProposalWithoutExecuting(t *testing.T) {
	t.Parallel()

	operation := NewMutatingBashOperation([]string{"sh", "-c", "printf updated > internal/demo.txt"}, ".", "update demo file")
	operation.RunID = "run-write"
	operation.Capability = "write-tool"

	for _, tc := range []struct {
		action           ManualAction
		wantAction       ManualAction
		allowed          bool
		approvalRequired bool
		wantReason       string
	}{
		{action: ManualActionApprove, wantAction: ManualActionApprove, allowed: true, approvalRequired: false, wantReason: "user approved proposed operation"},
		{action: ManualActionDeny, wantAction: ManualActionDeny, allowed: false, approvalRequired: false, wantReason: "user denied proposed operation"},
		{action: ManualActionDefer, wantAction: ManualActionDefer, allowed: false, approvalRequired: true, wantReason: "user deferred proposed operation"},
		{action: ManualAction("later"), wantAction: ManualActionDefer, allowed: false, approvalRequired: true, wantReason: "user deferred proposed operation"},
	} {
		tc := tc
		t.Run(string(tc.action), func(t *testing.T) {
			t.Parallel()

			proposal := operation
			proposal.Command = append([]string(nil), operation.Command...)
			record := RecordManualDecision(AutonomyRead, proposal, tc.action, "")
			if record.Source != decisionSourceManualApproval || record.Automatic || record.Allowed != tc.allowed || record.ApprovalRequired != tc.approvalRequired || record.ManualAction != tc.wantAction || record.Reason != tc.wantReason {
				t.Fatalf("manual record = %+v", record)
			}
			if record.Autonomy != AutonomyRead || record.OperationKind != OperationExec || record.Tool != "bash" || record.WorkingDir != "." || record.ExpectedEffect != "update demo file" || record.Reversible || record.RunID != "run-write" || record.Capability != "write-tool" {
				t.Fatalf("manual record operation evidence = %+v", record)
			}
			proposal.Command[0] = "mutated"
			if record.Command[0] != "sh" {
				t.Fatalf("manual record command aliases proposal slice: %+v", record.Command)
			}
		})
	}
}

func TestManualDecisionRecordUsesExplicitReason(t *testing.T) {
	t.Parallel()

	record := RecordManualDecision(AutonomyRead, NewWriteOperation("notes.txt", "missing", "+new", "create notes"), ManualActionDeny, "user said no")
	if record.Reason != "user said no" || record.ManualAction != ManualActionDeny || record.Allowed || record.ApprovalRequired {
		t.Fatalf("manual denial record = %+v", record)
	}
}
