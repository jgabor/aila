package tui

import (
	"context"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jgabor/aila/internal/policy"
)

// PromptSubmitFunc routes non-empty submitted prompt text to the application layer.
type PromptSubmitFunc func(text string) TranscriptTurn

// CommandRouteFunc receives policy-owned command recommendations and returns app-owned view state.
type CommandRouteFunc func(policy.CommandRecommendation, ViewState) ViewState

// FileReferenceFunc receives prompt file-reference queries and returns app-owned rows.
type FileReferenceFunc func(query string, state ViewState) ViewState

// InterruptRequestFunc routes user interrupt intent to the application layer.
type InterruptRequestFunc func(reason string) TranscriptTurn

// ApprovalDecisionFunc routes approval choices to the application layer.
type ApprovalDecisionFunc func(decision ApprovalDecisionInput) TranscriptTurn

// ApprovalDecisionInput is the presentation-selected approval action.
type ApprovalDecisionInput struct {
	ProposalID string
	Action     string
}

// TranscriptTurn is the presentation data for one submitted prompt and response.
type TranscriptTurn struct {
	UserText           string
	AssistantText      string
	AssistantStreaming bool
	AssistantSource    string
	AssistantModel     string
	Phase              string
	PhaseSource        string
	SurfaceTitle       string
	RuntimeStatus      string
	StatusSource       string
	StatusDetail       string
	RuntimeActive      bool
	RuntimeResult      string
	QueuedCount        int
	QueuedText         []string
	Diagnostics        []DiagnosticView
	Read               *ReadView
	Search             *SearchView
	Command            *CommandView
	Utility            *UtilityView
	Compact            *CompactView
	Context            *ContextView
	Brief              *BriefView
	Fetch              *FetchView
	Mutation           *MutationView
	Recovery           *RecoveryView
	Approval           *ApprovalProposalView
	ApprovalDecision   *ApprovalDecisionView
}

// ApprovalProposalView is app-injected risky operation review data. It is
// display-only; TUI code must never execute the proposed operation.
type ApprovalProposalView struct {
	ID             string
	OperationKind  string
	Target         string
	RiskSummary    string
	PreviewLines   []string
	DefaultAction  string
	Path           string
	Command        []string
	WorkingDir     string
	ExpectedEffect string
	DiffPreview    []string
	Reversible     bool
	RunID          string
	Capability     string
}

// ApprovalDecisionView is app-injected decision state for display.
type ApprovalDecisionView struct {
	ProposalID string
	Action     string
	Stale      bool
}

// ReadView is app-injected read presentation data. It is display-only;
// TUI code must never validate paths or read workspace files itself.
type ReadView struct {
	Name             string
	Status           string
	ReadOnly         bool
	Path             string
	RequestedRange   ReadLineRangeView
	EffectiveRange   ReadLineRangeView
	PreviewLines     []string
	PreviewTruncated bool
	LineLimitHit     bool
	TruncationMarker string
	ErrorKind        string
	ErrorMessage     string
	Decision         *DecisionView
}

// DecisionView is app-injected autonomy decision evidence for a tool result.
type DecisionView struct {
	Autonomy         string
	Source           string
	Allowed          bool
	Automatic        bool
	ApprovalRequired bool
	Reason           string
	OperationKind    string
	Name             string
	Target           string
	Command          []string
	WorkingDir       string
	ExpectedEffect   string
	Reversible       bool
	RunID            string
	Capability       string
}

// ReadLineRangeView records 1-based read line references for presentation.
type ReadLineRangeView struct {
	StartLine int
	EndLine   int
	Limit     int
}

// SearchView is app-injected find/grep presentation data. It is display-only;
// TUI code must never walk directories, read files, or evaluate queries itself.
type SearchView struct {
	Name              string
	Status            string
	ReadOnly          bool
	Pattern           string
	Query             string
	Regex             bool
	IncludePattern    string
	Matches           []SearchMatchView
	OmittedResults    int
	OmittedFiles      int
	PreviewTruncated  bool
	ResultLimitHit    bool
	TruncationMarkers string
	ErrorKind         string
	ErrorMessage      string
	Decision          *DecisionView
}

// SearchMatchView records one injected find or grep match.
type SearchMatchView struct {
	Path        string
	LineNumber  int
	PreviewText string
}

// CommandView is app-injected safe bash presentation data. It is display-only;
// TUI code must never classify or execute commands itself.
type CommandView struct {
	Name            string
	Status          string
	ReadOnly        bool
	Argv            []string
	WorkingDir      string
	CommandFamily   string
	ExpectedEffect  string
	ExitCode        int
	StdoutLines     []string
	StderrLines     []string
	StdoutTruncated bool
	StderrTruncated bool
	DurationMillis  int64
	ErrorKind       string
	ErrorMessage    string
	Decision        *DecisionView
}

// UtilityView is app-injected idle-only utility worker state. It is display-only;
// TUI code must never schedule jobs, mutate files, run git, or change workflow phase.
type UtilityView struct {
	Source          string
	Status          string
	JobID           string
	JobKind         string
	Model           string
	Summary         string
	PreparedContext UtilityPreparedContextView
	StaleContext    UtilityStaleContextView
	SummaryRefresh  UtilitySummaryRefreshView
	Suggestions     []UtilitySuggestionView
	EvidenceRefs    []UtilityEvidenceRefView
	Caveats         []string
	DeniedReason    string
	DeniedDetail    string
	ReadOnly        bool
	Safety          UtilitySafetyView
}

// UtilityPreparedContextView records non-authoritative context prep output.
type UtilityPreparedContextView struct {
	Summary          string
	EvidenceRefIDs   []string
	Caveats          []string
	NonAuthoritative bool
}

// UtilityStaleContextView records display-only saved-context freshness output.
type UtilityStaleContextView struct {
	Status              string
	Summary             string
	EvidenceRefIDs      []string
	Caveats             []string
	SuggestedNextAction string
}

// UtilitySummaryRefreshView records display-only refreshed summary output.
type UtilitySummaryRefreshView struct {
	Status           string
	OriginalSummary  string
	RefreshedSummary string
	SourceRefIDs     []string
	ExactDetails     []string
	Confidence       string
	Caveats          []string
}

// UtilitySuggestionView records one display-only utility suggestion.
type UtilitySuggestionView struct {
	Text           string
	EvidenceRefIDs []string
}

// UtilityEvidenceRefView records evidence supporting utility output.
type UtilityEvidenceRefView struct {
	ID     string
	Kind   string
	Source string
	Detail string
}

// UtilitySafetyView records negative evidence for forbidden utility actions.
type UtilitySafetyView struct {
	FileMutation            bool
	GitMutation             bool
	ProjectArtifactMutation bool
	ApprovalGrant           bool
	WorkflowPhaseTransition bool
	FinalJudgment           bool
	ContextRefresh          bool
	ContextCompaction       bool
	ContextRewrite          bool
}

