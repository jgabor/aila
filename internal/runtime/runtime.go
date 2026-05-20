package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/capability"
	"github.com/jgabor/aila/internal/diagnostic"
	"github.com/jgabor/aila/internal/utility"
	"github.com/jgabor/aila/internal/workflow"
)

// Status describes whether the runtime is waiting for user input or an operation
// result.
type Status string

const (
	StatusIdle            Status = "idle"
	StatusActive          Status = "active"
	StatusApprovalPending Status = "approval-pending"
	StatusCanceling       Status = "canceling"
	StatusCanceled        Status = "canceled"
	StatusPaused          Status = "paused"
)

// Message is an input to the deterministic runtime update function.
type Message interface {
	runtimeMessage()
}

// PromptSubmitted records user prompt text submitted through a presentation
// adapter.
type PromptSubmitted struct {
	Text string
}

func (PromptSubmitted) runtimeMessage() {}

// AgentPromptSubmitted records a prompt that should be handled by a provider-backed agent runner.
type AgentPromptSubmitted struct {
	Text      string
	Provider  string
	Model     string
	ToolNames []string
}

func (AgentPromptSubmitted) runtimeMessage() {}

// QueuedPromptDrainRequested records app intent to start the oldest queued
// prompt after the active turn has reached a terminal state.
type QueuedPromptDrainRequested struct {
	Provider  string
	Model     string
	ToolNames []string
}

func (QueuedPromptDrainRequested) runtimeMessage() {}

// CommandSelected records an inert command selected through a presentation
// adapter.
type CommandSelected struct {
	Name string
}

func (CommandSelected) runtimeMessage() {}

// CompactContextProposed records caller intent to compact current context.
type CompactContextProposed struct {
	Request CompactContextRequest
}

func (CompactContextProposed) runtimeMessage() {}

// BackgroundCompactContextProposed records utility intent to compact context
// only while primary runtime work can yield.
type BackgroundCompactContextProposed struct {
	Request CompactContextRequest
}

func (BackgroundCompactContextProposed) runtimeMessage() {}

// UtilityJobProposed records caller intent to run an idle-only utility job.
type UtilityJobProposed struct {
	Request utility.JobRequest
}

func (UtilityJobProposed) runtimeMessage() {}

// CapabilityProposed records caller intent to run a fixed built-in capability.
type CapabilityProposed struct {
	Request capability.Request
}

func (CapabilityProposed) runtimeMessage() {}

// SubagentSpawnProposed records caller intent to start supervised child work.
// Update records the child as observable state and emits a spawn effect; it
// does not choose orchestration policy.
type SubagentSpawnProposed struct {
	Request SubagentRequest
}

func (SubagentSpawnProposed) runtimeMessage() {}

// ReadToolProposed records caller intent to read a workspace file.
// Update turns it into an effect; app-owned dispatch performs validation and IO.
type ReadToolProposed struct {
	Request ReadToolRequest
}

func (ReadToolProposed) runtimeMessage() {}

// SearchToolProposed records caller intent to discover files or search content.
// Update turns it into an effect; app-owned dispatch performs validation and IO.
type SearchToolProposed struct {
	Request SearchToolRequest
}

func (SearchToolProposed) runtimeMessage() {}

// BashToolProposed records caller intent to run a safe inspection command.
// Update turns it into an effect; app-owned dispatch performs validation and IO.
type BashToolProposed struct {
	Request BashToolRequest
}

func (BashToolProposed) runtimeMessage() {}

// FetchToolProposed records caller intent to fetch remote content.
// Update turns it into an effect; app-owned dispatch performs validation and IO.
type FetchToolProposed struct {
	Request FetchToolRequest
}

func (FetchToolProposed) runtimeMessage() {}

// EditToolProposed records caller intent to edit a workspace file.
type EditToolProposed struct {
	Request MutationToolRequest
}

func (EditToolProposed) runtimeMessage() {}

// WriteToolProposed records caller intent to write a workspace file.
type WriteToolProposed struct {
	Request MutationToolRequest
}

func (WriteToolProposed) runtimeMessage() {}

// ApprovalProposed records inert risky operation data for user review.
// Update stores it for display only; it must not emit mutation effects.
type ApprovalProposed struct {
	Proposal ApprovalProposal
}

func (ApprovalProposed) runtimeMessage() {}

// ApprovalDecisionSelected records the user-selected approval action.
// The decision is typed state only in M25 and never executes mutations.
type ApprovalDecisionSelected struct {
	ProposalID string
	Action     ApprovalAction
	Reason     string
}

func (ApprovalDecisionSelected) runtimeMessage() {}

// InterruptRequested records user intent to stop the current fake operation.
type InterruptRequested struct {
	Reason string
}

func (InterruptRequested) runtimeMessage() {}

// AgentAssistantDelta records one provider-style assistant text delta.
type AgentAssistantDelta struct {
	Operation OperationMetadata
	Provider  string
	Model     string
	Sequence  int
	Text      string
}

func (AgentAssistantDelta) runtimeMessage() {}

// CapabilityOutputDelta records model text emitted while a fixed capability is
// active. It is kept separate from normal assistant transcript entries so
// capability runs cannot corrupt the chat transcript.
type CapabilityOutputDelta struct {
	Operation  OperationMetadata
	Capability capability.Name
	Sequence   int
	Text       string
}

func (CapabilityOutputDelta) runtimeMessage() {}

// AgentToolRequested records provider-requested tool metadata without execution.
type AgentToolRequested struct {
	Operation OperationMetadata
	Request   AgentToolRequest
}

func (AgentToolRequested) runtimeMessage() {}

// AgentTurnCompleted records provider-style turn completion metadata.
type AgentTurnCompleted struct {
	Operation    OperationMetadata
	Provider     string
	Model        string
	FinishReason string
}

func (AgentTurnCompleted) runtimeMessage() {}

// AgentTurnPaused records a resumable provider stop that did not complete or fail the turn.
type AgentTurnPaused struct {
	Operation OperationMetadata
	Provider  string
	Model     string
	Pause     AgentPauseMetadata
}

func (AgentTurnPaused) runtimeMessage() {}

// AgentTurnFailed records a bounded provider-style turn failure.
type AgentTurnFailed struct {
	Operation OperationMetadata
	Provider  string
	Model     string
	Failure   FailureMetadata
}

func (AgentTurnFailed) runtimeMessage() {}

// FakeEffectCompleted reports a deterministic in-memory fake effect result.
type FakeEffectCompleted struct {
	Operation OperationMetadata
	Result    string
}

func (FakeEffectCompleted) runtimeMessage() {}

// FakeEffectFailed reports deterministic failure metadata for a fake effect.
type FakeEffectFailed struct {
	Operation OperationMetadata
	Failure   FailureMetadata
}

func (FakeEffectFailed) runtimeMessage() {}

// ReadToolCompleted reports a read effect result, including bounded read errors.
type ReadToolCompleted struct {
	Operation OperationMetadata
	Result    ReadToolResult
}

func (ReadToolCompleted) runtimeMessage() {}

// SearchToolCompleted reports a find or grep effect result.
type SearchToolCompleted struct {
	Operation OperationMetadata
	Result    SearchToolResult
}

func (SearchToolCompleted) runtimeMessage() {}

// BashToolCompleted reports a safe inspection command effect result.
type BashToolCompleted struct {
	Operation OperationMetadata
	Result    BashToolResult
}

func (BashToolCompleted) runtimeMessage() {}

// CompactContextCompleted reports a context compaction result.
type CompactContextCompleted struct {
	Operation OperationMetadata
	Result    CompactContextResult
}

func (CompactContextCompleted) runtimeMessage() {}

// UtilityJobCompleted reports a deterministic utility job result.
type UtilityJobCompleted struct {
	Operation OperationMetadata
	Result    utility.JobResult
}

func (UtilityJobCompleted) runtimeMessage() {}

// CapabilityCompleted reports one fixed capability exit payload.
type CapabilityCompleted struct {
	Operation OperationMetadata
	Payload   capability.ExitPayload
}

func (CapabilityCompleted) runtimeMessage() {}

// SubagentProgressed reports visible supervised child progress.
type SubagentProgressed struct {
	ID            string
	ParentRunID   string
	Purpose       string
	Status        SubagentStatus
	Summary       string
	EvidenceLinks []SubagentEvidenceLink
}

func (SubagentProgressed) runtimeMessage() {}

// SubagentCompleted reports a supervised child result.
type SubagentCompleted struct {
	ParentRunID string
	Result      SubagentResult
}

func (SubagentCompleted) runtimeMessage() {}

// SubagentFailed reports supervised child failure metadata.
type SubagentFailed struct {
	ID            string
	ParentRunID   string
	Purpose       string
	Failure       FailureMetadata
	Summary       string
	EvidenceLinks []SubagentEvidenceLink
}

func (SubagentFailed) runtimeMessage() {}

// SubagentCanceled reports supervised child cancellation metadata.
type SubagentCanceled struct {
	ID            string
	ParentRunID   string
	Purpose       string
	Cancel        CancelMetadata
	Summary       string
	EvidenceLinks []SubagentEvidenceLink
}

func (SubagentCanceled) runtimeMessage() {}

// FetchToolCompleted reports a network read effect result.
type FetchToolCompleted struct {
	Operation OperationMetadata
	Result    FetchToolResult
}

func (FetchToolCompleted) runtimeMessage() {}

// MutationToolCompleted reports an edit/write effect result.
type MutationToolCompleted struct {
	Operation OperationMetadata
	Result    MutationToolResult
}

func (MutationToolCompleted) runtimeMessage() {}

// FakeInterruptResolved reports that the in-memory fake interrupt path resolved.
type FakeInterruptResolved struct {
	Operation OperationMetadata
	Cancel    CancelMetadata
}

func (FakeInterruptResolved) runtimeMessage() {}

// RuntimeDiagnostic records supervised runtime or effect diagnostics as typed
// runtime messages. It is passive data; Update decides how it affects state.
type RuntimeDiagnostic struct {
	Diagnostic diagnostic.Diagnostic
}

func (RuntimeDiagnostic) runtimeMessage() {}

// Model is runtime-owned state for the current fake interaction surface.
type Model struct {
	CurrentPhase         workflow.Phase
	Status               Status
	Transcript           []TranscriptEntry
	Queued               []QueuedEntry
	Result               string
	LastCommand          string
	ActiveUtility        utility.JobRequest
	LastUtility          utility.JobResult
	ActiveCapability     capability.Request
	LastCapability       capability.ExitPayload
	CapabilityDraft      string
	Subagents            []SubagentRun
	ActiveCompact        CompactContextRequest
	LastCompact          CompactContextResult
	ActiveRead           ReadToolRequest
	LastRead             ReadToolResult
	ActiveSearch         SearchToolRequest
	LastSearch           SearchToolResult
	ActiveBash           BashToolRequest
	LastBash             BashToolResult
	ActiveFetch          FetchToolRequest
	LastFetch            FetchToolResult
	ActiveMutation       MutationToolRequest
	LastMutation         MutationToolResult
	PendingApproval      ApprovalProposal
	LastApprovalDecision ApprovalDecision
	AssistantDraft       string
	AgentProvider        string
	AgentModel           string
	AgentToolNames       []string
	LastAgentToolRequest AgentToolRequest
	AgentFinishReason    string
	LastAgentPause       AgentPauseMetadata
	LastAgentFailure     FailureMetadata
	NextOperation        int
	ActiveOperation      OperationMetadata
	Diagnostics          []diagnostic.Diagnostic
}

