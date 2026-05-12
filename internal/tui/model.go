package tui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Size is the terminal dimensions used by the static M2 renderer.
type Size struct {
	Width  int
	Height int
}

// Model is the Bubble Tea model for the static shell.
type Model struct {
	state    ViewState
	size     Size
	quitting bool
}

// NewModel creates the default static shell model.
func NewModel() Model {
	return NewModelWithSize(Size{Width: 80, Height: 24})
}

// NewModelWithSize creates a static shell model with deterministic dimensions.
func NewModelWithSize(size Size) Model {
	return Model{
		state: IdleEmptyState(),
		size:  normalizeSize(size),
	}
}

// NewProgram constructs the Bubble Tea program for the static shell.
func NewProgram(input io.Reader, output io.Writer) *tea.Program {
	options := []tea.ProgramOption{tea.WithAltScreen()}
	if input != nil {
		options = append(options, tea.WithInput(input))
	}
	if output != nil {
		options = append(options, tea.WithOutput(output))
	}
	return tea.NewProgram(NewModel(), options...)
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
	case tea.KeyMsg:
		if msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
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

func normalizeSize(size Size) Size {
	if size.Width <= 0 {
		size.Width = 80
	}
	if size.Height <= 0 {
		size.Height = 24
	}
	return size
}