// CompactView is app-injected context compaction state. It is display-only;
// TUI code must never compact context, persist state, or call providers itself.
type CompactView struct {
	Mode          string
	Source        string
	Status        string
	Summary       string
	Meter         string
	OriginalMeter string
	Caveats       []string
	SourceRefs    []ContextSourceRefView
}

// ContextView is app-injected context assembly evidence. It is display-only;
// TUI code must never assemble context, read files, execute commands, or call providers.
type ContextView struct {
	Source     string
	Status     string
	Meter      string
	Blocks     []ContextBlockView
	Claims     []ContextClaimView
	SourceRefs []ContextSourceRefView
	Warnings   []string
}

// ContextBlockView records one app-injected context block.
type ContextBlockView struct {
	ID           string
	Kind         string
	Title        string
	Text         string
	SourceRefIDs []string
}

// ContextClaimView records visible summary text plus supporting refs.
type ContextClaimView struct {
	Text         string
	SourceRefIDs []string
}

// ContextSourceRefView records exact evidence that supports context claims.
type ContextSourceRefView struct {
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

// MutationView is app-injected edit/write presentation data. It is display-only;
// TUI code must never validate paths or mutate workspace files itself.
type MutationView struct {
	Name                  string
	Status                string
	Path                  string
	ExpectedEffect        string
	PreviousVersion       string
	NewVersion            string
	PreviousExists        bool
	BytesWritten          int
	ReplacementCount      int
	ResolvedPathAvailable bool
	ErrorKind             string
	ErrorMessage          string
	Decision              *DecisionView
}

// FetchView is app-injected network read presentation data. It is display-only;
// TUI code must never contact the network itself.
// RecoveryView is app-injected undo/redo presentation data. It is display-only;
// TUI code must never read history or mutate workspace files itself.
type RecoveryView struct {
	Command         string
	Status          string
	TargetEventID   string
	Action          string
	Paths           []string
	PreviousVersion string
	NewVersion      string
	RedoAvailable   bool
	RedoAction      string
	Reason          string
	ErrorKind       string
	ErrorMessage    string
	Decision        *DecisionView
}

type FetchView struct {
	Name              string
	Status            string
	ReadOnly          bool
	URL               string
	Method            string
	ExpectedEffect    string
	HTTPStatusCode    int
	HTTPStatus        string
	ContentType       string
	PreviewLines      []string
	PreviewTruncated  bool
	OmittedBytesKnown bool
	OmittedBytes      int64
	TruncationMarker  string
	DurationMillis    int64
	ErrorKind         string
	ErrorMessage      string
	Decision          *DecisionView
}

// Size is the terminal dimensions used by the static M2 renderer.
type Size struct {
	Width  int
	Height int
}

// LayoutClass names the deterministic responsive layout bucket for a terminal size.
type LayoutClass string

const (
	LayoutCompact  LayoutClass = "compact"
	LayoutStandard LayoutClass = "standard"
	LayoutSpacious LayoutClass = "spacious"
	LayoutDesktop  LayoutClass = "desktop"
)

// LayoutState is presentation-only responsive state derived from terminal size.
type LayoutState struct {
	Size             Size
	Class            LayoutClass
	RightRailVisible bool
}

// Model is the Bubble Tea model for the static shell.
type Model struct {
	state         ViewState
	size          Size
	layout        LayoutState
	submitPrompt  PromptSubmitFunc
	routeCommand  CommandRouteFunc
	fileReference FileReferenceFunc
	interrupt     InterruptRequestFunc
	approval      ApprovalDecisionFunc
	commandChord  bool
	quitting      bool
}

// NewModel creates the default static shell model.
func NewModel() Model {
	return NewModelWithSize(Size{Width: 80, Height: 24})
}

// NewModelWithSize creates a static shell model with deterministic dimensions.
func NewModelWithSize(size Size) Model {
	return NewModelWithSizeAndPromptSubmit(size, nil)
}

// NewModelWithSizeAndPromptSubmit creates a shell model with a prompt submit route.
func NewModelWithSizeAndPromptSubmit(size Size, submitPrompt PromptSubmitFunc) Model {
	return NewModelWithSizePromptSubmitAndCommandRoute(size, submitPrompt, nil)
}

// NewModelWithSizePromptSubmitAndCommandRoute creates a shell model with prompt and command routes.
func NewModelWithSizePromptSubmitAndCommandRoute(size Size, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc) Model {
	return NewModelWithStateSizePromptSubmitAndCommandRoute(IdleEmptyState(), size, submitPrompt, routeCommand)
}

// NewModelWithStateSizePromptSubmitAndCommandRoute creates a shell model from app-owned view state.
func NewModelWithStateSizePromptSubmitAndCommandRoute(state ViewState, size Size, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc) Model {
	return NewModelWithStateSizePromptSubmitCommandRouteAndInterrupt(state, size, submitPrompt, routeCommand, nil)
}

// NewModelWithStateSizePromptSubmitCommandRouteAndInterrupt creates a shell model with app-owned callbacks.
func NewModelWithStateSizePromptSubmitCommandRouteAndInterrupt(state ViewState, size Size, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc) Model {
	return NewModelWithStateSizePromptSubmitCommandRouteInterruptAndApproval(state, size, submitPrompt, routeCommand, interrupt, nil)
}

// NewModelWithStateSizePromptSubmitCommandRouteInterruptAndApproval creates a shell model with app-owned approval routing.
func NewModelWithStateSizePromptSubmitCommandRouteInterruptAndApproval(state ViewState, size Size, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc, approval ApprovalDecisionFunc) Model {
	return NewModelWithStateSizePromptSubmitCommandRouteInterruptApprovalAndFileReference(state, size, submitPrompt, routeCommand, interrupt, approval, nil)
}

// NewModelWithStateSizePromptSubmitCommandRouteInterruptApprovalAndFileReference creates a shell model with prompt file-reference routing.
func NewModelWithStateSizePromptSubmitCommandRouteInterruptApprovalAndFileReference(state ViewState, size Size, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc, approval ApprovalDecisionFunc, fileReference FileReferenceFunc) Model {
	size = normalizeSize(size)
	return Model{
		state:         state,
		size:          size,
		layout:        layoutForSize(size),
		submitPrompt:  submitPrompt,
		routeCommand:  routeCommand,
		fileReference: fileReference,
		interrupt:     interrupt,
		approval:      approval,
	}
}

// NewProgram constructs the Bubble Tea program for the static shell.
func NewProgram(input io.Reader, output io.Writer) *tea.Program {
	return NewProgramWithPromptSubmit(input, output, nil)
}

// NewProgramWithPromptSubmit constructs the Bubble Tea program with app prompt routing.
func NewProgramWithPromptSubmit(input io.Reader, output io.Writer, submitPrompt PromptSubmitFunc) *tea.Program {
	return NewProgramWithStateAndPromptSubmit(input, output, IdleEmptyState(), submitPrompt)
}

// NewProgramWithStateAndPromptSubmit constructs the Bubble Tea program with app-owned view state.
func NewProgramWithStateAndPromptSubmit(input io.Reader, output io.Writer, state ViewState, submitPrompt PromptSubmitFunc) *tea.Program {
	return NewProgramWithStatePromptSubmitAndCommandRoute(input, output, state, submitPrompt, nil)
}

// NewProgramWithStatePromptSubmitAndCommandRoute constructs the Bubble Tea program with app-owned callbacks.
func NewProgramWithStatePromptSubmitAndCommandRoute(input io.Reader, output io.Writer, state ViewState, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc) *tea.Program {
	return NewProgramWithStatePromptSubmitCommandRouteAndInterrupt(input, output, state, submitPrompt, routeCommand, nil)
}

// NewProgramWithStatePromptSubmitCommandRouteAndInterrupt constructs the Bubble Tea program with app-owned callbacks.
func NewProgramWithStatePromptSubmitCommandRouteAndInterrupt(input io.Reader, output io.Writer, state ViewState, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc) *tea.Program {
	return NewProgramWithContextStatePromptSubmitCommandRouteAndInterrupt(context.Background(), input, output, state, submitPrompt, routeCommand, interrupt)
}

// NewProgramWithContextStatePromptSubmitCommandRouteAndInterrupt constructs a Bubble Tea program canceled by an app-owned context.
func NewProgramWithContextStatePromptSubmitCommandRouteAndInterrupt(ctx context.Context, input io.Reader, output io.Writer, state ViewState, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc) *tea.Program {
	return NewProgramWithContextStatePromptSubmitCommandRouteInterruptAndApproval(ctx, input, output, state, submitPrompt, routeCommand, interrupt, nil)
}

// NewProgramWithContextStatePromptSubmitCommandRouteInterruptAndApproval constructs a Bubble Tea program with approval routing.
func NewProgramWithContextStatePromptSubmitCommandRouteInterruptAndApproval(ctx context.Context, input io.Reader, output io.Writer, state ViewState, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc, approval ApprovalDecisionFunc) *tea.Program {
	return NewProgramWithContextStatePromptSubmitCommandRouteInterruptApprovalAndFileReference(ctx, input, output, state, submitPrompt, routeCommand, interrupt, approval, nil)
}

// NewProgramWithContextStatePromptSubmitCommandRouteInterruptApprovalAndFileReference constructs a Bubble Tea program with prompt file-reference routing.
func NewProgramWithContextStatePromptSubmitCommandRouteInterruptApprovalAndFileReference(ctx context.Context, input io.Reader, output io.Writer, state ViewState, submitPrompt PromptSubmitFunc, routeCommand CommandRouteFunc, interrupt InterruptRequestFunc, approval ApprovalDecisionFunc, fileReference FileReferenceFunc) *tea.Program {
	if ctx == nil {
		ctx = context.Background()
	}
	options := []tea.ProgramOption{tea.WithAltScreen()}
	options = append(options, tea.WithContext(ctx), tea.WithoutSignalHandler())
	if input != nil {
		options = append(options, tea.WithInput(input))
	}
	if output != nil {
		options = append(options, tea.WithOutput(output))
	}
	return tea.NewProgram(NewModelWithStateSizePromptSubmitCommandRouteInterruptApprovalAndFileReference(state, Size{Width: 80, Height: 24}, submitPrompt, routeCommand, interrupt, approval, fileReference), options...)
}

// Init has no startup effect for the static shell.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles presentation-only terminal messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.size = normalizeSize(Size{Width: msg.Width, Height: msg.Height})
		m.layout = layoutForSize(m.size)
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, m.requestInterrupt("ctrl-c")
		}
		if m.commandChord {
			m.commandChord = false
			return m, m.routeShortcut(msg)
		}
		if msg.Type == tea.KeyCtrlX {
			m.commandChord = true
			return m, nil
		}
		if m.state.PromptInput == "" && msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
		if m.state.Approval != nil {
			return m, m.handleApprovalKey(msg)
		}
		if m.modelSwitchFocused() {
			return m.handleModelSwitchKey(msg)
		}
		if m.autonomySwitchFocused() {
			return m.handleAutonomySwitchKey(msg)
		}
		if m.fileReferenceFocused() {
			return m.handleFileReferenceKey(msg), nil
		}
		if m.sessionFocused() {
			return m.handleSessionKey(msg), nil
		}
		if m.historyFocused() {
			return m.handleHistoryKey(msg), nil
		}
		if m.diffFocused() {
			return m.handleDiffKey(msg), nil
		}
		return m, m.handlePromptKey(msg)
	}
	return m, nil
}

