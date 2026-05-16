package permission

// AutonomyLevel controls whether a classified operation may proceed automatically.
type AutonomyLevel string

const (
	AutonomyOff   AutonomyLevel = "off"
	AutonomyRead  AutonomyLevel = "read"
	AutonomyWrite AutonomyLevel = "write"
	AutonomyYolo  AutonomyLevel = "yolo"
)

// OperationKind classifies an operation by the effect it can have on the world.
type OperationKind string

const (
	OperationRead     OperationKind = "read"
	OperationMutation OperationKind = "mutation"
	OperationExec     OperationKind = "exec"
)

// ProposedOperation is the exact operation data considered by autonomy policy.
type ProposedOperation struct {
	Kind           OperationKind
	Tool           string
	TargetPath     string
	TargetVersion  string
	Command        []string
	WorkingDir     string
	ExpectedEffect string
	DiffPreview    string
	Reversible     bool
	RunID          string
	Capability     string
}

// Decision records a passive autonomy decision for one proposed operation.
type Decision struct {
	Allowed   bool
	Automatic bool
	Reason    string
}

const (
	decisionSourceAutonomyPolicy = "autonomy_policy"
	decisionSourceManualApproval = "manual_approval"
)

// ManualAction names an explicit user decision for a proposed operation.
type ManualAction string

const (
	ManualActionApprove ManualAction = "approve"
	ManualActionDeny    ManualAction = "deny"
	ManualActionDefer   ManualAction = "defer"
)

// DecisionRecord is the copied, serializable evidence for one autonomy decision.
type DecisionRecord struct {
	Autonomy         AutonomyLevel
	Source           string
	Allowed          bool
	Automatic        bool
	ApprovalRequired bool
	Reason           string
	OperationKind    OperationKind
	Tool             string
	TargetPath       string
	Command          []string
	WorkingDir       string
	ExpectedEffect   string
	Reversible       bool
	RunID            string
	Capability       string
	ManualAction     ManualAction
}

// NewReadOperation classifies the built-in read tool as read-only.
func NewReadOperation(targetPath string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationRead,
		Tool:           "read",
		TargetPath:     targetPath,
		ExpectedEffect: "bounded workspace file preview",
		Reversible:     true,
	}
}

// NewFindOperation classifies the built-in find tool as read-only discovery.
func NewFindOperation(pattern string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationRead,
		Tool:           "find",
		TargetPath:     pattern,
		ExpectedEffect: "bounded workspace file discovery",
		Reversible:     true,
	}
}

// NewGrepOperation classifies the built-in grep tool as read-only content search.
func NewGrepOperation(query string, includePattern string) ProposedOperation {
	target := query
	if includePattern != "" {
		target += " in " + includePattern
	}
	return ProposedOperation{
		Kind:           OperationRead,
		Tool:           "grep",
		TargetPath:     target,
		ExpectedEffect: "bounded workspace content search",
		Reversible:     true,
	}
}

// NewBashInspectionOperation classifies an allowed safe bash inspection command as read-only.
func NewBashInspectionOperation(command []string, workingDir string, expectedEffect string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationRead,
		Tool:           "bash",
		Command:        append([]string(nil), command...),
		WorkingDir:     workingDir,
		ExpectedEffect: expectedEffect,
		Reversible:     true,
	}
}

// NewFetchOperation classifies the built-in fetch tool as a read-only network operation.
func NewFetchOperation(targetURL string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationRead,
		Tool:           "fetch",
		TargetPath:     targetURL,
		ExpectedEffect: "bounded remote content preview",
		Reversible:     true,
	}
}

// NewEditOperation classifies a future edit proposal as a file mutation.
func NewEditOperation(targetPath string, targetVersion string, diffPreview string, expectedEffect string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationMutation,
		Tool:           "edit",
		TargetPath:     targetPath,
		TargetVersion:  targetVersion,
		ExpectedEffect: expectedEffect,
		DiffPreview:    diffPreview,
		Reversible:     true,
	}
}

// NewWriteOperation classifies a future write proposal as a file mutation.
func NewWriteOperation(targetPath string, targetVersion string, diffPreview string, expectedEffect string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationMutation,
		Tool:           "write",
		TargetPath:     targetPath,
		TargetVersion:  targetVersion,
		ExpectedEffect: expectedEffect,
		DiffPreview:    diffPreview,
		Reversible:     false,
	}
}

// NewRecoveryOperation classifies an app-owned undo or redo recovery as a file mutation.
func NewRecoveryOperation(command string, targetPath string, targetVersion string, expectedEffect string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationMutation,
		Tool:           command,
		TargetPath:     targetPath,
		TargetVersion:  targetVersion,
		ExpectedEffect: expectedEffect,
		Reversible:     true,
	}
}

