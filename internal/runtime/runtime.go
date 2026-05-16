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
	StatusIdle      Status = "idle"
	StatusActive    Status = "active"
	StatusCanceling Status = "canceling"
	StatusCanceled  Status = "canceled"
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

// InterruptRequested records user intent to stop the current fake operation.
type InterruptRequested struct {
	Reason string
}

func (InterruptRequested) runtimeMessage() {}

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
	Status          Status
	Transcript      []TranscriptEntry
	Queued          []QueuedEntry
	Result          string
	LastCommand     string
	ActiveRead      ReadToolRequest
	LastRead        ReadToolResult
	ActiveSearch    SearchToolRequest
	LastSearch      SearchToolResult
	ActiveBash      BashToolRequest
	LastBash        BashToolResult
	NextOperation   int
	ActiveOperation OperationMetadata
	Diagnostics     []diagnostic.Diagnostic
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
	OperationPrompt  OperationKind = "prompt"
	OperationCommand OperationKind = "command"
	OperationRead    OperationKind = "read"
	OperationFind    OperationKind = "find"
	OperationGrep    OperationKind = "grep"
	OperationBash    OperationKind = "bash"
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
}

// Update applies one runtime message and returns the next model plus typed
// effects for an external interpreter.
func Update(model Model, message Message) (Model, []Effect) {
	next := model
	next.Transcript = append([]TranscriptEntry(nil), model.Transcript...)
	next.Queued = append([]QueuedEntry(nil), model.Queued...)
	next.Diagnostics = append([]diagnostic.Diagnostic(nil), model.Diagnostics...)

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
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "prompt", Text: text})
		return next, []Effect{FakePromptEffect{Operation: operation, Prompt: text}}
	case CommandSelected:
		operation := nextOperation(&next, OperationCommand, msg.Name)
		next.Status = StatusActive
		next.Result = ""
		next.LastCommand = msg.Name
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
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
		next.ActiveOperation = operation
		next.Transcript = append(next.Transcript, TranscriptEntry{Kind: "tool", Text: "bash " + bashSubjectLabel(request)})
		return next, []Effect{BashToolEffect{Operation: operation, Request: request}}
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
	case FakeEffectCompleted:
		next.Status = StatusIdle
		next.Result = msg.Result
		next.ActiveRead = ReadToolRequest{}
		next.LastRead = ReadToolResult{}
		next.ActiveSearch = SearchToolRequest{}
		next.LastSearch = SearchToolResult{}
		next.ActiveBash = BashToolRequest{}
		next.LastBash = BashToolResult{}
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
		next.ActiveOperation = OperationMetadata{}
		kind := "result"
		if msg.Result.Error.Kind != "" && msg.Result.Error.Kind != BashToolErrorNone {
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
	return status == StatusActive || status == StatusCanceling
}

func hasActiveFakeWork(model Model) bool {
	return hasActiveWork(model.Status) && !isToolOperation(model.ActiveOperation.Kind)
}

func isToolOperation(kind OperationKind) bool {
	return kind == OperationRead || kind == OperationFind || kind == OperationGrep || kind == OperationBash
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