// View renders the static shell with ANSI styling.
func (m Model) View() string {
	return RenderANSI(m.state, m.size)
}

// Quitting reports whether the single M2 quit path was selected.
func (m Model) Quitting() bool {
	return m.quitting
}

// PromptInput reports the current prompt input view state.
func (m Model) PromptInput() string {
	return m.state.PromptInput
}

// Layout reports the current presentation-only responsive layout state.
func (m Model) Layout() LayoutState {
	return m.layout
}

func (m *Model) handlePromptKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyRunes:
		text := string(msg.Runes)
		m.state = appendPromptInputText(m.state, text)
		if !tea.Key(msg).Paste && text == "@" {
			m.refreshFileReference("")
		}
	case tea.KeySpace:
		m.state = appendPromptInputText(m.state, " ")
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.state = dropPromptInputRune(m.state)
	case tea.KeyEnter:
		if m.state.PromptInput == "" || strings.TrimSpace(m.state.PromptInput) == "" {
			return nil
		}
		text := m.state.PromptInput
		m.state = clearPromptInput(m.state)
		if recommendation, ok := policy.RecommendSlashCommand(text); ok {
			return m.routeRecommendation(recommendation)
		}
		if m.submitPrompt != nil {
			turn := m.submitPrompt(text)
			m.state = ApplyTranscriptTurn(m.state, turn)
		}
	}
	return nil
}

func (m *Model) refreshFileReference(query string) {
	if m.fileReference != nil {
		m.state = m.fileReference(query, m.state)
		return
	}
	m.state = ApplyFileReferenceView(m.state, &FileReferenceView{
		Source: "policy.command",
		Status: "unavailable",
		Query:  query,
		Detail: "app-owned file reference discovery unavailable in presentation-only fallback",
		Focus:  true,
	})
}

// ApplyTranscriptTurn applies app-owned runtime presentation data to a view state.
func ApplyTranscriptTurn(state ViewState, turn TranscriptTurn) ViewState {
	if turn.UserText != "" || turn.AssistantText != "" {
		state.Transcript = append(state.Transcript, turn)
	}
	return applyRuntimeStatus(state, turn)
}