// AgentToolArgument records one provider-supplied tool argument.
type AgentToolArgument struct {
	Name  string
	Value string
}

// AgentToolRequest records provider-requested tool metadata without execution.
type AgentToolRequest struct {
	ID        string
	Name      string
	Arguments []AgentToolArgument
	Provider  string
	Model     string
	Sequence  int
}

// AgentPauseMetadata describes a bounded resumable stop surfaced by the agent adapter.
type AgentPauseMetadata struct {
	Reason     string
	Message    string
	Resumable  bool
	Suggestion string
}

// QueuedEntry is a user message queued while fake work is active.
type QueuedEntry struct {
	Kind string
	Text string
}

// TranscriptEntry is a deterministic surface entry for prompt, command, result,
// and failure messages.
type TranscriptEntry struct {
	Kind string
	Text string
}

// Effect is a typed operation request returned by Update for later execution.
type Effect interface {
	runtimeEffect()
	Metadata() OperationMetadata
}

// FakePromptEffect requests fake in-memory handling for a prompt.
type FakePromptEffect struct {
	Operation OperationMetadata
	Prompt    string
}

func (FakePromptEffect) runtimeEffect() {}

func (effect FakePromptEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// AgentPromptEffect requests provider-backed agent handling outside Update.
type AgentPromptEffect struct {
	Operation OperationMetadata
	Prompt    string
	Provider  string
	Model     string
	ToolNames []string
}

func (AgentPromptEffect) runtimeEffect() {}

func (effect AgentPromptEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// FakeCommandEffect requests fake in-memory handling for a command.
type FakeCommandEffect struct {
	Operation OperationMetadata
	Command   string
}

func (FakeCommandEffect) runtimeEffect() {}

func (effect FakeCommandEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// CompactContextEffect requests context compaction outside Update.
type CompactContextEffect struct {
	Operation OperationMetadata
	Request   CompactContextRequest
}

func (CompactContextEffect) runtimeEffect() {}

func (effect CompactContextEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// UtilityJobEffect requests a deterministic idle-only utility job.
type UtilityJobEffect struct {
	Operation OperationMetadata
	Request   utility.JobRequest
}

func (UtilityJobEffect) runtimeEffect() {}

func (effect UtilityJobEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// CapabilityEffect requests fixed built-in capability execution outside Update.
type CapabilityEffect struct {
	Operation OperationMetadata
	Request   capability.Request
	Execution capability.PreparedExecution
}

func (CapabilityEffect) runtimeEffect() {}

func (effect CapabilityEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// SpawnSubagentEffect requests supervised child work outside Update.
type SpawnSubagentEffect struct {
	Operation OperationMetadata
	Request   SubagentRequest
}

func (SpawnSubagentEffect) runtimeEffect() {}

func (effect SpawnSubagentEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// ReadToolEffect requests read-only workspace file execution outside Update.
type ReadToolEffect struct {
	Operation OperationMetadata
	Request   ReadToolRequest
}

func (ReadToolEffect) runtimeEffect() {}

func (effect ReadToolEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// SearchToolEffect requests read-only workspace discovery or content search outside Update.
type SearchToolEffect struct {
	Operation OperationMetadata
	Request   SearchToolRequest
}

func (SearchToolEffect) runtimeEffect() {}

func (effect SearchToolEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// BashToolEffect requests safe inspection command execution outside Update.
type BashToolEffect struct {
	Operation OperationMetadata
	Request   BashToolRequest
}

func (BashToolEffect) runtimeEffect() {}

func (effect BashToolEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// FetchToolEffect requests remote read execution outside Update.
type FetchToolEffect struct {
	Operation OperationMetadata
	Request   FetchToolRequest
}

func (FetchToolEffect) runtimeEffect() {}

func (effect FetchToolEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// EditToolEffect requests workspace edit execution outside Update.
type EditToolEffect struct {
	Operation OperationMetadata
	Request   MutationToolRequest
}

func (EditToolEffect) runtimeEffect() {}

func (effect EditToolEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// WriteToolEffect requests workspace write execution outside Update.
type WriteToolEffect struct {
	Operation OperationMetadata
	Request   MutationToolRequest
}

func (WriteToolEffect) runtimeEffect() {}

func (effect WriteToolEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// FakeInterruptEffect requests fake in-memory interruption of active work.
type FakeInterruptEffect struct {
	Operation OperationMetadata
	Cancel    CancelMetadata
}

func (FakeInterruptEffect) runtimeEffect() {}

func (effect FakeInterruptEffect) Metadata() OperationMetadata {
	return effect.Operation
}

// Dispatch interprets fake in-memory effects synchronously and returns result
// messages in the same order as the input effects.
func Dispatch(effects []Effect) []Message {
	return DispatchContext(context.Background(), effects)
}

// DispatchContext interprets fake in-memory effects and records context
// cancellation as a typed diagnostic message.
func DispatchContext(ctx context.Context, effects []Effect) []Message {
	if len(effects) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return []Message{CancellationMessage(diagnostic.SourceEffect, err)}
	}

	messages := make([]Message, 0, len(effects))
	for _, effect := range effects {
		messages = append(messages, dispatchOne(effect)...)
	}

	return messages
}

func dispatchOne(effect Effect) (messages []Message) {
	defer func() {
		if recovered := recover(); recovered != nil {
			messages = []Message{PanicMessage(diagnostic.SourceEffect, recovered)}
		}
	}()

	switch typed := effect.(type) {
	case FakePromptEffect:
		return []Message{FakeEffectCompleted{
			Operation: typed.Operation,
			Result:    "Fake Aila response: " + typed.Prompt,
		}}
	case FakeCommandEffect:
		return []Message{FakeEffectCompleted{
			Operation: typed.Operation,
			Result:    "fake command result: " + typed.Command,
		}}
	case CompactContextEffect:
		return []Message{CompactContextCompleted{
			Operation: typed.Operation,
			Result:    fakeCompactContextResult(typed.Request),
		}}
	case UtilityJobEffect:
		return []Message{UtilityJobCompleted{
			Operation: typed.Operation,
			Result:    utility.RunJob(typed.Request),
		}}
	case CapabilityEffect:
		payload, err := capability.RunBuiltIn(context.Background(), typed.Request)
		if err != nil {
			payload = capability.ExitPayload{
				Capability: typed.Request.Capability,
				Signal:     capability.ExitStuck,
				Blocker:    err.Error(),
				Attempted:  true,
			}
		}
		return []Message{CapabilityCompleted{Operation: typed.Operation, Payload: payload}}
	case SpawnSubagentEffect:
		return []Message{SubagentProgressed{
			ID:            typed.Request.ID,
			ParentRunID:   typed.Request.ParentRunID,
			Purpose:       typed.Request.Purpose,
			Status:        SubagentStatusRunning,
			Summary:       "subagent running: " + typed.Request.Purpose,
			EvidenceLinks: typed.Request.EvidenceLinks,
		}}
	case FakeInterruptEffect:
		return []Message{FakeInterruptResolved(typed)}
	case interface{ dispatchPanic() }:
		typed.dispatchPanic()
	}
	return nil
}

// PanicMessage converts recovered panic data into a typed diagnostic message.
func PanicMessage(source diagnostic.Source, recovered any) RuntimeDiagnostic {
	category := diagnostic.CategoryRuntime
	if source == diagnostic.SourceEffect {
		category = diagnostic.CategoryEffect
	}
	return RuntimeDiagnostic{Diagnostic: diagnostic.New(diagnostic.Spec{
		Category:         category,
		Source:           source,
		Severity:         diagnostic.SeverityError,
		Message:          fmt.Sprintf("supervised %s panic recovered: %v", source, recovered),
		AffectedArtifact: diagnostic.ArtifactRuntimeEffect,
		RecoveryAction:   diagnostic.RecoveryInspect,
		UserInputNeeded:  true,
	})}
}

// CancellationMessage converts handled context cancellation into a typed
// diagnostic message.
func CancellationMessage(source diagnostic.Source, err error) RuntimeDiagnostic {
	message := "runtime work canceled"
	if err != nil {
		message += ": " + err.Error()
	}
	return RuntimeDiagnostic{Diagnostic: diagnostic.New(diagnostic.Spec{
		Category:         diagnostic.CategoryCancellation,
		Source:           source,
		Severity:         diagnostic.SeverityWarning,
		Message:          message,
		AffectedArtifact: diagnostic.ArtifactRuntimeEffect,
		RecoveryAction:   diagnostic.RecoveryIgnoreForRun,
		UserInputNeeded:  false,
	})}
}

// OperationKind classifies a requested operation without performing it.
type OperationKind string

const (
	OperationPrompt     OperationKind = "prompt"
	OperationCommand    OperationKind = "command"
	OperationUtility    OperationKind = "utility"
	OperationCapability OperationKind = "capability"
	OperationCompact    OperationKind = "compact"
	OperationRead       OperationKind = "read"
	OperationFind       OperationKind = "find"
	OperationGrep       OperationKind = "grep"
	OperationBash       OperationKind = "bash"
	OperationFetch      OperationKind = "fetch"
	OperationEdit       OperationKind = "edit"
	OperationWrite      OperationKind = "write"
	OperationApproval   OperationKind = "approval"
	OperationSubagent   OperationKind = "subagent"
)

// OperationMetadata is inert typed data for future permission and dispatch
// decisions.
type OperationMetadata struct {
	ID      string
	Kind    OperationKind
	Subject string
	Source  string
	Failure FailureMetadata
	Cancel  CancelMetadata
}

// FailureMetadata is inert typed data describing a fake failure.
type FailureMetadata struct {
	Code      string
	Message   string
	Retryable bool
}

// CancelMetadata is inert typed data reserved for future cancellation surfaces.
type CancelMetadata struct {
	Requested bool
	Reason    string
}

// SubagentStatus records the supervised child lifecycle.
type SubagentStatus string

const (
	SubagentStatusQueued    SubagentStatus = "queued"
	SubagentStatusRunning   SubagentStatus = "running"
	SubagentStatusCompleted SubagentStatus = "completed"
	SubagentStatusFailed    SubagentStatus = "failed"
	SubagentStatusCanceled  SubagentStatus = "canceled"
)

// Active reports whether the child still needs supervision.
func (status SubagentStatus) Active() bool {
	return status == SubagentStatusQueued || status == SubagentStatusRunning
}

// SubagentBudget records bounded child-work limits.
type SubagentBudget struct {
	MaxTurns      int
	MaxTokens     int
	TimeoutMillis int
}

// SubagentSourceMetadata records caller-visible child-work provenance.
type SubagentSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// SubagentEvidenceLink records evidence exposed by supervised child work.
type SubagentEvidenceLink struct {
	ID      string
	Kind    string
	Path    string
	Command string
	Excerpt string
}

// SubagentRequest is runtime-owned child-work proposal data. It is inert and
// contains constraints only; execution policy remains outside this slice.
type SubagentRequest struct {
	ID            string
	ParentRunID   string
	Purpose       string
	Input         string
	Tools         []string
	Budget        SubagentBudget
	EvidenceLinks []SubagentEvidenceLink
	Source        SubagentSourceMetadata
}

// SubagentResult records the terminal child output summary.
type SubagentResult struct {
	ID            string
	ParentRunID   string
	Purpose       string
	Summary       string
	EvidenceLinks []SubagentEvidenceLink
}

// SubagentRun records supervised child work kept visible to callers.
type SubagentRun struct {
	ID            string
	ParentRunID   string
	Purpose       string
	Input         string
	Tools         []string
	Budget        SubagentBudget
	Status        SubagentStatus
	Summary       string
	EvidenceLinks []SubagentEvidenceLink
	Failure       FailureMetadata
	Cancel        CancelMetadata
	Source        SubagentSourceMetadata
}

// CompactContextRequest is primitive context data for compaction.
type CompactContextRequest struct {
	Blocks     []CompactContextBlock
	SourceRefs []CompactSourceRef
	Claims     []CompactContextClaim
	Budget     CompactContextBudget
	Warnings   []string
	MaxBytes   int
	Source     CompactSourceMetadata
}

// CompactContextBlock records one compactable context block.
type CompactContextBlock struct {
	ID           string
	Kind         string
	Title        string
	Text         string
	SourceRefIDs []string
}

// CompactContextClaim records one source-backed context claim.
type CompactContextClaim struct {
	Text         string
	SourceRefIDs []string
}

// CompactSourceRef records exact supporting evidence for compacted context.
type CompactSourceRef struct {
	ID        string
	Kind      string
	Label     string
	Path      string
	LineStart int
	LineEnd   int
	Command   string
	Stream    string
	Excerpt   string
}

// CompactContextBudget records context size before or after compaction.
type CompactContextBudget struct {
	MaxBytes       int
	UsedBytes      int
	BlockCount     int
	SourceRefCount int
	ClaimCount     int
	Truncated      bool
}

// CompactMode names the caller-visible compaction path.
type CompactMode string

const (
	CompactModeManual     CompactMode = "manual"
	CompactModeBackground CompactMode = "background"
)

// CompactSourceMetadata records caller-visible compaction provenance.
type CompactSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
	Mode        CompactMode
}

// CompactContextErrorKind is a bounded manual compaction failure category.
type CompactContextErrorKind string

const (
	CompactContextErrorNone      CompactContextErrorKind = "none"
	CompactContextErrorNoContext CompactContextErrorKind = "no_context"
)

// CompactContextError is safe to surface in the TUI.
type CompactContextError struct {
	Kind    CompactContextErrorKind
	Message string
}

// CompactContextResult is the typed runtime payload returned by compaction effects.
type CompactContextResult struct {
	Status         string
	Summary        string
	Blocks         []CompactContextBlock
	SourceRefs     []CompactSourceRef
	Claims         []CompactContextClaim
	OriginalBudget CompactContextBudget
	Budget         CompactContextBudget
	Caveats        []string
	Error          CompactContextError
	Source         CompactSourceMetadata
}

// ReadToolRequest is the runtime-owned read proposal data. It intentionally
// mirrors only primitive request fields so runtime stays filesystem-free.
type ReadToolRequest struct {
	Path            string
	StartLine       int
	LineLimit       int
	MaxPreviewBytes int
	Source          ReadSourceMetadata
}

// ReadSourceMetadata records caller-visible provenance for a read request.
type ReadSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// ReadLineRange records inclusive 1-based line bounds for read results.
type ReadLineRange struct {
	StartLine int
	EndLine   int
	Limit     int
}

// ReadTruncation records bounded preview truncation decisions.
type ReadTruncation struct {
	PreviewBytesLimit int
	PreviewTruncated  bool
	LineLimitHit      bool
	Marker            string
}

// ReadToolErrorKind is a bounded machine-readable read failure category.
type ReadToolErrorKind string

const (
	ReadToolErrorNone              ReadToolErrorKind = "none"
	ReadToolErrorInvalidPath       ReadToolErrorKind = "invalid_path"
	ReadToolErrorOutsideWorkspace  ReadToolErrorKind = "outside_workspace"
	ReadToolErrorReservedPath      ReadToolErrorKind = "reserved_path"
	ReadToolErrorDirectoryLikePath ReadToolErrorKind = "directory_like_path"
	ReadToolErrorInvalidRange      ReadToolErrorKind = "invalid_range"
	ReadToolErrorMissingFile       ReadToolErrorKind = "missing_file"
	ReadToolErrorDirectory         ReadToolErrorKind = "directory"
	ReadToolErrorPermission        ReadToolErrorKind = "permission_denied"
	ReadToolErrorSymlinkEscape     ReadToolErrorKind = "symlink_escape"
	ReadToolErrorBinaryContent     ReadToolErrorKind = "binary_content"
	ReadToolErrorOversizedFile     ReadToolErrorKind = "oversized_file"
	ReadToolErrorCanceled          ReadToolErrorKind = "canceled"
	ReadToolErrorExecution         ReadToolErrorKind = "execution_error"
)

// ReadToolError is safe to surface without leaking host-local paths.
type ReadToolError struct {
	Kind    ReadToolErrorKind
	Message string
}

// ToolDecision records app-owned autonomy policy evidence for a tool result.
type ToolDecision struct {
	Present          bool
	Autonomy         string
	Source           string
	Allowed          bool
	Automatic        bool
	ApprovalRequired bool
	Reason           string
	OperationKind    string
	Tool             string
	Target           string
	Command          []string
	WorkingDir       string
	ExpectedEffect   string
	Reversible       bool
	RunID            string
	Capability       string
}

// ReadToolResult is the typed runtime message payload returned by read effects.
type ReadToolResult struct {
	ToolName              string
	RequestedPath         string
	WorkspaceRelativePath string
	ResolvedPath          string
	ResolvedPathAvailable bool
	RequestedRange        ReadLineRange
	EffectiveRange        ReadLineRange
	PreviewText           string
	Truncation            ReadTruncation
	Error                 ReadToolError
	Source                ReadSourceMetadata
	Decision              ToolDecision
}

// SearchToolName names one of the fixed read-only search tools.
type SearchToolName string

const (
	SearchToolFind SearchToolName = "find"
	SearchToolGrep SearchToolName = "grep"
)

// SearchToolRequest is the runtime-owned find/grep proposal data.
// It intentionally mirrors only primitive request fields so runtime stays filesystem-free.
type SearchToolRequest struct {
	ToolName        SearchToolName
	Pattern         string
	Query           string
	Regex           bool
	IncludePattern  string
	MaxResults      int
	MaxPreviewBytes int
	Source          SearchSourceMetadata
}

// SearchSourceMetadata records caller-visible provenance for search requests.
type SearchSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// SearchToolErrorKind is a bounded machine-readable search failure category.
type SearchToolErrorKind string

const (
	SearchToolErrorNone             SearchToolErrorKind = "none"
	SearchToolErrorInvalidPath      SearchToolErrorKind = "invalid_path"
	SearchToolErrorOutsideWorkspace SearchToolErrorKind = "outside_workspace"
	SearchToolErrorReservedPath     SearchToolErrorKind = "reserved_path"
	SearchToolErrorInvalidPattern   SearchToolErrorKind = "invalid_pattern"
	SearchToolErrorInvalidQuery     SearchToolErrorKind = "invalid_query"
	SearchToolErrorInvalidRange     SearchToolErrorKind = "invalid_range"
	SearchToolErrorPermission       SearchToolErrorKind = "permission_denied"
	SearchToolErrorSymlinkEscape    SearchToolErrorKind = "symlink_escape"
	SearchToolErrorCanceled         SearchToolErrorKind = "canceled"
	SearchToolErrorExecution        SearchToolErrorKind = "execution_error"
)

// SearchToolError is safe to surface without leaking host-local paths.
type SearchToolError struct {
	Kind    SearchToolErrorKind
	Message string
}

// SearchToolMatch records one find or grep match for runtime and presentation.
type SearchToolMatch struct {
	Path        string
	LineNumber  int
	PreviewText string
}

// SearchToolTruncation records bounded search omission and truncation metadata.
type SearchToolTruncation struct {
	MaxResults        int
	MaxPreviewBytes   int
	OmittedResults    int
	OmittedFiles      int
	PreviewTruncated  bool
	ResultLimitHit    bool
	FileSkipCount     int
	TruncationMarkers string
}

// SearchToolResult is the typed runtime message payload returned by find/grep effects.
type SearchToolResult struct {
	ToolName       string
	Pattern        string
	Query          string
	Regex          bool
	IncludePattern string
	Matches        []SearchToolMatch
	Truncation     SearchToolTruncation
	Error          SearchToolError
	Source         SearchSourceMetadata
	Decision       ToolDecision
}

// BashToolRequest is the runtime-owned safe bash proposal data.
// It intentionally mirrors only primitive request fields so runtime stays IO-free.
type BashToolRequest struct {
	Argv           []string
	WorkingDir     string
	MaxOutputBytes int
	TimeoutMillis  int
	Source         BashSourceMetadata
}

// BashSourceMetadata records caller-visible provenance for safe bash requests.
type BashSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// BashToolErrorKind is a bounded machine-readable safe bash failure category.
type BashToolErrorKind string

const (
	BashToolErrorNone             BashToolErrorKind = "none"
	BashToolErrorInvalidCommand   BashToolErrorKind = "invalid_command"
	BashToolErrorUnsafeCommand    BashToolErrorKind = "unsafe_command"
	BashToolErrorInvalidPath      BashToolErrorKind = "invalid_path"
	BashToolErrorOutsideWorkspace BashToolErrorKind = "outside_workspace"
	BashToolErrorReservedPath     BashToolErrorKind = "reserved_path"
	BashToolErrorInvalidRange     BashToolErrorKind = "invalid_range"
	BashToolErrorPermission       BashToolErrorKind = "permission_denied"
	BashToolErrorCanceled         BashToolErrorKind = "canceled"
	BashToolErrorTimeout          BashToolErrorKind = "timeout"
	BashToolErrorExecution        BashToolErrorKind = "execution_error"
)

// BashToolError is safe to surface without leaking host-local paths.
type BashToolError struct {
	Kind    BashToolErrorKind
	Message string
}

// BashToolOutput records one bounded command output stream.
type BashToolOutput struct {
	Text      string
	Bytes     int
	Truncated bool
}

// BashToolResult is the typed runtime message payload returned by bash effects.
type BashToolResult struct {
	ToolName                 string
	RequestedArgv            []string
	EffectiveArgv            []string
	WorkspaceRelativeWorkDir string
	CommandFamily            string
	ExpectedEffect           string
	ExitCode                 int
	Status                   string
	Stdout                   BashToolOutput
	Stderr                   BashToolOutput
	DurationMillis           int64
	Error                    BashToolError
	Source                   BashSourceMetadata
	Decision                 ToolDecision
}

// FetchToolRequest is the runtime-owned fetch proposal data.
// It intentionally mirrors only primitive request fields so runtime stays IO-free.
type FetchToolRequest struct {
	URL             string
	Method          string
	MaxPreviewBytes int
	TimeoutMillis   int
	Source          FetchSourceMetadata
}

// FetchSourceMetadata records caller-visible provenance for fetch requests.
type FetchSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// FetchToolErrorKind is a bounded machine-readable fetch failure category.
type FetchToolErrorKind string

const (
	FetchToolErrorNone              FetchToolErrorKind = "none"
	FetchToolErrorInvalidURL        FetchToolErrorKind = "invalid_url"
	FetchToolErrorUnsupportedScheme FetchToolErrorKind = "unsupported_scheme"
	FetchToolErrorInvalidMethod     FetchToolErrorKind = "invalid_method"
	FetchToolErrorInvalidRange      FetchToolErrorKind = "invalid_range"
	FetchToolErrorPermission        FetchToolErrorKind = "permission_denied"
	FetchToolErrorHTTPStatus        FetchToolErrorKind = "http_status"
	FetchToolErrorCanceled          FetchToolErrorKind = "canceled"
	FetchToolErrorTimeout           FetchToolErrorKind = "timeout"
	FetchToolErrorContent           FetchToolErrorKind = "content_error"
	FetchToolErrorExecution         FetchToolErrorKind = "execution_error"
)

// FetchToolError is safe to surface without leaking host-local paths.
type FetchToolError struct {
	Kind    FetchToolErrorKind
	Message string
}

// FetchToolTruncation records bounded network body omission metadata.
type FetchToolTruncation struct {
	PreviewBytesLimit int
	PreviewTruncated  bool
	OmittedBytesKnown bool
	OmittedBytes      int64
	Marker            string
}

// FetchToolResult is the typed runtime message payload returned by fetch effects.
type FetchToolResult struct {
	ToolName       string
	RequestedURL   string
	EffectiveURL   string
	Method         string
	ExpectedEffect string
	Status         string
	HTTPStatusCode int
	HTTPStatus     string
	ContentType    string
	PreviewText    string
	Truncation     FetchToolTruncation
	DurationMillis int64
	Error          FetchToolError
	Source         FetchSourceMetadata
	Decision       ToolDecision
}

// ApprovalAction names one user-selectable action for an approval proposal.
// MutationToolName names one of the fixed file mutation tools.
type MutationToolName string

const (
	MutationToolEdit  MutationToolName = "edit"
	MutationToolWrite MutationToolName = "write"
)

// MutationToolRequest is the runtime-owned edit/write proposal data.
// It intentionally mirrors only primitive fields so runtime stays IO-free.
type MutationToolRequest struct {
	ToolName       MutationToolName
	Path           string
	TargetVersion  string
	OldText        string
	NewText        string
	Content        string
	ExpectedEffect string
	Source         MutationSourceMetadata
}

// MutationSourceMetadata records caller-visible provenance for mutation requests.
type MutationSourceMetadata struct {
	Caller      string
	RequestID   string
	Description string
}

// MutationToolErrorKind is a bounded machine-readable mutation failure category.
type MutationToolErrorKind string

const (
	MutationToolErrorNone                  MutationToolErrorKind = "none"
	MutationToolErrorInvalidPath           MutationToolErrorKind = "invalid_path"
	MutationToolErrorOutsideWorkspace      MutationToolErrorKind = "outside_workspace"
	MutationToolErrorReservedPath          MutationToolErrorKind = "reserved_path"
	MutationToolErrorDirectoryLikePath     MutationToolErrorKind = "directory_like_path"
	MutationToolErrorInvalidContent        MutationToolErrorKind = "invalid_content"
	MutationToolErrorMissingFile           MutationToolErrorKind = "missing_file"
	MutationToolErrorDirectory             MutationToolErrorKind = "directory"
	MutationToolErrorPermission            MutationToolErrorKind = "permission_denied"
	MutationToolErrorSymlinkEscape         MutationToolErrorKind = "symlink_escape"
	MutationToolErrorTargetVersionMismatch MutationToolErrorKind = "target_version_mismatch"
	MutationToolErrorOldTextMismatch       MutationToolErrorKind = "old_text_mismatch"
	MutationToolErrorCanceled              MutationToolErrorKind = "canceled"
	MutationToolErrorExecution             MutationToolErrorKind = "execution_error"
)

// MutationToolError is safe to surface without leaking host-local paths.
type MutationToolError struct {
	Kind    MutationToolErrorKind
	Message string
}

// MutationToolResult is the typed runtime message payload returned by edit/write effects.
type MutationToolResult struct {
	ToolName              string
	RequestedPath         string
	WorkspaceRelativePath string
	ResolvedPath          string
	ResolvedPathAvailable bool
	Status                string
	ExpectedEffect        string
	PreviousVersion       string
	NewVersion            string
	PreviousExists        bool
	BytesWritten          int
	ReplacementCount      int
	Error                 MutationToolError
	Source                MutationSourceMetadata
	Decision              ToolDecision
}

type ApprovalAction string

const (
	ApprovalActionApprove ApprovalAction = "approve"
	ApprovalActionDeny    ApprovalAction = "deny"
	ApprovalActionDefer   ApprovalAction = "defer"
)

// ApprovalProposal is generic, inert risky-operation display data. It does not
// imply final write permission classes or contain an executable mutation.
type ApprovalProposal struct {
	ID             string
	OperationKind  string
	Target         string
	RiskSummary    string
	Preview        []string
	DefaultAction  ApprovalAction
	Path           string
	Command        []string
	WorkingDir     string
	ExpectedEffect string
	DiffPreview    []string
	Reversible     bool
	RunID          string
	Capability     string
}

// ApprovalDecision records the selected action for a proposal without executing
// the proposed operation.
type ApprovalDecision struct {
	ProposalID string
	Action     ApprovalAction
	Reason     string
	Stale      bool
}

// Update applies one runtime message and returns the next model plus typed
// effects for an external interpreter.
func Update(model Model, message Message) (Model, []Effect) {
	next := model
	next.Transcript = append([]TranscriptEntry(nil), model.Transcript...)
	next.Queued = append([]QueuedEntry(nil), model.Queued...)
	next.Diagnostics = append([]diagnostic.Diagnostic(nil), model.Diagnostics...)
	next.Subagents = cloneSubagentRuns(model.Subagents)
	next.AgentToolNames = append([]string(nil), model.AgentToolNames...)
	next.PendingApproval = cloneApprovalProposal(model.PendingApproval)
	next.LastApprovalDecision = model.LastApprovalDecision

	switch msg := message.(type) {
	case PromptSubmitted:
		text := msg.Text
		if strings.TrimSpace(text) == "" {
			return next, nil
		}
		if hasActiveWork(next.Status) {
			next = appendQueuedEntry(next, QueuedEntry{Kind: "prompt", Text: text})
			return next, nil
		}

		return startFakePrompt(next, text)
	case AgentPromptSubmitted:
		text := msg.Text
		if strings.TrimSpace(text) == "" {
			return next, nil
		}
		if hasActiveWork(next.Status) {
			next = appendQueuedEntry(next, QueuedEntry{Kind: "prompt", Text: text})
			return next, nil
		}

		return startAgentPrompt(next, text, msg.Provider, msg.Model, msg.ToolNames)
	case QueuedPromptDrainRequested:
		if hasActiveWork(next.Status) || len(next.Queued) == 0 {
			return next, nil
		}
		entry := next.Queued[0]
		next.Queued = append([]QueuedEntry(nil), next.Queued[1:]...)
		if entry.Kind != "prompt" || strings.TrimSpace(entry.Text) == "" {
			return next, nil
		}
		provider := strings.TrimSpace(msg.Provider)
		if provider == "" {
			provider = next.AgentProvider
		}
		modelName := strings.TrimSpace(msg.Model)
		if modelName == "" {
			modelName = next.AgentModel
		}
		toolNames := msg.ToolNames
		if len(toolNames) == 0 {
			toolNames = next.AgentToolNames
		}
		if provider != "" || modelName != "" || len(toolNames) > 0 {
			return startAgentPrompt(next, entry.Text, provider, modelName, toolNames)
		}
		return startFakePrompt(next, entry.Text)
	case CommandSelected:
		operation := nextOperation(&next, OperationCommand, msg.Name)
		next.Status = StatusActive
		next.Result = ""
		next.LastCommand = msg.Name
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "command", Text: msg.Name})
		return next, []Effect{FakeCommandEffect{Operation: operation, Command: msg.Name}}
	case UtilityJobProposed:
		request := utility.NormalizeJobRequest(msg.Request)
		decision := utility.CanRun(utilityActivity(next), request)
		if !decision.Allowed {
			next.LastUtility = utility.BlockedResult(request, decision)
			return next, nil
		}

		operation := nextOperation(&next, OperationUtility, utilitySubjectLabel(request))
		operation.Source = "runtime.utility"
		next.ActiveUtility = request
		next.LastUtility = utility.RunningResult(request)
		return next, []Effect{UtilityJobEffect{Operation: operation, Request: request}}
	case CapabilityProposed:
		request := normalizeCapabilityRequest(msg.Request)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "capability", Text: capabilitySubjectLabel(request)})
			return next, nil
		}

		operation := nextOperation(&next, OperationCapability, capabilitySubjectLabel(request))
		operation.Source = "runtime.capability"
		next.Status = StatusActive
		next.Result = ""
		next.ActiveCapability = request
		next.LastCapability = capability.ExitPayload{}
		next.CapabilityDraft = ""
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "capability", Text: capabilitySubjectLabel(request)})
		return next, []Effect{CapabilityEffect{Operation: operation, Request: request, Execution: capability.PrepareModelExecution(request)}}
	case SubagentSpawnProposed:
		operation := nextOperation(&next, OperationSubagent, subagentSubjectLabel(msg.Request))
		operation.Source = "runtime.subagent"
		request := normalizeSubagentRequest(msg.Request, next.ActiveOperation.ID, operation.ID)
		next.Subagents = upsertSubagentRun(next.Subagents, SubagentRun{
			ID:            request.ID,
			ParentRunID:   request.ParentRunID,
			Purpose:       request.Purpose,
			Input:         request.Input,
			Tools:         append([]string(nil), request.Tools...),
			Budget:        request.Budget,
			Status:        SubagentStatusRunning,
			Summary:       "spawn requested: " + request.Purpose,
			EvidenceLinks: cloneSubagentEvidenceLinks(request.EvidenceLinks),
			Source:        request.Source,
		})
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "subagent", Text: request.Purpose})
		return next, []Effect{SpawnSubagentEffect{Operation: operation, Request: request}}
	case BackgroundCompactContextProposed:
		request := normalizeCompactRequest(msg.Request, CompactModeBackground)
		decision := utility.CanRun(utilityActivity(next), backgroundCompactGateRequest(request))
		if !decision.Allowed {
			next.LastCompact = blockedBackgroundCompactResult(request, decision)
			return next, nil
		}

		operation := nextOperation(&next, OperationCompact, "background context compaction")
		operation.Source = "runtime.utility"
		next.ActiveCompact = request
		next.LastCompact = runningCompactResult(request)
		return next, []Effect{CompactContextEffect{Operation: operation, Request: request}}
	case CompactContextProposed:
		request := normalizeCompactRequest(msg.Request, CompactModeManual)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "compact", Text: "compact context"})
			return next, nil
		}

		operation := nextOperation(&next, OperationCompact, "manual context compaction")
		next.Status = StatusActive
		next.Result = ""
		next.LastCommand = "compact"
		next.ActiveCompact = request
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "command", Text: "compact"})
		return next, []Effect{CompactContextEffect{Operation: operation, Request: request}}
	case ReadToolProposed:
		request := msg.Request
		request.Path = strings.TrimSpace(request.Path)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "read", Text: readPathLabel(request.Path)})
			return next, nil
		}

		operation := nextOperation(&next, OperationRead, readPathLabel(request.Path))
		next.Status = StatusActive
		next.Result = ""
		next.ActiveRead = request
		next.LastRead = ReadToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: "read " + readPathLabel(request.Path)})
		return next, []Effect{ReadToolEffect{Operation: operation, Request: request}}
	case SearchToolProposed:
		request := trimSearchRequest(msg.Request)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: string(request.ToolName), Text: searchSubjectLabel(request)})
			return next, nil
		}

		operation := nextOperation(&next, operationKindForSearch(request.ToolName), searchSubjectLabel(request))
		next.Status = StatusActive
		next.Result = ""
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = request
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: string(request.ToolName) + " " + searchSubjectLabel(request)})
		return next, []Effect{SearchToolEffect{Operation: operation, Request: request}}
	case BashToolProposed:
		request := trimBashRequest(msg.Request)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "bash", Text: bashSubjectLabel(request)})
			return next, nil
		}

		operation := nextOperation(&next, OperationBash, bashSubjectLabel(request))
		next.Status = StatusActive
		next.Result = ""
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = request
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: "bash " + bashSubjectLabel(request)})
		return next, []Effect{BashToolEffect{Operation: operation, Request: request}}
	case FetchToolProposed:
		request := trimFetchRequest(msg.Request)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "fetch", Text: fetchSubjectLabel(request)})
			return next, nil
		}

		operation := nextOperation(&next, OperationFetch, fetchSubjectLabel(request))
		next.Status = StatusActive
		next.Result = ""
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = request
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: "fetch " + fetchSubjectLabel(request)})
		return next, []Effect{FetchToolEffect{Operation: operation, Request: request}}
	case EditToolProposed:
		request := trimMutationRequest(MutationToolEdit, msg.Request)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "edit", Text: mutationSubjectLabel(request)})
			return next, nil
		}

		operation := nextOperation(&next, OperationEdit, mutationSubjectLabel(request))
		next.Status = StatusActive
		next.Result = ""
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = request
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: "edit " + mutationSubjectLabel(request)})
		return next, []Effect{EditToolEffect{Operation: operation, Request: request}}
	case WriteToolProposed:
		request := trimMutationRequest(MutationToolWrite, msg.Request)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "write", Text: mutationSubjectLabel(request)})
			return next, nil
		}

		operation := nextOperation(&next, OperationWrite, mutationSubjectLabel(request))
		next.Status = StatusActive
		next.Result = ""
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = request
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: "write " + mutationSubjectLabel(request)})
		return next, []Effect{WriteToolEffect{Operation: operation, Request: request}}
	case ApprovalProposed:
		proposal := normalizeApprovalProposal(msg.Proposal)
		operation := nextOperation(&next, OperationApproval, approvalSubjectLabel(proposal))
		if proposal.ID == "" {
			proposal.ID = operation.ID
		}
		next.Status = StatusApprovalPending
		next.Result = "approval pending: " + approvalSubjectLabel(proposal)
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.PendingApproval = proposal
		next.LastApprovalDecision = ApprovalDecision{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "approval_pending", Text: next.Result})
		return next, nil
	case ApprovalDecisionSelected:
		decision := ApprovalDecision{ProposalID: strings.TrimSpace(msg.ProposalID), Action: normalizeApprovalAction(msg.Action), Reason: strings.TrimSpace(msg.Reason)}
		if decision.ProposalID == "" {
			decision.ProposalID = next.PendingApproval.ID
		}
		if next.PendingApproval.ID == "" || decision.ProposalID != next.PendingApproval.ID {
			decision.Stale = true
			next.LastApprovalDecision = decision
			next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "approval_stale", Text: "approval decision ignored: stale proposal"})
			return next, nil
		}
		result := "approval " + string(decision.Action) + ": " + approvalSubjectLabel(next.PendingApproval)
		next.Status = StatusIdle
		next.Result = result
		next.LastApprovalDecision = decision
		next.PendingApproval = ApprovalProposal{}
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "approval_" + string(decision.Action), Text: result})
		return next, nil
	case InterruptRequested:
		if !hasActiveFakeWork(next) {
			return next, nil
		}

		cancel := CancelMetadata{Requested: true, Reason: strings.TrimSpace(msg.Reason)}
		operation := next.ActiveOperation
		operation.Cancel = cancel
		next.Status = StatusCanceling
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "interrupting", Text: cancel.Reason})
		return next, []Effect{FakeInterruptEffect{Operation: operation, Cancel: cancel}}
	case AgentAssistantDelta:
		next.Status = StatusActive
		if next.ActiveOperation.ID == "" {
			next.ActiveOperation = msg.Operation
		}
		if next.AssistantDraft != "" && msg.Text != "" && !strings.HasSuffix(next.AssistantDraft, " ") && !strings.HasPrefix(msg.Text, " ") {
			next.AssistantDraft += " "
		}
		next.AssistantDraft += msg.Text
		next.AgentProvider = msg.Provider
		next.AgentModel = msg.Model
		next.Result = next.AssistantDraft
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "assistant_delta", Text: msg.Text})
		return next, nil
	case CapabilityOutputDelta:
		if next.ActiveOperation.ID != "" && msg.Operation.ID != "" && next.ActiveOperation.ID != msg.Operation.ID {
			return next, nil
		}
		next.Status = StatusActive
		if next.ActiveOperation.ID == "" {
			next.ActiveOperation = msg.Operation
		}
		if next.ActiveCapability.Capability == "" && msg.Capability != "" {
			next.ActiveCapability.Capability = msg.Capability
		}
		if next.CapabilityDraft != "" && msg.Text != "" && !strings.HasSuffix(next.CapabilityDraft, " ") && !strings.HasPrefix(msg.Text, " ") {
			next.CapabilityDraft += " "
		}
		next.CapabilityDraft += msg.Text
		next.Result = next.CapabilityDraft
		return next, nil
	case AgentToolRequested:
		next.Status = StatusActive
		if next.ActiveOperation.ID == "" {
			next.ActiveOperation = msg.Operation
		}
		next.LastAgentToolRequest = msg.Request
		next.AgentProvider = msg.Request.Provider
		next.AgentModel = msg.Request.Model
		next.Result = "tool request: " + msg.Request.Name
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool_request", Text: agentToolRequestSummary(msg.Request)})
		return next, nil
	case AgentTurnCompleted:
		next.Status = StatusIdle
		next.AgentProvider = msg.Provider
		next.AgentModel = msg.Model
		next.AgentFinishReason = msg.FinishReason
		next.LastAgentPause = AgentPauseMetadata{}
		next.LastAgentFailure = FailureMetadata{}
		result := strings.TrimSpace(next.AssistantDraft)
		if result == "" {
			result = "agent turn completed"
		}
		next.Result = result
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "result", Text: result})
		return next, nil
	case AgentTurnPaused:
		pause := normalizeAgentPause(msg.Pause)
		next.Status = StatusPaused
		next.AgentProvider = msg.Provider
		next.AgentModel = msg.Model
		next.AgentFinishReason = pause.Reason
		next.LastAgentPause = pause
		next.LastAgentFailure = FailureMetadata{}
		result := strings.TrimSpace(next.AssistantDraft)
		if result != "" {
			result += "\n\n" + pause.Message
		} else {
			result = pause.Message
		}
		next.Result = result
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "paused", Text: result})
		return next, nil
	case AgentTurnFailed:
		next.Status = StatusIdle
		next.Result = msg.Failure.Message
		next.AgentProvider = msg.Provider
		next.AgentModel = msg.Model
		next.LastAgentPause = AgentPauseMetadata{}
		next.LastAgentFailure = msg.Failure
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "failure", Text: msg.Failure.Message})
		return next, nil
	case FakeEffectCompleted:
		next.Status = StatusIdle
		next.Result = msg.Result
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "result", Text: msg.Result})
		return next, nil
	case FakeEffectFailed:
		next.Status = StatusIdle
		next.Result = msg.Failure.Message
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "failure", Text: msg.Failure.Message})
		return next, nil
	case ReadToolCompleted:
		summary := readResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = msg.Result
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != ReadToolErrorNone {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case SearchToolCompleted:
		summary := searchResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = msg.Result
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != SearchToolErrorNone {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case BashToolCompleted:
		summary := bashResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = msg.Result
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != BashToolErrorNone {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case UtilityJobCompleted:
		next.ActiveUtility = utility.JobRequest{}
		next.LastUtility = msg.Result
		return next, nil
	case SubagentProgressed:
		status := normalizeSubagentStatus(msg.Status, SubagentStatusRunning)
		next.Subagents = updateSubagentRun(next.Subagents, subagentProgressRun(msg, status))
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "subagent_" + string(status), Text: subagentLifecycleSummary(msg.Purpose, msg.Summary)})
		return next, nil
	case SubagentCompleted:
		run := subagentCompletedRun(msg)
		next.Subagents = updateSubagentRun(next.Subagents, run)
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "subagent_completed", Text: subagentLifecycleSummary(run.Purpose, run.Summary)})
		return next, nil
	case SubagentFailed:
		run := subagentFailedRun(msg)
		next.Subagents = updateSubagentRun(next.Subagents, run)
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "subagent_failed", Text: subagentLifecycleSummary(run.Purpose, run.Summary)})
		return next, nil
	case SubagentCanceled:
		run := subagentCanceledRun(msg)
		next.Subagents = updateSubagentRun(next.Subagents, run)
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "subagent_canceled", Text: subagentLifecycleSummary(run.Purpose, run.Summary)})
		return next, nil
	case CapabilityCompleted:
		summary := capabilityResultSummary(msg.Payload)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveCapability = capability.Request{}
		next.LastCapability = msg.Payload

		nextPhase := next.CurrentPhase
		isCrossCutting := false
		if capDef, ok := lookupCapabilityDefinition(string(msg.Payload.Capability)); ok && capDef.CrossCutting {
			isCrossCutting = true
		}

		if !isCrossCutting {
			switch msg.Payload.Signal {
			case capability.ExitWaiting:
				// Retain CurrentPhase (do not transition).
			case capability.ExitStuck:
				// Transition to RecommendedSuccessor if valid, otherwise PhaseIdle.
				nextPhase = workflow.PhaseIdle
				if msg.Payload.RecommendedSuccessor != "" {
					if isValidTransition(next.CurrentPhase, msg.Payload.RecommendedSuccessor) {
						nextPhase = msg.Payload.RecommendedSuccessor
					}
				}
			default: // capability.ExitComplete, capability.ExitFlagged
				resolved := false
				if msg.Payload.RecommendedSuccessor != "" {
					if isValidTransition(next.CurrentPhase, msg.Payload.RecommendedSuccessor) {
						nextPhase = msg.Payload.RecommendedSuccessor
						resolved = true
					}
				}
				if !resolved {
					if next.CurrentPhase == workflow.PhaseBuild || next.CurrentPhase == workflow.PhaseAudit {
						nextPhase = workflow.PhaseIdle
					} else if next.CurrentPhase != workflow.PhaseIdle {
						successors, _ := workflow.ProtocolSuccessors(next.CurrentPhase)
						if len(successors) > 0 {
							nextPhase = successors[0]
						}
					}
				}
			}
		}
		next.CurrentPhase = nextPhase

		next.CapabilityDraft = ""
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Payload.Signal == capability.ExitStuck {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case CompactContextCompleted:
		if normalizeCompactMode(msg.Result.Source.Mode) == CompactModeBackground {
			next.ActiveCompact = CompactContextRequest{}
			next.LastCompact = msg.Result
			return next, nil
		}
		summary := compactResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = msg.Result
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != CompactContextErrorNone {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case FetchToolCompleted:
		summary := fetchResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = msg.Result
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != FetchToolErrorNone {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case MutationToolCompleted:
		summary := mutationResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
		next.ActiveCompact = CompactContextRequest{}
		next.LastCompact = CompactContextResult{}
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = msg.Result
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != MutationToolErrorNone {
			kind = "failure"
		}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: kind, Text: summary})
		return next, nil
	case FakeInterruptResolved:
		next.Status = StatusCanceled
		next.Result = "fake work canceled"
		operation := msg.Operation
		operation.Cancel = msg.Cancel
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "canceled", Text: next.Result})
		return next, nil
	case RuntimeDiagnostic:
		next.Diagnostics = append(next.Diagnostics, msg.Diagnostic)
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "diagnostic", Text: msg.Diagnostic.BoundedMessage})
		return next, nil
	default:
		return next, nil
	}
}

