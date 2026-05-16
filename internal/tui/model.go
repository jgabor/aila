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

// CommandRouteFunc receives policy-owned command recommendations selected by the TUI.
type CommandRouteFunc func(policy.CommandRecommendation)

// InterruptRequestFunc routes user interrupt intent to the application layer.
type InterruptRequestFunc func(reason string) TranscriptTurn

// TranscriptTurn is the presentation data for one submitted prompt and response.
type TranscriptTurn struct {
	UserText      string
	AssistantText string
	RuntimeStatus string
	StatusSource  string
	StatusDetail  string
	RuntimeActive bool
	RuntimeResult string
	QueuedCount   int
	QueuedText    []string
	Diagnostics   []DiagnosticView
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
	state        ViewState
	size         Size
	layout       LayoutState
	submitPrompt PromptSubmitFunc
	routeCommand CommandRouteFunc
	interrupt    InterruptRequestFunc
	commandChord bool
	quitting     bool
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
	size = normalizeSize(size)
	return Model{
		state:        state,
		size:         size,
		layout:       layoutForSize(size),
		submitPrompt: submitPrompt,
		routeCommand: routeCommand,
		interrupt:    interrupt,
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
	return tea.NewProgram(NewModelWithStateSizePromptSubmitCommandRouteAndInterrupt(state, Size{Width: 80, Height: 24}, submitPrompt, routeCommand, interrupt), options...)
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
		m.state.PromptInput += string(msg.Runes)
	case tea.KeySpace:
		m.state.PromptInput += " "
	case tea.KeyBackspace, tea.KeyCtrlH:
		m.state.PromptInput = dropLastRune(m.state.PromptInput)
	case tea.KeyEnter:
		if m.state.PromptInput == "" || strings.TrimSpace(m.state.PromptInput) == "" {
			return nil
		}
		text := m.state.PromptInput
		m.state.PromptInput = ""
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

// ApplyTranscriptTurn applies app-owned runtime presentation data to a view state.
func ApplyTranscriptTurn(state ViewState, turn TranscriptTurn) ViewState {
	if turn.UserText != "" || turn.AssistantText != "" {
		state.Transcript = append(state.Transcript, turn)
	}
	return applyRuntimeStatus(state, turn)
}

func applyRuntimeStatus(state ViewState, turn TranscriptTurn) ViewState {
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
	return state
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

func (m *Model) routeShortcut(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if msg.Type == tea.KeyRunes {
		key = string(msg.Runes)
	}
	if key == "c" {
		return m.requestInterrupt("ctrl+x c")
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
		m.routeCommand(recommendation)
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
	switch recommendation.Route {
	case policy.CommandRouteStatus:
		lines := []string{
			"Deterministic placeholder status.",
			"stage " + state.Phase + " (display-only)",
			"primary model " + state.PrimaryModel,
			"utility model " + state.UtilityModel,
			"autonomy: " + state.Autonomy,
		}
		if state.ProjectStoreStatus != "" {
			lines = append(lines, "project store: "+state.ProjectStoreStatus+" ("+state.ProjectStoreSource+"; "+state.ProjectStoreDetail+")")
		}
		for _, line := range diagnosticLines(state.Diagnostics) {
			lines = append(lines, strings.TrimSpace(line))
		}
		lines = append(lines,
			"git: "+state.FooterGit,
			"context: "+state.FooterContext,
			"real status sources: deferred",
		)
		state.SurfaceTitle = "status"
		state.SurfaceLines = lines
	case policy.CommandRouteHelp:
		state.SurfaceTitle = "help"
		state.SurfaceLines = []string{
			"Deterministic placeholder help.",
			"commands:",
			"/status - Show deterministic placeholder status.",
			"/help - Show this deterministic placeholder help.",
			"/quit - Quit Aila.",
			"shortcuts:",
			"ctrl+x s - Show deterministic placeholder status.",
			"ctrl+x q - Quit Aila.",
		}
	}
	return state
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