func applyRuntimeStatus(state ViewState, turn TranscriptTurn) ViewState {
	if turn.Phase != "" {
		state.Phase = turn.Phase
	}
	if turn.PhaseSource != "" {
		state.PhaseSource = turn.PhaseSource
	}
	if turn.SurfaceTitle != "" {
		state.SurfaceTitle = turn.SurfaceTitle
	}
	if turn.RuntimeStatus == "" {
		return state
	}
	state.RuntimeStatus = turn.RuntimeStatus
	state.StatusSource = turn.StatusSource
	state.StatusDetail = turn.StatusDetail
	state.RuntimeActive = turn.RuntimeActive
	state.RuntimeResult = turn.RuntimeResult
	state.QueuedCount = turn.QueuedCount
	state.QueuedText = append([]string(nil), turn.QueuedText...)
	state.Diagnostics = mergeDiagnosticViews(state.Diagnostics, turn.Diagnostics)
	state.Read = cloneReadView(turn.Read)
	state.Search = cloneSearchView(turn.Search)
	state.Command = cloneCommandView(turn.Command)
	state.Utility = cloneUtilityView(turn.Utility)
	state.Compact = cloneCompactView(turn.Compact)
	state.Context = cloneContextView(turn.Context)
	if turn.Brief != nil {
		state.Brief = cloneBriefView(turn.Brief)
	}
	if state.Context != nil && state.Context.Meter != "" {
		state.FooterContext = state.Context.Meter
	}
	state.Fetch = cloneFetchView(turn.Fetch)
	state.Mutation = cloneMutationView(turn.Mutation)
	state.Recovery = cloneRecoveryView(turn.Recovery)
	state.Approval = cloneApprovalProposalView(turn.Approval)
	state.ApprovalDecision = cloneApprovalDecisionView(turn.ApprovalDecision)
	return state
}

func cloneApprovalProposalView(approval *ApprovalProposalView) *ApprovalProposalView {
	if approval == nil {
		return nil
	}
	clone := *approval
	clone.PreviewLines = append([]string(nil), approval.PreviewLines...)
	clone.Command = append([]string(nil), approval.Command...)
	clone.DiffPreview = append([]string(nil), approval.DiffPreview...)
	return &clone
}

func cloneApprovalDecisionView(decision *ApprovalDecisionView) *ApprovalDecisionView {
	if decision == nil {
		return nil
	}
	clone := *decision
	return &clone
}

func cloneReadView(read *ReadView) *ReadView {
	if read == nil {
		return nil
	}
	clone := *read
	clone.PreviewLines = append([]string(nil), read.PreviewLines...)
	clone.Decision = cloneDecisionView(read.Decision)
	return &clone
}

func cloneSearchView(search *SearchView) *SearchView {
	if search == nil {
		return nil
	}
	clone := *search
	clone.Matches = append([]SearchMatchView(nil), search.Matches...)
	clone.Decision = cloneDecisionView(search.Decision)
	return &clone
}

func cloneCommandView(command *CommandView) *CommandView {
	if command == nil {
		return nil
	}
	clone := *command
	clone.Argv = append([]string(nil), command.Argv...)
	clone.StdoutLines = append([]string(nil), command.StdoutLines...)
	clone.StderrLines = append([]string(nil), command.StderrLines...)
	clone.Decision = cloneDecisionView(command.Decision)
	return &clone
}

func cloneUtilityView(utility *UtilityView) *UtilityView {
	if utility == nil {
		return nil
	}
	clone := *utility
	clone.PreparedContext.EvidenceRefIDs = append([]string(nil), utility.PreparedContext.EvidenceRefIDs...)
	clone.PreparedContext.Caveats = append([]string(nil), utility.PreparedContext.Caveats...)
	clone.StaleContext.EvidenceRefIDs = append([]string(nil), utility.StaleContext.EvidenceRefIDs...)
	clone.StaleContext.Caveats = append([]string(nil), utility.StaleContext.Caveats...)
	clone.SummaryRefresh.SourceRefIDs = append([]string(nil), utility.SummaryRefresh.SourceRefIDs...)
	clone.SummaryRefresh.ExactDetails = append([]string(nil), utility.SummaryRefresh.ExactDetails...)
	clone.SummaryRefresh.Caveats = append([]string(nil), utility.SummaryRefresh.Caveats...)
	clone.Suggestions = make([]UtilitySuggestionView, 0, len(utility.Suggestions))
	for _, suggestion := range utility.Suggestions {
		item := suggestion
		item.EvidenceRefIDs = append([]string(nil), suggestion.EvidenceRefIDs...)
		clone.Suggestions = append(clone.Suggestions, item)
	}
	clone.EvidenceRefs = append([]UtilityEvidenceRefView(nil), utility.EvidenceRefs...)
	clone.Caveats = append([]string(nil), utility.Caveats...)
	return &clone
}

func cloneCompactView(compact *CompactView) *CompactView {
	if compact == nil {
		return nil
	}
	clone := *compact
	clone.Caveats = append([]string(nil), compact.Caveats...)
	clone.SourceRefs = append([]ContextSourceRefView(nil), compact.SourceRefs...)
	return &clone
}

func cloneContextView(contextView *ContextView) *ContextView {
	if contextView == nil {
		return nil
	}
	clone := *contextView
	clone.Blocks = make([]ContextBlockView, 0, len(contextView.Blocks))
	for _, block := range contextView.Blocks {
		cloned := block
		cloned.SourceRefIDs = append([]string(nil), block.SourceRefIDs...)
		clone.Blocks = append(clone.Blocks, cloned)
	}
	clone.Claims = make([]ContextClaimView, 0, len(contextView.Claims))
	for _, claim := range contextView.Claims {
		cloned := claim
		cloned.SourceRefIDs = append([]string(nil), claim.SourceRefIDs...)
		clone.Claims = append(clone.Claims, cloned)
	}
	clone.SourceRefs = append([]ContextSourceRefView(nil), contextView.SourceRefs...)
	clone.Warnings = append([]string(nil), contextView.Warnings...)
	return &clone
}

func cloneMutationView(mutation *MutationView) *MutationView {
	if mutation == nil {
		return nil
	}
	clone := *mutation
	clone.Decision = cloneDecisionView(mutation.Decision)
	return &clone
}

func cloneRecoveryView(recovery *RecoveryView) *RecoveryView {
	if recovery == nil {
		return nil
	}
	clone := *recovery
	clone.Paths = append([]string(nil), recovery.Paths...)
	clone.Decision = cloneDecisionView(recovery.Decision)
	return &clone
}

func cloneFetchView(fetch *FetchView) *FetchView {
	if fetch == nil {
		return nil
	}
	clone := *fetch
	clone.PreviewLines = append([]string(nil), fetch.PreviewLines...)
	clone.Decision = cloneDecisionView(fetch.Decision)
	return &clone
}

