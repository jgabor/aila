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

const decisionSourceAutonomyPolicy = "autonomy_policy"

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

// Decide applies the current autonomy level without executing or approving work.
func Decide(level AutonomyLevel, operation ProposedOperation) Decision {
	switch level {
	case AutonomyRead, AutonomyWrite, AutonomyYolo:
		if operation.Kind == OperationRead {
			return Decision{Allowed: true, Automatic: true, Reason: "safe read-only operation"}
		}
	case AutonomyOff:
		return Decision{Allowed: false, Automatic: false, Reason: "autonomy off requires approval"}
	}
	return Decision{Allowed: false, Automatic: false, Reason: "operation is not allowed automatically"}
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
		ApprovalRequired: !decision.Allowed && level == AutonomyOff && operation.Kind == OperationRead,
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