func normalizeSubagentRequest(request SubagentRequest, activeParentID string, fallbackID string) SubagentRequest {
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		request.ID = fallbackID
	}
	request.ParentRunID = strings.TrimSpace(request.ParentRunID)
	if request.ParentRunID == "" {
		request.ParentRunID = strings.TrimSpace(activeParentID)
	}
	if request.ParentRunID == "" {
		request.ParentRunID = request.ID
	}
	request.Purpose = strings.TrimSpace(request.Purpose)
	if request.Purpose == "" {
		request.Purpose = "supervised subagent"
	}
	request.Input = strings.TrimSpace(request.Input)
	request.Tools = trimStringSlice(request.Tools)
	request.EvidenceLinks = cloneSubagentEvidenceLinks(request.EvidenceLinks)
	request.Source.Caller = strings.TrimSpace(request.Source.Caller)
	if request.Source.Caller == "" {
		request.Source.Caller = "runtime.subagent"
	}
	request.Source.RequestID = strings.TrimSpace(request.Source.RequestID)
	request.Source.Description = strings.TrimSpace(request.Source.Description)
	return request
}

func subagentSubjectLabel(request SubagentRequest) string {
	purpose := strings.TrimSpace(request.Purpose)
	if purpose != "" {
		return purpose
	}
	if request.ID != "" {
		return request.ID
	}
	return "supervised subagent"
}