func cloneDecisionView(decision *DecisionView) *DecisionView {
	if decision == nil {
		return nil
	}
	clone := *decision
	clone.Command = append([]string(nil), decision.Command...)
	return &clone
}

func mergeDiagnosticViews(existing []DiagnosticView, added []DiagnosticView) []DiagnosticView {
	if len(added) == 0 {
		return existing
	}
	merged := append([]DiagnosticView(nil), existing...)
	for _, diagnostic := range added {
		if !hasDiagnosticView(merged, diagnostic) {
			merged = append(merged, diagnostic)
		}
	}
	return merged
}

func hasDiagnosticView(diagnostics []DiagnosticView, diagnostic DiagnosticView) bool {
	for _, existing := range diagnostics {
		if existing == diagnostic {
			return true
		}
	}
	return false
}

func (m *Model) handleApprovalKey(msg tea.KeyMsg) tea.Cmd {
	action := approvalActionForKey(msg)
	if action == "" || m.approval == nil || m.state.Approval == nil {
		return nil
	}
	turn := m.approval(ApprovalDecisionInput{ProposalID: m.state.Approval.ID, Action: action})
	m.state = ApplyTranscriptTurn(m.state, turn)
	return nil
}

func approvalActionForKey(msg tea.KeyMsg) string {
	key := msg.String()
	if msg.Type == tea.KeyRunes {
		key = string(msg.Runes)
	}
	switch strings.ToLower(key) {
	case "a":
		return "approve"
	case "n":
		return "deny"
	case "d":
		return "defer"
	default:
		return ""
	}
}

func (m *Model) routeShortcut(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if msg.Type == tea.KeyRunes {
		key = string(msg.Runes)
	}
	recommendation, ok := policy.RecommendShortcut("ctrl+x", key)
	if !ok {
		return nil
	}
	return m.routeRecommendation(recommendation)
}

func (m *Model) routeRecommendation(recommendation policy.CommandRecommendation) tea.Cmd {
	m.state = ApplyCommandRecommendation(m.state, recommendation)
	if m.routeCommand != nil {
		m.state = m.routeCommand(recommendation, m.state)
	}
	if recommendation.Route == policy.CommandRouteQuit {
		m.quitting = true
		return tea.Quit
	}
	return nil
}

func (m *Model) requestInterrupt(reason string) tea.Cmd {
	if m.interrupt != nil {
		turn := m.interrupt(reason)
		m.state = ApplyTranscriptTurn(m.state, turn)
	}
	return nil
}

// ApplyCommandRecommendation applies the visible command surface for a policy route.
func ApplyCommandRecommendation(state ViewState, recommendation policy.CommandRecommendation) ViewState {
	state.CommandRoute = string(recommendation.Route)
	state.RouteSource = "policy.command"
	if recommendation.Route != policy.CommandRouteModel {
		state.ModelSwitch = nil
	}
	if recommendation.Route != policy.CommandRouteAuto {
		state.AutonomySwitch = nil
	}
	if recommendation.Route != policy.CommandRouteEditor {
		state.PromptEditor = nil
	}
	if recommendation.Route != policy.CommandRouteStatus {
		state.Utility = nil
	}
	state.FileReference = nil
	switch recommendation.Route {
	case policy.CommandRouteNew:
		state = ApplySessionView(state, &SessionView{
			Action:       "new",
			Source:       "policy.command",
			Status:       "fresh",
			SessionID:    "current",
			MemoryStatus: "fresh",
			Detail:       "app-owned new session unavailable in presentation-only fallback",
		})
	case policy.CommandRouteClear:
		state = ApplySessionView(state, &SessionView{
			Action:       "clear",
			Source:       "policy.command",
			Status:       "cleared",
			SessionID:    "current",
			MemoryStatus: "cleared",
			Detail:       "app-owned clear session unavailable in presentation-only fallback",
		})
	case policy.CommandRouteContinue:
		state = ApplySessionView(state, &SessionView{
			Action:       "continue",
			Source:       "policy.command",
			Status:       "no_memory",
			SessionID:    "current",
			MemoryStatus: "no_memory",
			Detail:       "app-owned continue session unavailable in presentation-only fallback",
		})
	case policy.CommandRouteEditor:
		state = ApplyPromptEditorView(state, &PromptEditorView{
			Source: "policy.command",
			Status: "unavailable",
			Detail: "app-owned editor unavailable in presentation-only fallback",
		})
	case policy.CommandRouteModel:
		state = applyModelCommandFallback(state, recommendation)
	case policy.CommandRouteAuto:
		state = applyAutonomyCommandFallback(state, recommendation)
	case policy.CommandRouteStatus:
		state.SurfaceTitle = "status"
		state.SurfaceLines = []string{
			"app-owned status inspection unavailable in presentation-only fallback",
			"stage: " + state.Phase,
			"primary model: " + state.PrimaryModel,
			"utility model: " + state.UtilityModel,
			"autonomy: " + state.Autonomy,
			"git: " + state.FooterGit,
			"context: " + state.FooterContext,
		}
	case policy.CommandRouteReview:
		state.SurfaceTitle = "review"
		state.SurfaceLines = []string{
			"app-owned review inspection unavailable in presentation-only fallback",
			"read-only: true",
			"model-assisted review: not invoked",
		}
	case policy.CommandRouteHelp:
		state.SurfaceTitle = "help"
		state.SurfaceLines = []string{
			"Deterministic placeholder help.",
			"commands:",
			"/new - Start a fresh session and preserve project memory.",
			"/clear - Clear visible session state and current memory.",
			"/continue - Restore the current saved session.",
			"/editor - Edit the current prompt in $EDITOR.",
			"/model - Choose the active primary model for this session.",
			"/model --utility - Choose the active utility model for this session.",
			"/auto - Choose the active autonomy level for this session.",
			"/status - Inspect current runtime and state.",
			"/review - Inspect current changes, risks, and sources.",
			"/history - Browse runs, edits, checks, and undo data.",
			"/compact - Immediately compact the current conversation.",
			"/diff - Review current changes.",
			"/undo - Undo the latest supported mutation.",
			"/redo - Redo the latest supported recovery.",
			"/quit - Quit Aila.",
			"shortcuts:",
			"ctrl+x n - Start a fresh session and preserve project memory.",
			"ctrl+x c - Restore the current saved session.",
			"ctrl+x e - Edit the current prompt in $EDITOR.",
			"ctrl+x m - Choose the active primary model for this session.",
			"ctrl+x a - Choose the active autonomy level for this session.",
			"ctrl+x s - Inspect current runtime and state.",
			"ctrl+x i - Inspect current changes, risks, and sources.",
			"ctrl+x h - Browse runs, edits, checks, and undo data.",
			"ctrl+x k - Immediately compact the current conversation.",
			"ctrl+x d - Review current changes.",
			"ctrl+x u - Undo the latest supported mutation.",
			"ctrl+x r - Redo the latest supported recovery.",
		}
	case policy.CommandRouteHistory:
		state = ApplyHistoryView(state, nil, 0, true)
	case policy.CommandRouteCompact:
		state.SurfaceTitle = "compact"
		state.SurfaceLines = []string{
			"app-owned manual compaction unavailable in presentation-only fallback",
			"background compaction: out of scope",
		}
	case policy.CommandRouteDiff:
		state = ApplyDiffView(state, &DiffView{Source: "policy.command", Status: "empty", Empty: true}, 0, true)
	}
	return state
}

