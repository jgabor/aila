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