// NewMutatingBashOperation classifies a future mutating shell command as exec.
func NewMutatingBashOperation(command []string, workingDir string, expectedEffect string) ProposedOperation {
	return ProposedOperation{
		Kind:           OperationExec,
		Tool:           "bash",
		Command:        append([]string(nil), command...),
		WorkingDir:     workingDir,
		ExpectedEffect: expectedEffect,
		Reversible:     false,
	}
}

// Decide applies the current autonomy level without executing or approving work.
func Decide(level AutonomyLevel, operation ProposedOperation) Decision {
	if !classifiedOperation(operation.Kind) {
		return Decision{Allowed: false, Automatic: false, Reason: "operation is not classified"}
	}
	switch level {
	case AutonomyOff:
		return Decision{Allowed: false, Automatic: false, Reason: "autonomy off requires approval"}
	case AutonomyRead:
		if operation.Kind == OperationRead {
			return Decision{Allowed: true, Automatic: true, Reason: "safe read-only operation"}
		}
		return Decision{Allowed: false, Automatic: false, Reason: "read autonomy requires approval for write-shaped operation"}
	case AutonomyWrite:
		return Decision{Allowed: true, Automatic: true, Reason: "write autonomy allows classified operation"}
	case AutonomyYolo:
		return Decision{Allowed: true, Automatic: true, Reason: "yolo autonomy grants classified operation"}
	default:
		return Decision{Allowed: false, Automatic: false, Reason: "unknown autonomy level"}
	}
}

// DecideRecord applies the autonomy policy and returns durable decision evidence.
func DecideRecord(level AutonomyLevel, operation ProposedOperation) DecisionRecord {
	return RecordDecision(level, operation, Decide(level, operation))
}

// RecordDecision copies the operation and policy outcome without executing work.
func RecordDecision(level AutonomyLevel, operation ProposedOperation, decision Decision) DecisionRecord {
	return DecisionRecord{
		Autonomy:         level,
		Source:           decisionSourceAutonomyPolicy,
		Allowed:          decision.Allowed,
		Automatic:        decision.Automatic,
		ApprovalRequired: approvalRequiredFor(level, operation, decision),
		Reason:           decision.Reason,
		OperationKind:    operation.Kind,
		Tool:             operation.Tool,
		TargetPath:       operation.TargetPath,
		Command:          append([]string(nil), operation.Command...),
		WorkingDir:       operation.WorkingDir,
		ExpectedEffect:   operation.ExpectedEffect,
		Reversible:       operation.Reversible,
		RunID:            operation.RunID,
		Capability:       operation.Capability,
	}
}

// RecordManualDecision copies an explicit user approval decision without executing work.
func RecordManualDecision(level AutonomyLevel, operation ProposedOperation, action ManualAction, reason string) DecisionRecord {
	action = normalizeManualAction(action)
	if reason == "" {
		reason = manualDecisionReason(action)
	}
	return DecisionRecord{
		Autonomy:         level,
		Source:           decisionSourceManualApproval,
		Allowed:          action == ManualActionApprove,
		Automatic:        false,
		ApprovalRequired: action == ManualActionDefer,
		Reason:           reason,
		OperationKind:    operation.Kind,
		Tool:             operation.Tool,
		TargetPath:       operation.TargetPath,
		Command:          append([]string(nil), operation.Command...),
		WorkingDir:       operation.WorkingDir,
		ExpectedEffect:   operation.ExpectedEffect,
		Reversible:       operation.Reversible,
		RunID:            operation.RunID,
		Capability:       operation.Capability,
		ManualAction:     action,
	}
}

func classifiedOperation(kind OperationKind) bool {
	return kind == OperationRead || kind == OperationMutation || kind == OperationExec
}

func approvalRequiredFor(level AutonomyLevel, operation ProposedOperation, decision Decision) bool {
	if decision.Allowed || !classifiedOperation(operation.Kind) {
		return false
	}
	return level == AutonomyOff || level == AutonomyRead
}

func normalizeManualAction(action ManualAction) ManualAction {
	switch action {
	case ManualActionApprove, ManualActionDeny, ManualActionDefer:
		return action
	default:
		return ManualActionDefer
	}
}

func manualDecisionReason(action ManualAction) string {
	switch action {
	case ManualActionApprove:
		return "user approved proposed operation"
	case ManualActionDeny:
		return "user denied proposed operation"
	default:
		return "user deferred proposed operation"
	}
}
