package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIdleEmptyFixtureShape(t *testing.T) {
	t.Parallel()

	data := readFixtureFile(t, "fixture.json")
	var fixture struct {
		Name             string            `json:"name"`
		Kind             string            `json:"kind"`
		TerminalBehavior string            `json:"terminal_behavior"`
		QuitInput        string            `json:"quit_input"`
		States           []string          `json:"states"`
		RenderOutputs    map[string]string `json:"render_outputs"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal idle-empty fixture: %v", err)
	}
	if fixture.Name != "idle-empty" {
		t.Fatalf("fixture name = %q, want idle-empty", fixture.Name)
	}
	if fixture.Kind != "static_shell" {
		t.Fatalf("fixture kind = %q, want static_shell", fixture.Kind)
	}
	if fixture.TerminalBehavior != "bubbletea_static" {
		t.Fatalf("terminal behavior = %q, want bubbletea_static", fixture.TerminalBehavior)
	}
	if fixture.QuitInput != "q" {
		t.Fatalf("quit input = %q, want q", fixture.QuitInput)
	}
	if len(fixture.States) != 1 || fixture.States[0] != "idle" {
		t.Fatalf("fixture states = %v, want [idle]", fixture.States)
	}
	for _, name := range []string{
		"plain_80x24",
		"plain_120x32",
		"ansi_80x24",
		"ansi_120x32",
		"semantic_80x24",
		"semantic_120x32",
	} {
		if fixture.RenderOutputs[name] == "" {
			t.Fatalf("render output %q missing from fixture metadata", name)
		}
	}
}

func TestIdleEmptyRenderSnapshots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		size   Size
		file   string
		render func(ViewState, Size) string
	}{
		{name: "plain 80x24", size: Size{Width: 80, Height: 24}, file: "plain-80x24.txt", render: RenderPlain},
		{name: "plain 120x32", size: Size{Width: 120, Height: 32}, file: "plain-120x32.txt", render: RenderPlain},
		{name: "ansi 80x24", size: Size{Width: 80, Height: 24}, file: "ansi-80x24.txt", render: RenderANSI},
		{name: "ansi 120x32", size: Size{Width: 120, Height: 32}, file: "ansi-120x32.txt", render: RenderANSI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.render(IdleEmptyState(), tc.size)
			want := snapshotString(t, tc.file)
			if got != want {
				t.Fatalf("render mismatch for %s\nwant:\n%s\n\ngot:\n%s", tc.file, want, got)
			}
		})
	}
}

func TestIdleEmptySemanticSnapshots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		size Size
		file string
	}{
		{name: "semantic 80x24", size: Size{Width: 80, Height: 24}, file: "semantic-80x24.json"},
		{name: "semantic 120x32", size: Size{Width: 120, Height: 32}, file: "semantic-120x32.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := RenderSemanticJSON(IdleEmptyState(), tc.size)
			want := snapshotString(t, tc.file)
			if got != want {
				t.Fatalf("semantic mismatch for %s\nwant:\n%s\n\ngot:\n%s", tc.file, want, got)
			}

			var snapshot SemanticSnapshot
			if err := json.Unmarshal([]byte(got), &snapshot); err != nil {
				t.Fatalf("unmarshal semantic snapshot: %v", err)
			}
			if snapshot.Session.Phase != "placeholder" || snapshot.Session.PhaseSource != "not_started" {
				t.Fatalf("phase = %q from %q, want placeholder from not_started", snapshot.Session.Phase, snapshot.Session.PhaseSource)
			}
			if snapshot.Session.WorkflowTransition {
				t.Fatal("placeholder phase must not imply workflow transition behavior")
			}
			if len(snapshot.Actions) != 1 || snapshot.Actions[0].Input != "q" {
				t.Fatalf("actions = %+v, want single q quit action", snapshot.Actions)
			}
		})
	}
}

func TestStaticModelQuitPath(t *testing.T) {
	t.Parallel()

	model := NewModelWithSize(Size{Width: 80, Height: 24})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	quitModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.Model", updated)
	}
	if !quitModel.Quitting() {
		t.Fatal("q should mark the static model as quitting")
	}
	if cmd == nil {
		t.Fatal("q should emit a Bubble Tea quit command")
	}

	model = NewModelWithSize(Size{Width: 80, Height: 24})
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc is out of scope for M2 and must not quit")
	}
	if updated.(Model).Quitting() {
		t.Fatal("Esc should not mark the static model as quitting")
	}
}

func readFixtureFile(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", "fixtures", "idle-empty", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func snapshotString(t *testing.T, name string) string {
	t.Helper()

	return strings.TrimSuffix(string(readFixtureFile(t, name)), "\n")
}