// ApplyCommandSurface injects app-owned read-only command display data into the TUI state.
func ApplyCommandSurface(state ViewState, route policy.CommandRoute, title string, lines []string) ViewState {
	state.CommandRoute = string(route)
	state.RouteSource = "policy.command"
	state.SurfaceTitle = title
	state.SurfaceLines = append([]string(nil), lines...)
	return state
}

// ApplyBriefView injects app-owned brief capability output into visible state.
func ApplyBriefView(state ViewState, brief *BriefView) ViewState {
	if brief == nil {
		return state
	}
	state.Brief = cloneBriefView(brief)
	return state
}

// ApplyPolicyRouteView injects app-owned policy routing evidence into visible state.
func ApplyPolicyRouteView(state ViewState, route *PolicyRouteView) ViewState {
	if route == nil {
		return state
	}
	state.PolicyRoute = clonePolicyRouteView(route)
	return state
}

func cloneBriefView(brief *BriefView) *BriefView {
	if brief == nil {
		return nil
	}
	clone := *brief
	clone.KnownGaps = append([]string(nil), brief.KnownGaps...)
	clone.SourceRefs = append([]BriefSourceRefView(nil), brief.SourceRefs...)
	clone.BoundaryRequests = append([]BriefBoundaryRequestView(nil), brief.BoundaryRequests...)
	return &clone
}

func clonePolicyRouteView(route *PolicyRouteView) *PolicyRouteView {
	if route == nil {
		return nil
	}
	clone := *route
	clone.SourceRefs = append([]PolicyRouteSourceRefView(nil), route.SourceRefs...)
	clone.BoundaryRequests = append([]PolicyRouteBoundaryRequestView(nil), route.BoundaryRequests...)
	return &clone
}

// ApplySessionView injects app-owned session lifecycle display data into the TUI state.
func ApplySessionView(state ViewState, session *SessionView) ViewState {
	if session == nil {
		return state
	}
	state.CommandRoute = session.Action
	state.RouteSource = "policy.command"
	state.SurfaceTitle = "session"
	state.Session = cloneSessionView(session)
	state.Session.Selected = clampSessionSelection(*state.Session)
	state.SurfaceLines = sessionSurfaceLines(*state.Session)
	return state
}

func cloneSessionView(session *SessionView) *SessionView {
	if session == nil {
		return nil
	}
	clone := *session
	clone.Items = append([]SessionItemView(nil), session.Items...)
	return &clone
}

func clampSessionSelection(session SessionView) int {
	if len(session.Items) == 0 {
		return 0
	}
	if session.Selected < 0 {
		return 0
	}
	if session.Selected >= len(session.Items) {
		return len(session.Items) - 1
	}
	return session.Selected
}

func applyModelCommandFallback(state ViewState, recommendation policy.CommandRecommendation) ViewState {
	target := recommendation.Target
	if target == policy.CommandTargetNone {
		target = policy.CommandTargetPrimaryModel
	}
	if recommendation.Selection != "" {
		switch target {
		case policy.CommandTargetUtilityModel:
			state.UtilityModel = recommendation.Selection
		default:
			state.PrimaryModel = recommendation.Selection
		}
	}
	modelSwitch := state.ModelSwitch
	if modelSwitch == nil {
		current := state.PrimaryModel
		if target == policy.CommandTargetUtilityModel {
			current = state.UtilityModel
		}
		modelSwitch = &ModelSwitchView{
			Target:         string(target),
			Source:         "policy.command",
			Status:         "fallback",
			CurrentPrimary: state.PrimaryModel,
			CurrentUtility: state.UtilityModel,
			Detail:         "app-owned model selection unavailable in presentation-only fallback",
			Focus:          true,
			Items: []ModelSwitchItemView{{
				Label:            current,
				SourceName:       "current",
				Model:            current,
				Family:           "session",
				Class:            "current",
				Status:           "current",
				CredentialSource: "not inspected",
				Current:          true,
			}},
		}
	} else {
		cloned := cloneModelSwitchView(modelSwitch)
		modelSwitch = cloned
		modelSwitch.Target = string(target)
		modelSwitch.CurrentPrimary = state.PrimaryModel
		modelSwitch.CurrentUtility = state.UtilityModel
		if recommendation.Selection != "" {
			for index := range modelSwitch.Items {
				modelSwitch.Items[index].Current = modelSwitch.Items[index].Label == recommendation.Selection
				if modelSwitch.Items[index].Label == recommendation.Selection {
					modelSwitch.Selected = index
				}
			}
		}
	}
	return ApplyModelSwitchView(state, modelSwitch)
}

func applyAutonomyCommandFallback(state ViewState, recommendation policy.CommandRecommendation) ViewState {
	if recommendation.Selection != "" {
		state.Autonomy = recommendation.Selection
	}
	autonomySwitch := state.AutonomySwitch
	if autonomySwitch == nil {
		autonomySwitch = &AutonomySwitchView{
			Source:  "policy.command",
			Status:  "fallback",
			Current: state.Autonomy,
			Detail:  "app-owned autonomy selection unavailable in presentation-only fallback",
			Focus:   true,
			Items: []AutonomySwitchItemView{{
				Level:   state.Autonomy,
				Status:  "current",
				Detail:  "current session autonomy",
				Current: true,
			}},
		}
	} else {
		cloned := cloneAutonomySwitchView(autonomySwitch)
		autonomySwitch = cloned
		autonomySwitch.Current = state.Autonomy
		if recommendation.Selection != "" {
			for index := range autonomySwitch.Items {
				autonomySwitch.Items[index].Current = autonomySwitch.Items[index].Level == recommendation.Selection
				if autonomySwitch.Items[index].Level == recommendation.Selection {
					autonomySwitch.Selected = index
				}
			}
		}
	}
	return ApplyAutonomySwitchView(state, autonomySwitch)
}

// ApplyModelSwitchView injects app-owned model selection display data into the TUI state.
func ApplyModelSwitchView(state ViewState, modelSwitch *ModelSwitchView) ViewState {
	if modelSwitch == nil {
		return state
	}
	state.CommandRoute = string(policy.CommandRouteModel)
	state.RouteSource = "policy.command"
	state.SurfaceTitle = "model"
	state.ModelSwitch = cloneModelSwitchView(modelSwitch)
	state.ModelSwitch.Selected = clampModelSwitchSelection(*state.ModelSwitch)
	state.SurfaceLines = modelSwitchSurfaceLines(*state.ModelSwitch)
	return state
}