func normalizeSubagentStatus(status SubagentStatus, fallback SubagentStatus) SubagentStatus {
	switch status {
	case SubagentStatusQueued, SubagentStatusRunning, SubagentStatusCompleted, SubagentStatusFailed, SubagentStatusCanceled:
		return status
	}
	return fallback
}

func subagentProgressRun(msg SubagentProgressed, status SubagentStatus) SubagentRun {
	return SubagentRun{
		ID:            strings.TrimSpace(msg.ID),
		ParentRunID:   strings.TrimSpace(msg.ParentRunID),
		Purpose:       strings.TrimSpace(msg.Purpose),
		Status:        status,
		Summary:       strings.TrimSpace(msg.Summary),
		EvidenceLinks: cloneSubagentEvidenceLinks(msg.EvidenceLinks),
	}
}

func subagentCompletedRun(msg SubagentCompleted) SubagentRun {
	result := msg.Result
	parentRunID := strings.TrimSpace(result.ParentRunID)
	if parentRunID == "" {
		parentRunID = strings.TrimSpace(msg.ParentRunID)
	}
	return SubagentRun{
		ID:            strings.TrimSpace(result.ID),
		ParentRunID:   parentRunID,
		Purpose:       strings.TrimSpace(result.Purpose),
		Status:        SubagentStatusCompleted,
		Summary:       strings.TrimSpace(result.Summary),
		EvidenceLinks: cloneSubagentEvidenceLinks(result.EvidenceLinks),
	}
}

