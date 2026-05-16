package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/jgabor/aila/internal/diagnostic"
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

// CommandSelected records an inert command selected through a presentation
// adapter.
type CommandSelected struct {
	Name string
}

func (CommandSelected) runtimeMessage() {}

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
	Status               Status
	Transcript           []TranscriptEntry
	Queued               []QueuedEntry
	Result               string
	LastCommand          string
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
	LastAgentToolRequest AgentToolRequest
	AgentFinishReason    string
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

// FakeCommandEffect requests fake in-memory handling for a command.
type FakeCommandEffect struct {
	Operation OperationMetadata
	Command   string
}

func (FakeCommandEffect) runtimeEffect() {}

func (effect FakeCommandEffect) Metadata() OperationMetadata {
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
	OperationPrompt   OperationKind = "prompt"
	OperationCommand  OperationKind = "command"
	OperationRead     OperationKind = "read"
	OperationFind     OperationKind = "find"
	OperationGrep     OperationKind = "grep"
	OperationBash     OperationKind = "bash"
	OperationFetch    OperationKind = "fetch"
	OperationEdit     OperationKind = "edit"
	OperationWrite    OperationKind = "write"
	OperationApproval OperationKind = "approval"
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
	next.PendingApproval = cloneApprovalProposal(model.PendingApproval)
	next.LastApprovalDecision = model.LastApprovalDecision

	switch msg := message.(type) {
	case PromptSubmitted:
		text := strings.TrimSpace(msg.Text)
		if hasActiveWork(next.Status) {
			next.Queued = append(next.Queued, QueuedEntry{Kind: "prompt", Text: text})
			return next, nil
		}

		operation := nextOperation(&next, OperationPrompt, text)
		next.Status = StatusActive
		next.Result = ""
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.AssistantDraft = ""
		next.AgentProvider = ""
		next.AgentModel = ""
		next.LastAgentToolRequest = AgentToolRequest{}
		next.AgentFinishReason = ""
		next.LastAgentFailure = FailureMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "prompt", Text: text})
		return next, []Effect{FakePromptEffect{Operation: operation, Prompt: text}}
	case CommandSelected:
		operation := nextOperation(&next, OperationCommand, msg.Name)
		next.Status = StatusActive
		next.Result = ""
		next.LastCommand = msg.Name
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveFetch = FetchToolRequest{}
		next.LastFetch = FetchToolResult{}
		next.ActiveMutation = MutationToolRequest{}
		next.LastMutation = MutationToolResult{}
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "command", Text: msg.Name})
		return next, []Effect{FakeCommandEffect{Operation: operation, Command: msg.Name}}
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
		next.AssistantDraft += msg.Text
		next.AgentProvider = msg.Provider
		next.AgentModel = msg.Model
		next.Result = next.AssistantDraft
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "assistant_delta", Text: msg.Text})
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
		next.LastAgentFailure = FailureMetadata{}
		result := strings.TrimSpace(next.AssistantDraft)
		if result == "" {
			result = "agent turn completed"
		}
		next.Result = result
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "result", Text: result})
		return next, nil
	case AgentTurnFailed:
		next.Status = StatusIdle
		next.Result = msg.Failure.Message
		next.AgentProvider = msg.Provider
		next.AgentModel = msg.Model
		next.LastAgentFailure = msg.Failure
		next.ActiveOperation = OperationMetadata{}
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "failure", Text: msg.Failure.Message})
		return next, nil
	case FakeEffectCompleted:
		next.Status = StatusIdle
		next.Result = msg.Result
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
	case FetchToolCompleted:
		summary := fetchResultSummary(msg.Result)
		next.Status = StatusIdle
		next.Result = summary
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

func hasActiveWork(status Status) bool {
	return status == StatusActive || status == StatusApprovalPending || status == StatusCanceling
}

func hasActiveFakeWork(model Model) bool {
	return (model.Status == StatusActive || model.Status == StatusCanceling) && !isToolOperation(model.ActiveOperation.Kind)
}

func isToolOperation(kind OperationKind) bool {
	return kind == OperationRead || kind == OperationFind || kind == OperationGrep || kind == OperationBash || kind == OperationFetch || kind == OperationEdit || kind == OperationWrite
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