func cloneModelSwitchView(modelSwitch *ModelSwitchView) *ModelSwitchView {
	if modelSwitch == nil {
		return nil
	}
	clone := *modelSwitch
	clone.Items = append([]ModelSwitchItemView(nil), modelSwitch.Items...)
	return &clone
}

func clampModelSwitchSelection(modelSwitch ModelSwitchView) int {
	if len(modelSwitch.Items) == 0 {
		return 0
	}
	if modelSwitch.Selected < 0 {
		return 0
	}
	if modelSwitch.Selected >= len(modelSwitch.Items) {
		return len(modelSwitch.Items) - 1
	}
	return modelSwitch.Selected
}

// ApplyAutonomySwitchView injects app-owned autonomy selection display data into the TUI state.
func ApplyAutonomySwitchView(state ViewState, autonomySwitch *AutonomySwitchView) ViewState {
	if autonomySwitch == nil {
		return state
	}
	state.CommandRoute = string(policy.CommandRouteAuto)
	state.RouteSource = "policy.command"
	state.SurfaceTitle = "auto"
	state.AutonomySwitch = cloneAutonomySwitchView(autonomySwitch)
	state.AutonomySwitch.Selected = clampAutonomySwitchSelection(*state.AutonomySwitch)
	state.SurfaceLines = autonomySwitchSurfaceLines(*state.AutonomySwitch)
	return state
}

func cloneAutonomySwitchView(autonomySwitch *AutonomySwitchView) *AutonomySwitchView {
	if autonomySwitch == nil {
		return nil
	}
	clone := *autonomySwitch
	clone.Items = append([]AutonomySwitchItemView(nil), autonomySwitch.Items...)
	return &clone
}

func clampAutonomySwitchSelection(autonomySwitch AutonomySwitchView) int {
	if len(autonomySwitch.Items) == 0 {
		return 0
	}
	if autonomySwitch.Selected < 0 {
		return 0
	}
	if autonomySwitch.Selected >= len(autonomySwitch.Items) {
		return len(autonomySwitch.Items) - 1
	}
	return autonomySwitch.Selected
}

// ApplyHistoryView injects app-owned read-only history display data into the TUI state.
func ApplyHistoryView(state ViewState, items []HistoryItem, selected int, focus bool) ViewState {
	state.CommandRoute = string(policy.CommandRouteHistory)
	state.RouteSource = "policy.command"
	state.SurfaceTitle = "history"
	state.HistoryItems = cloneHistoryItems(items)
	state.HistoryEmpty = len(items) == 0
	state.HistoryFocus = focus
	state.HistorySelected = selected
	state.HistorySelected = clampHistorySelection(state)
	state.SurfaceLines = historySurfaceLines(state)
	return state
}

// ApplyRecoveryView injects app-owned undo/redo result display data into the TUI state.
func ApplyRecoveryView(state ViewState, recovery *RecoveryView) ViewState {
	state.RouteSource = "policy.command"
	if recovery != nil {
		state.CommandRoute = recovery.Command
	}
	state.SurfaceTitle = "recovery"
	state.Recovery = cloneRecoveryView(recovery)
	state.SurfaceLines = recoverySurfaceLines(recovery)
	return state
}

// ApplyDiffView injects app-owned read-only diff display data into the TUI state.
func ApplyDiffView(state ViewState, diff *DiffView, selected int, focus bool) ViewState {
	state.CommandRoute = string(policy.CommandRouteDiff)
	state.RouteSource = "policy.command"
	state.SurfaceTitle = "diff"
	state.Diff = cloneDiffView(diff)
	if state.Diff == nil {
		state.Diff = &DiffView{Source: "app.diff", Status: "empty", Empty: true}
	}
	state.DiffFocus = focus
	state.DiffSelected = selected
	state.DiffSelected = clampDiffSelection(state)
	state.SurfaceLines = diffSurfaceLines(state)
	return state
}

// ClearDiffView exits the focused diff surface without touching lower layers.
func ClearDiffView(state ViewState) ViewState {
	state.CommandRoute = ""
	state.RouteSource = ""
	state.SurfaceTitle = ""
	state.SurfaceLines = nil
	state.Diff = nil
	state.DiffSelected = 0
	state.DiffFocus = false
	return state
}

func cloneDiffView(diff *DiffView) *DiffView {
	if diff == nil {
		return nil
	}
	clone := *diff
	clone.Files = make([]DiffFileView, 0, len(diff.Files))
	for _, file := range diff.Files {
		fileClone := file
		fileClone.Hunks = make([]DiffHunkView, 0, len(file.Hunks))
		for _, hunk := range file.Hunks {
			hunkClone := hunk
			hunkClone.Lines = append([]DiffLineView(nil), hunk.Lines...)
			fileClone.Hunks = append(fileClone.Hunks, hunkClone)
		}
		clone.Files = append(clone.Files, fileClone)
	}
	return &clone
}

func cloneHistoryItems(items []HistoryItem) []HistoryItem {
	if len(items) == 0 {
		return nil
	}
	clone := make([]HistoryItem, 0, len(items))
	for _, item := range items {
		itemClone := item
		if item.Mutation != nil {
			mutation := *item.Mutation
			mutation.ChangedPaths = append([]string(nil), item.Mutation.ChangedPaths...)
			itemClone.Mutation = &mutation
		}
		if item.Undo != nil {
			undo := *item.Undo
			undo.Paths = append([]string(nil), item.Undo.Paths...)
			itemClone.Undo = &undo
		}
		if item.Recovery != nil {
			recovery := *item.Recovery
			recovery.Paths = append([]string(nil), item.Recovery.Paths...)
			itemClone.Recovery = &recovery
		}
		clone = append(clone, itemClone)
	}
	return clone
}

func (m Model) fileReferenceFocused() bool {
	return m.state.FileReference != nil && m.state.FileReference.Focus && m.state.SurfaceTitle == "file-reference"
}

func (m Model) handleFileReferenceKey(msg tea.KeyMsg) Model {
	if m.state.FileReference == nil {
		return m
	}
	switch msg.Type {
	case tea.KeyUp:
		m.state.FileReference.Selected--
	case tea.KeyDown:
		m.state.FileReference.Selected++
	case tea.KeyHome:
		m.state.FileReference.Selected = 0
	case tea.KeyEnd:
		m.state.FileReference.Selected = len(m.state.FileReference.Items) - 1
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.state.FileReference.Query = dropLastRune(m.state.FileReference.Query)
		m.refreshFileReference(m.state.FileReference.Query)
		return m
	case tea.KeyRunes:
		if !tea.Key(msg).Paste {
			m.state.FileReference.Query += string(msg.Runes)
			m.refreshFileReference(m.state.FileReference.Query)
			return m
		}
	case tea.KeyEsc:
		m.state = closeFileReference(m.state, "canceled", "file reference picker closed")
		return m
	case tea.KeyEnter:
		m.state = insertSelectedFileReference(m.state)
		return m
	}
	m.state.FileReference.Selected = clampFileReferenceSelection(*m.state.FileReference)
	m.state.SurfaceLines = fileReferenceSurfaceLines(*m.state.FileReference)
	return m
}