func subagentFailedRun(msg SubagentFailed) SubagentRun {
	summary := strings.TrimSpace(msg.Summary)
	if summary == "" {
		summary = strings.TrimSpace(msg.Failure.Message)
	}
	return SubagentRun{
		ID:            strings.TrimSpace(msg.ID),
		ParentRunID:   strings.TrimSpace(msg.ParentRunID),
		Purpose:       strings.TrimSpace(msg.Purpose),
		Status:        SubagentStatusFailed,
		Summary:       summary,
		EvidenceLinks: cloneSubagentEvidenceLinks(msg.EvidenceLinks),
		Failure:       msg.Failure,
	}
}

func subagentCanceledRun(msg SubagentCanceled) SubagentRun {
	summary := strings.TrimSpace(msg.Summary)
	if summary == "" {
		summary = strings.TrimSpace(msg.Cancel.Reason)
	}
	return SubagentRun{
		ID:            strings.TrimSpace(msg.ID),
		ParentRunID:   strings.TrimSpace(msg.ParentRunID),
		Purpose:       strings.TrimSpace(msg.Purpose),
		Status:        SubagentStatusCanceled,
		Summary:       summary,
		EvidenceLinks: cloneSubagentEvidenceLinks(msg.EvidenceLinks),
		Cancel:        msg.Cancel,
	}
}

func upsertSubagentRun(runs []SubagentRun, run SubagentRun) []SubagentRun {
	return mergeSubagentRun(runs, run, false)
}

func updateSubagentRun(runs []SubagentRun, update SubagentRun) []SubagentRun {
	return mergeSubagentRun(runs, update, true)
}

func mergeSubagentRun(runs []SubagentRun, update SubagentRun, preserveExisting bool) []SubagentRun {
	update.ID = strings.TrimSpace(update.ID)
	update.ParentRunID = strings.TrimSpace(update.ParentRunID)
	update.Purpose = strings.TrimSpace(update.Purpose)
	if update.Status == "" {
		update.Status = SubagentStatusRunning
	}
	if update.ID == "" && update.ParentRunID == "" && update.Purpose == "" {
		update.ID = "subagent"
	}
	next := cloneSubagentRuns(runs)
	for index := range next {
		if !sameSubagentRun(next[index], update) {
			continue
		}
		if preserveExisting {
			next[index] = mergeExistingSubagentRun(next[index], update)
		} else {
			next[index] = cloneSubagentRun(update)
		}
		return next
	}
	return append(next, cloneSubagentRun(update))
}

func sameSubagentRun(existing SubagentRun, update SubagentRun) bool {
	if update.ID != "" && existing.ID == update.ID {
		return true
	}
	return update.ID == "" && update.ParentRunID != "" && update.Purpose != "" && existing.ParentRunID == update.ParentRunID && existing.Purpose == update.Purpose
}

func mergeExistingSubagentRun(existing SubagentRun, update SubagentRun) SubagentRun {
	merged := cloneSubagentRun(existing)
	if update.ID != "" {
		merged.ID = update.ID
	}
	if update.ParentRunID != "" {
		merged.ParentRunID = update.ParentRunID
	}
	if update.Purpose != "" {
		merged.Purpose = update.Purpose
	}
	if update.Input != "" {
		merged.Input = update.Input
	}
	if len(update.Tools) > 0 {
		merged.Tools = append([]string(nil), update.Tools...)
	}
	if update.Budget != (SubagentBudget{}) {
		merged.Budget = update.Budget
	}
	if update.Status != "" {
		merged.Status = update.Status
	}
	if update.Summary != "" {
		merged.Summary = update.Summary
	}
	if len(update.EvidenceLinks) > 0 {
		merged.EvidenceLinks = cloneSubagentEvidenceLinks(update.EvidenceLinks)
	}
	if update.Failure != (FailureMetadata{}) {
		merged.Failure = update.Failure
	}
	if update.Cancel != (CancelMetadata{}) {
		merged.Cancel = update.Cancel
	}
	if update.Source != (SubagentSourceMetadata{}) {
		merged.Source = update.Source
	}
	return merged
}

func cloneSubagentRuns(runs []SubagentRun) []SubagentRun {
	if len(runs) == 0 {
		return nil
	}
	clones := make([]SubagentRun, 0, len(runs))
	for _, run := range runs {
		clones = append(clones, cloneSubagentRun(run))
	}
	return clones
}

func cloneSubagentRun(run SubagentRun) SubagentRun {
	run.Tools = append([]string(nil), run.Tools...)
	run.EvidenceLinks = cloneSubagentEvidenceLinks(run.EvidenceLinks)
	return run
}

func cloneSubagentEvidenceLinks(links []SubagentEvidenceLink) []SubagentEvidenceLink {
	if len(links) == 0 {
		return nil
	}
	return append([]SubagentEvidenceLink(nil), links...)
}

func trimStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func subagentLifecycleSummary(purpose string, summary string) string {
	purpose = strings.TrimSpace(purpose)
	summary = strings.TrimSpace(summary)
	if purpose == "" {
		purpose = "subagent"
	}
	if summary == "" {
		return purpose
	}
	return purpose + ": " + summary
}

func hasActiveWork(status Status) bool {
	return status == StatusActive || status == StatusApprovalPending || status == StatusCanceling
}

func normalizeAgentPause(pause AgentPauseMetadata) AgentPauseMetadata {
	pause.Reason = strings.TrimSpace(pause.Reason)
	if pause.Reason == "" {
		pause.Reason = "step_limit"
	}
	pause.Message = strings.TrimSpace(pause.Message)
	if pause.Message == "" {
		pause.Message = "Agent paused at the step budget. Send a continuation prompt to continue."
	}
	pause.Suggestion = strings.TrimSpace(pause.Suggestion)
	if pause.Suggestion == "" {
		pause.Suggestion = "continue"
	}
	pause.Resumable = true
	return pause
}

func hasActiveFakeWork(model Model) bool {
	return (model.Status == StatusActive || model.Status == StatusCanceling) && !isToolOperation(model.ActiveOperation.Kind)
}

func isToolOperation(kind OperationKind) bool {
	return kind == OperationCompact || kind == OperationRead || kind == OperationFind || kind == OperationGrep || kind == OperationBash || kind == OperationFetch || kind == OperationEdit || kind == OperationWrite
}

func utilityActivity(model Model) utility.Activity {
	return utility.Activity{
		PrimaryStatus:       string(model.Status),
		ActiveOperationKind: string(model.ActiveOperation.Kind),
		ApprovalPending:     model.PendingApproval.ID != "",
		QueuedCount:         len(model.Queued),
	}
}