func (m Model) modelSwitchFocused() bool {
	return m.state.ModelSwitch != nil && m.state.ModelSwitch.Focus && m.state.SurfaceTitle == "model"
}

func (m Model) handleModelSwitchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.state.ModelSwitch == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyUp:
		m.state.ModelSwitch.Selected--
	case tea.KeyDown:
		m.state.ModelSwitch.Selected++
	case tea.KeyHome:
		m.state.ModelSwitch.Selected = 0
	case tea.KeyEnd:
		m.state.ModelSwitch.Selected = len(m.state.ModelSwitch.Items) - 1
	case tea.KeyEsc:
		m.state.ModelSwitch.Focus = false
	case tea.KeyEnter:
		selected := clampModelSwitchSelection(*m.state.ModelSwitch)
		if len(m.state.ModelSwitch.Items) == 0 {
			m.state.ModelSwitch.Focus = false
			break
		}
		selection := m.state.ModelSwitch.Items[selected].Label
		target := policy.CommandTarget(m.state.ModelSwitch.Target)
		if target == policy.CommandTargetNone {
			target = policy.CommandTargetPrimaryModel
		}
		return m, m.routeRecommendation(policy.CommandRecommendation{Route: policy.CommandRouteModel, Kind: policy.CommandInputSelection, Target: target, Selection: selection})
	}
	m.state.ModelSwitch.Selected = clampModelSwitchSelection(*m.state.ModelSwitch)
	m.state.SurfaceLines = modelSwitchSurfaceLines(*m.state.ModelSwitch)
	return m, nil
}

func (m Model) autonomySwitchFocused() bool {
	return m.state.AutonomySwitch != nil && m.state.AutonomySwitch.Focus && m.state.SurfaceTitle == "auto"
}

func (m Model) handleAutonomySwitchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.state.AutonomySwitch == nil {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyUp:
		m.state.AutonomySwitch.Selected--
	case tea.KeyDown:
		m.state.AutonomySwitch.Selected++
	case tea.KeyHome:
		m.state.AutonomySwitch.Selected = 0
	case tea.KeyEnd:
		m.state.AutonomySwitch.Selected = len(m.state.AutonomySwitch.Items) - 1
	case tea.KeyEsc:
		m.state.AutonomySwitch.Focus = false
	case tea.KeyEnter:
		selected := clampAutonomySwitchSelection(*m.state.AutonomySwitch)
		if len(m.state.AutonomySwitch.Items) == 0 {
			m.state.AutonomySwitch.Focus = false
			break
		}
		selection := m.state.AutonomySwitch.Items[selected].Level
		return m, m.routeRecommendation(policy.CommandRecommendation{Route: policy.CommandRouteAuto, Kind: policy.CommandInputSelection, Target: policy.CommandTargetAutonomy, Selection: selection})
	}
	m.state.AutonomySwitch.Selected = clampAutonomySwitchSelection(*m.state.AutonomySwitch)
	m.state.SurfaceLines = autonomySwitchSurfaceLines(*m.state.AutonomySwitch)
	return m, nil
}

func (m Model) sessionFocused() bool {
	return m.state.Session != nil && m.state.Session.Focus && m.state.SurfaceTitle == "session"
}

func (m Model) handleSessionKey(msg tea.KeyMsg) Model {
	if m.state.Session == nil {
		return m
	}
	switch msg.Type {
	case tea.KeyUp:
		m.state.Session.Selected--
	case tea.KeyDown:
		m.state.Session.Selected++
	case tea.KeyHome:
		m.state.Session.Selected = 0
	case tea.KeyEnd:
		m.state.Session.Selected = len(m.state.Session.Items) - 1
	case tea.KeyEnter, tea.KeyEsc:
		m.state.Session.Focus = false
	}
	m.state.Session.Selected = clampSessionSelection(*m.state.Session)
	m.state.SurfaceLines = sessionSurfaceLines(*m.state.Session)
	return m
}

func (m Model) historyFocused() bool {
	return m.state.HistoryFocus && m.state.SurfaceTitle == "history"
}

func (m Model) handleHistoryKey(msg tea.KeyMsg) Model {
	switch msg.Type {
	case tea.KeyUp:
		m.state.HistorySelected--
	case tea.KeyDown:
		m.state.HistorySelected++
	case tea.KeyHome:
		m.state.HistorySelected = 0
	case tea.KeyEnd:
		m.state.HistorySelected = len(m.state.HistoryItems) - 1
	case tea.KeyPgUp:
		m.state.HistorySelected -= 12
	case tea.KeyPgDown:
		m.state.HistorySelected += 12
	case tea.KeyEsc:
		m.state.HistoryFocus = false
	}
	m.state.HistorySelected = clampHistorySelection(m.state)
	m.state.SurfaceLines = historySurfaceLines(m.state)
	return m
}

func (m Model) diffFocused() bool {
	return m.state.DiffFocus && m.state.SurfaceTitle == "diff"
}

func (m Model) handleDiffKey(msg tea.KeyMsg) Model {
	switch msg.Type {
	case tea.KeyUp:
		m.state.DiffSelected--
	case tea.KeyDown:
		m.state.DiffSelected++
	case tea.KeyHome:
		m.state.DiffSelected = 0
	case tea.KeyEnd:
		m.state.DiffSelected = len(diffRows(m.state)) - 1
	case tea.KeyPgUp:
		m.state.DiffSelected -= 12
	case tea.KeyPgDown:
		m.state.DiffSelected += 12
	case tea.KeyEsc:
		m.state = ClearDiffView(m.state)
		return m
	}
	m.state.DiffSelected = clampDiffSelection(m.state)
	m.state.SurfaceLines = diffSurfaceLines(m.state)
	return m
}

func dropLastRune(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	return string(runes[:len(runes)-1])
}

func normalizeSize(size Size) Size {
	if size.Width <= 0 {
		size.Width = 80
	}
	if size.Height <= 0 {
		size.Height = 24
	}
	return size
}

func layoutForSize(size Size) LayoutState {
	size = normalizeSize(size)
	layout := LayoutState{Size: size, Class: LayoutCompact}
	switch {
	case size.Width >= 140:
		layout.Class = LayoutDesktop
		layout.RightRailVisible = true
	case size.Width >= 120:
		layout.Class = LayoutSpacious
	case size.Width >= 100:
		layout.Class = LayoutStandard
	}
	return layout
}