func utilitySubjectLabel(request utility.JobRequest) string {
	request = utility.NormalizeJobRequest(request)
	return string(request.Kind) + " " + request.ID
}

func backgroundCompactGateRequest(request CompactContextRequest) utility.JobRequest {
	source := normalizeCompactSource(request.Source, CompactModeBackground)
	return utility.NormalizeJobRequest(utility.JobRequest{
		ID:    source.RequestID,
		Kind:  utility.JobSuggestion,
		Model: "utility",
		Source: utility.Source{
			Caller:      source.Caller,
			RequestID:   source.RequestID,
			Description: source.Description,
		},
	})
}

func normalizeCompactRequest(request CompactContextRequest, fallback CompactMode) CompactContextRequest {
	request.Source = normalizeCompactSource(request.Source, fallback)
	return request
}

func normalizeCompactSource(source CompactSourceMetadata, fallback CompactMode) CompactSourceMetadata {
	source.Caller = strings.TrimSpace(source.Caller)
	source.RequestID = strings.TrimSpace(source.RequestID)
	source.Description = strings.TrimSpace(source.Description)
	source.Mode = normalizeCompactMode(source.Mode)
	if source.Mode == "" {
		source.Mode = normalizeCompactMode(fallback)
	}
	if source.Mode == "" {
		source.Mode = CompactModeManual
	}
	if source.Caller == "" {
		if source.Mode == CompactModeBackground {
			source.Caller = "app.compact.background"
		} else {
			source.Caller = "app.compact"
		}
	}
	if source.RequestID == "" {
		if source.Mode == CompactModeBackground {
			source.RequestID = "background-compact"
		} else {
			source.RequestID = "manual-compact"
		}
	}
	if source.Description == "" {
		if source.Mode == CompactModeBackground {
			source.Description = "idle-only background context compaction"
		} else {
			source.Description = "manual /compact command"
		}
	}
	return source
}

func normalizeCompactMode(mode CompactMode) CompactMode {
	switch CompactMode(strings.TrimSpace(string(mode))) {
	case CompactModeManual:
		return CompactModeManual
	case CompactModeBackground:
		return CompactModeBackground
	default:
		return ""
	}
}

func compactModeLabel(mode CompactMode) string {
	mode = normalizeCompactMode(mode)
	if mode == "" {
		mode = CompactModeManual
	}
	return string(mode)
}

func normalizeCapabilityRequest(request capability.Request) capability.Request {
	if request.Capability == "" {
		request.Capability = capability.NameBrief
	}
	return request
}

func capabilitySubjectLabel(request capability.Request) string {
	request = normalizeCapabilityRequest(request)
	name := strings.TrimSpace(string(request.Capability))
	if name == "" {
		return "brief"
	}
	return name
}

func capabilityResultSummary(payload capability.ExitPayload) string {
	if strings.TrimSpace(payload.Summary) != "" {
		return payload.Summary
	}
	name := strings.TrimSpace(string(payload.Capability))
	if name == "" {
		name = "capability"
	}
	if payload.Blocker != "" {
		return name + " stuck: " + payload.Blocker
	}
	if payload.Signal != "" {
		return name + " " + string(payload.Signal)
	}
	return name + " completed"
}

const maxQueuedEntries = 8

func appendQueuedEntry(model Model, entry QueuedEntry) Model {
	entry.Kind = strings.TrimSpace(entry.Kind)
	entry.Text = strings.TrimSpace(entry.Text)
	if entry.Kind == "" || entry.Text == "" {
		return model
	}
	if len(model.Queued) >= maxQueuedEntries {
		model.Diagnostics = append(model.Diagnostics, diagnostic.New(diagnostic.Spec{
			Category:         diagnostic.CategoryRuntime,
			Source:           diagnostic.SourceRuntime,
			Severity:         diagnostic.SeverityWarning,
			Message:          fmt.Sprintf("queued input limit reached (%d); newest %s request was not queued", maxQueuedEntries, entry.Kind),
			AffectedArtifact: diagnostic.ArtifactRuntimeEffect,
			RecoveryAction:   diagnostic.RecoveryManualRepair,
			UserInputNeeded:  true,
		}))
		return model
	}
	model.Queued = append(model.Queued, entry)
	return model
}

func startFakePrompt(model Model, text string) (Model, []Effect) {
	operation := nextOperation(&model, OperationPrompt, text)
	model.Status = StatusActive
	model.Result = ""
	model.ActiveCompact = CompactContextRequest{}
	model.ActiveRead = ReadToolRequest{}
	model.LastRead = ReadToolResult{}
	model.ActiveFetch = FetchToolRequest{}
	model.LastFetch = FetchToolResult{}
	model.ActiveMutation = MutationToolRequest{}
	model.LastMutation = MutationToolResult{}
	model.ActiveOperation = operation
	model.AssistantDraft = ""
	model.AgentProvider = ""
	model.AgentModel = ""
	model.AgentToolNames = nil
	model.LastAgentToolRequest = AgentToolRequest{}
	model.AgentFinishReason = ""
	model.LastAgentPause = AgentPauseMetadata{}
	model.LastAgentFailure = FailureMetadata{}
	model.Transcript = append(model.Transcript, TranscriptEntry{Kind: "prompt", Text: text})
	return model, []Effect{FakePromptEffect{Operation: operation, Prompt: text}}
}

func startAgentPrompt(model Model, text string, provider string, modelName string, toolNames []string) (Model, []Effect) {
	operation := nextOperation(&model, OperationPrompt, text)
	operation.Source = "runtime.agent"
	toolNames = filterToolsForPhase(model.CurrentPhase, toolNames)
	model.Status = StatusActive
	model.Result = ""
	model.ActiveCompact = CompactContextRequest{}
	model.ActiveRead = ReadToolRequest{}
	model.LastRead = ReadToolResult{}
	model.ActiveFetch = FetchToolRequest{}
	model.LastFetch = FetchToolResult{}
	model.ActiveMutation = MutationToolRequest{}
	model.LastMutation = MutationToolResult{}
	model.ActiveOperation = operation
	model.AssistantDraft = ""
	model.AgentProvider = provider
	model.AgentModel = modelName
	model.AgentToolNames = toolNames
	model.LastAgentToolRequest = AgentToolRequest{}
	model.AgentFinishReason = ""
	model.LastAgentPause = AgentPauseMetadata{}
	model.LastAgentFailure = FailureMetadata{}
	model.Transcript = append(model.Transcript, TranscriptEntry{Kind: "prompt", Text: text})
	return model, []Effect{AgentPromptEffect{Operation: operation, Prompt: text, Provider: provider, Model: modelName, ToolNames: toolNames}}
}

func nextOperation(model *Model, kind OperationKind, subject string) OperationMetadata {
	model.NextOperation++
	return OperationMetadata{
		ID:      operationID(model.NextOperation),
		Kind:    kind,
		Subject: subject,
		Source:  "user",
	}
}

func operationID(number int) string {
	const prefix = "op-"
	if number <= 0 {
		return prefix + "0"
	}

	digits := make([]byte, 0, 8)
	for number > 0 {
		digits = append(digits, byte('0'+number%10))
		number /= 10
	}

	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}

	return prefix + string(digits)
}

func cloneApprovalProposal(proposal ApprovalProposal) ApprovalProposal {
	clone := proposal
	clone.Preview = append([]string(nil), proposal.Preview...)
	clone.Command = append([]string(nil), proposal.Command...)
	clone.DiffPreview = append([]string(nil), proposal.DiffPreview...)
	return clone
}

func normalizeApprovalProposal(proposal ApprovalProposal) ApprovalProposal {
	proposal = cloneApprovalProposal(proposal)
	proposal.ID = strings.TrimSpace(proposal.ID)
	proposal.OperationKind = strings.TrimSpace(proposal.OperationKind)
	proposal.Target = strings.TrimSpace(proposal.Target)
	proposal.RiskSummary = strings.TrimSpace(proposal.RiskSummary)
	proposal.Path = strings.TrimSpace(proposal.Path)
	proposal.WorkingDir = strings.TrimSpace(proposal.WorkingDir)
	proposal.ExpectedEffect = strings.TrimSpace(proposal.ExpectedEffect)
	proposal.RunID = strings.TrimSpace(proposal.RunID)
	proposal.Capability = strings.TrimSpace(proposal.Capability)
	proposal.DefaultAction = normalizeApprovalAction(proposal.DefaultAction)
	if proposal.DefaultAction == "" {
		proposal.DefaultAction = ApprovalActionDeny
	}
	for index, line := range proposal.Preview {
		proposal.Preview[index] = strings.TrimRight(line, "\r\n")
	}
	for index, line := range proposal.DiffPreview {
		proposal.DiffPreview[index] = strings.TrimRight(line, "\r\n")
	}
	trimmed := make([]string, 0, len(proposal.Command))
	for _, arg := range proposal.Command {
		trimmed = append(trimmed, strings.TrimSpace(arg))
	}
	proposal.Command = trimmed
	return proposal
}

func normalizeApprovalAction(action ApprovalAction) ApprovalAction {
	switch ApprovalAction(strings.TrimSpace(string(action))) {
	case ApprovalActionApprove:
		return ApprovalActionApprove
	case ApprovalActionDeny:
		return ApprovalActionDeny
	case ApprovalActionDefer:
		return ApprovalActionDefer
	default:
		return ApprovalActionDefer
	}
}

func approvalSubjectLabel(proposal ApprovalProposal) string {
	for _, value := range []string{proposal.Target, proposal.Path, strings.Join(proposal.Command, " "), proposal.OperationKind} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "risky operation"
}

func agentToolRequestSummary(request AgentToolRequest) string {
	if request.ID == "" {
		return "tool request " + request.Name
	}
	return "tool request " + request.Name + " " + request.ID
}

func runningCompactResult(request CompactContextRequest) CompactContextResult {
	request = normalizeCompactRequest(request, CompactModeBackground)
	return CompactContextResult{
		Status:         "running",
		Summary:        compactModeLabel(request.Source.Mode) + " context compaction running",
		Blocks:         append([]CompactContextBlock(nil), request.Blocks...),
		SourceRefs:     append([]CompactSourceRef(nil), request.SourceRefs...),
		Claims:         append([]CompactContextClaim(nil), request.Claims...),
		OriginalBudget: request.Budget,
		Budget:         request.Budget,
		Source:         request.Source,
	}
}

func blockedBackgroundCompactResult(request CompactContextRequest, decision utility.Decision) CompactContextResult {
	request = normalizeCompactRequest(request, CompactModeBackground)
	detail := strings.TrimSpace(decision.Detail)
	if detail == "" {
		detail = "primary runtime cannot yield"
	}
	reason := strings.TrimSpace(string(decision.Reason))
	if reason == "" || reason == string(utility.DenialNone) {
		reason = string(utility.DenialPrimaryBusy)
	}
	caveat := reason + ": " + detail
	return CompactContextResult{
		Status:         "blocked",
		Summary:        "background compaction blocked: " + caveat,
		Blocks:         append([]CompactContextBlock(nil), request.Blocks...),
		SourceRefs:     append([]CompactSourceRef(nil), request.SourceRefs...),
		Claims:         append([]CompactContextClaim(nil), request.Claims...),
		OriginalBudget: request.Budget,
		Budget:         request.Budget,
		Caveats:        []string{caveat},
		Source:         request.Source,
	}
}

func fakeCompactContextResult(request CompactContextRequest) CompactContextResult {
	request = normalizeCompactRequest(request, CompactModeManual)
	status := "completed"
	if len(request.Blocks) == 0 && len(request.Claims) == 0 {
		status = "flagged"
	}
	mode := compactModeLabel(request.Source.Mode)
	result := CompactContextResult{
		Status:         status,
		Summary:        mode + " compaction completed",
		Blocks:         append([]CompactContextBlock(nil), request.Blocks...),
		SourceRefs:     append([]CompactSourceRef(nil), request.SourceRefs...),
		Claims:         append([]CompactContextClaim(nil), request.Claims...),
		OriginalBudget: request.Budget,
		Budget:         request.Budget,
		Source:         request.Source,
	}
	if status == "flagged" {
		result.Summary = mode + " compaction completed with caveats"
		result.Caveats = []string{"no context blocks were available to compact"}
	}
	return result
}

func compactResultSummary(result CompactContextResult) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = fmt.Sprintf("%s compaction preserved %d source refs", compactModeLabel(result.Source.Mode), len(result.SourceRefs))
	}
	if result.Error.Kind != "" && result.Error.Kind != CompactContextErrorNone {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			return fmt.Sprintf("compact failed: %s", result.Error.Kind)
		}
		return fmt.Sprintf("compact failed: %s: %s", result.Error.Kind, message)
	}
	if len(result.Caveats) > 0 && !strings.Contains(summary, "caveat") {
		return summary + " with caveats"
	}
	return summary
}

func readResultSummary(result ReadToolResult) string {
	path := result.WorkspaceRelativePath
	if path == "" {
		path = readPathLabel(result.RequestedPath)
	}
	if path == "" {
		path = "requested path"
	}

	if result.Error.Kind != "" && result.Error.Kind != ReadToolErrorNone {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			return fmt.Sprintf("read %s failed: %s", path, result.Error.Kind)
		}
		return fmt.Sprintf("read %s failed: %s: %s", path, result.Error.Kind, message)
	}

	if result.EffectiveRange.EndLine > 0 {
		return fmt.Sprintf("read %s:%d-%d\n%s", path, result.EffectiveRange.StartLine, result.EffectiveRange.EndLine, strings.TrimRight(result.PreviewText, "\n"))
	}
	return "read " + path + ": no matching lines"
}

func trimSearchRequest(request SearchToolRequest) SearchToolRequest {
	request.Pattern = strings.TrimSpace(request.Pattern)
	request.Query = strings.TrimSpace(request.Query)
	request.IncludePattern = strings.TrimSpace(request.IncludePattern)
	return request
}

func operationKindForSearch(tool SearchToolName) OperationKind {
	if tool == SearchToolGrep {
		return OperationGrep
	}
	return OperationFind
}

func searchSubjectLabel(request SearchToolRequest) string {
	subject := request.Pattern
	if request.ToolName == SearchToolGrep {
		subject = request.Query
		if request.IncludePattern != "" {
			subject += " in " + request.IncludePattern
		}
	}
	return readPathLabel(subject)
}

func searchResultSummary(result SearchToolResult) string {
	subject := result.Pattern
	if result.ToolName == string(SearchToolGrep) {
		subject = result.Query
		if result.IncludePattern != "" {
			subject += " in " + result.IncludePattern
		}
	}
	subject = readPathLabel(subject)
	if subject == "" {
		subject = "request"
	}
	if result.Error.Kind != "" && result.Error.Kind != SearchToolErrorNone {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			return fmt.Sprintf("%s %s failed: %s", result.ToolName, subject, result.Error.Kind)
		}
		return fmt.Sprintf("%s %s failed: %s: %s", result.ToolName, subject, result.Error.Kind, message)
	}
	if len(result.Matches) == 0 {
		return fmt.Sprintf("%s %s: no matches", result.ToolName, subject)
	}
	parts := make([]string, 0, len(result.Matches)+1)
	parts = append(parts, fmt.Sprintf("%s %s: %d matches", result.ToolName, subject, len(result.Matches)))
	for _, match := range result.Matches {
		if match.LineNumber > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d: %s", match.Path, match.LineNumber, match.PreviewText))
		} else {
			parts = append(parts, match.Path)
		}
	}
	if result.Truncation.OmittedResults > 0 {
		parts = append(parts, fmt.Sprintf("omitted results: %d", result.Truncation.OmittedResults))
	}
	return strings.Join(parts, "\n")
}

func trimBashRequest(request BashToolRequest) BashToolRequest {
	request.WorkingDir = strings.TrimSpace(request.WorkingDir)
	trimmed := make([]string, 0, len(request.Argv))
	for _, arg := range request.Argv {
		trimmed = append(trimmed, strings.TrimSpace(arg))
	}
	request.Argv = trimmed
	return request
}

func bashSubjectLabel(request BashToolRequest) string {
	if len(request.Argv) == 0 {
		return "requested command"
	}
	parts := make([]string, 0, len(request.Argv))
	for _, arg := range request.Argv {
		if arg != "" {
			parts = append(parts, arg)
		}
	}
	if len(parts) == 0 {
		return "requested command"
	}
	return strings.Join(parts, " ")
}

func bashResultSummary(result BashToolResult) string {
	command := strings.Join(result.RequestedArgv, " ")
	if command == "" {
		command = "requested command"
	}
	if result.Error.Kind != "" && result.Error.Kind != BashToolErrorNone {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			return fmt.Sprintf("bash %s failed: %s", command, result.Error.Kind)
		}
		return fmt.Sprintf("bash %s failed: %s: %s", command, result.Error.Kind, message)
	}
	return fmt.Sprintf("bash %s: %s exit %d", command, result.Status, result.ExitCode)
}

func trimMutationRequest(tool MutationToolName, request MutationToolRequest) MutationToolRequest {
	request.ToolName = tool
	request.Path = strings.TrimSpace(request.Path)
	request.TargetVersion = strings.TrimSpace(request.TargetVersion)
	request.ExpectedEffect = strings.TrimSpace(request.ExpectedEffect)
	return request
}

func mutationSubjectLabel(request MutationToolRequest) string {
	return readPathLabel(request.Path)
}

func mutationResultSummary(result MutationToolResult) string {
	path := result.WorkspaceRelativePath
	if path == "" {
		path = readPathLabel(result.RequestedPath)
	}
	if path == "" {
		path = "requested path"
	}
	tool := result.ToolName
	if tool == "" {
		tool = "mutation"
	}
	if result.Error.Kind != "" && result.Error.Kind != MutationToolErrorNone {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			return fmt.Sprintf("%s %s failed: %s", tool, path, result.Error.Kind)
		}
		return fmt.Sprintf("%s %s failed: %s: %s", tool, path, result.Error.Kind, message)
	}
	return fmt.Sprintf("%s %s: %s %d bytes", tool, path, result.Status, result.BytesWritten)
}

func trimFetchRequest(request FetchToolRequest) FetchToolRequest {
	request.URL = strings.TrimSpace(request.URL)
	request.Method = strings.TrimSpace(request.Method)
	return request
}

func fetchSubjectLabel(request FetchToolRequest) string {
	return fetchURLLabel(request.URL)
}

func fetchResultSummary(result FetchToolResult) string {
	url := fetchURLLabel(result.EffectiveURL)
	if url == "requested url" {
		url = fetchURLLabel(result.RequestedURL)
	}
	if result.Error.Kind != "" && result.Error.Kind != FetchToolErrorNone {
		message := strings.TrimSpace(result.Error.Message)
		if message == "" {
			return fmt.Sprintf("fetch %s failed: %s", url, result.Error.Kind)
		}
		return fmt.Sprintf("fetch %s failed: %s: %s", url, result.Error.Kind, message)
	}
	if result.HTTPStatusCode > 0 {
		return fmt.Sprintf("fetch %s: %s %d", url, result.Status, result.HTTPStatusCode)
	}
	return fmt.Sprintf("fetch %s: %s", url, result.Status)
}

func fetchURLLabel(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.ContainsAny(rawURL, " \t\n\r|;&`$<>") || strings.Contains(rawURL, "..") {
		return "requested url"
	}
	if strings.Contains(rawURL, "@") || strings.HasPrefix(rawURL, "file:") || strings.HasPrefix(rawURL, "~") || strings.HasPrefix(rawURL, "$HOME") || strings.HasPrefix(rawURL, "${HOME}") || strings.HasPrefix(rawURL, "$XDG_") || strings.HasPrefix(rawURL, "${XDG_") {
		return "requested url"
	}
	return rawURL
}

func readPathLabel(path string) string {
	path = strings.TrimSpace(path)
	slashPath := strings.ReplaceAll(path, "\\", "/")
	if path == "" || strings.HasPrefix(slashPath, "/") || strings.Contains(slashPath, "..") {
		return "requested path"
	}
	for _, reserved := range []string{"~", "$HOME", "${HOME}", "$XDG_", "${XDG_", ".aila", ".agentera"} {
		if strings.HasPrefix(slashPath, reserved) {
			return "requested path"
		}
	}
	if strings.Contains(slashPath, ".config") {
		return "requested path"
	}
	return path
}

func filterToolsForPhase(phase workflow.Phase, toolNames []string) []string {
	if phase == "" {
		phase = workflow.PhaseIdle
	}

	// 1. Identify which of the requested tools are capabilities
	isCapability := make(map[string]bool)
	for _, capDef := range capability.Definitions() {
		isCapability[string(capDef.Name)] = true
	}

	// 2. Filter
	var filtered []string
	for _, name := range toolNames {
		// Primitive tools
		if !isCapability[name] {
			// BUILD phase allows all primitive tools.
			// IDLE allows none.
			// Other phases allow only read-only primitive tools.
			if phase == workflow.PhaseBuild {
				filtered = append(filtered, name)
			} else if phase != workflow.PhaseIdle {
				if isReadOnlyPrimitiveTool(name) {
					filtered = append(filtered, name)
				}
			}
			continue
		}

		// Capability tools
		capDef, ok := lookupCapabilityDefinition(name)
		if !ok {
			continue
		}

		// In IDLE phase, expose ONLY the brief capability as per contract.
		if phase == workflow.PhaseIdle {
			if capDef.Name == capability.NameBrief {
				filtered = append(filtered, name)
			}
			continue
		}

		// In other phases, expose if it is the primary capability for the phase or cross-cutting.
		if capDef.OwningPhase == phase || capDef.CrossCutting {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func isReadOnlyPrimitiveTool(name string) bool {
	switch name {
	case "read", "grep", "find", "search", "read_file":
		return true
	}
	return false
}

func lookupCapabilityDefinition(name string) (capability.Definition, bool) {
	for _, d := range capability.Definitions() {
		if string(d.Name) == name {
			return d, true
		}
	}
	return capability.Definition{}, false
}

func isValidTransition(from, to workflow.Phase) bool {
	if from == workflow.PhaseIdle {
		// Can transition from PhaseIdle to any valid protocol phase.
		_, err := workflow.ProtocolSuccessors(to)
		return err == nil
	}
	return workflow.ValidateProtocolSuccessor(from, to) == nil
}
