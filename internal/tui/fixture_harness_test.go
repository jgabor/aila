package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const updateSnapshotsEnv = "AILA_UPDATE_TUI_SNAPSHOTS"

type renderFixture struct {
	Name             string            `json:"name"`
	Kind             string            `json:"kind"`
	TerminalBehavior string            `json:"terminal_behavior"`
	QuitInput        string            `json:"quit_input"`
	States           []string          `json:"states"`
	Sizes            []fixtureSize     `json:"render_sizes"`
	Outputs          map[string]string `json:"render_outputs"`
	State            ViewState         `json:"-"`
	path             string
}

type fixtureSize struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type renderCase struct {
	name   string
	size   Size
	file   string
	render func(ViewState, Size) string
}

type semanticCase struct {
	name string
	size Size
	file string
}

func loadRenderFixture(t *testing.T, name string, state ViewState) renderFixture {
	t.Helper()

	fixture := renderFixture{
		State: state,
		path:  filepath.Join("testdata", "fixtures", name),
	}
	data := fixture.ReadFile(t, "fixture.json")
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal %s fixture: %v", name, err)
	}
	fixture.State = state
	fixture.path = filepath.Join("testdata", "fixtures", name)
	return fixture
}

func (f renderFixture) TextCases() []renderCase {
	cases := make([]renderCase, 0, len(f.Sizes)*2)
	for _, size := range f.Sizes {
		terminalSize := Size{Width: size.Width, Height: size.Height}
		cases = append(cases,
			renderCase{name: "plain " + size.Name, size: terminalSize, file: f.Outputs[fixtureOutputKey("plain", size.Name)], render: RenderPlain},
			renderCase{name: "ansi " + size.Name, size: terminalSize, file: f.Outputs[fixtureOutputKey("ansi", size.Name)], render: RenderANSI},
		)
	}
	return cases
}

func (f renderFixture) SemanticCases() []semanticCase {
	cases := make([]semanticCase, 0, len(f.Sizes))
	for _, size := range f.Sizes {
		cases = append(cases, semanticCase{
			name: "semantic " + size.Name,
			size: Size{Width: size.Width, Height: size.Height},
			file: f.Outputs[fixtureOutputKey("semantic", size.Name)],
		})
	}
	return cases
}

func fixtureOutputKey(kind string, size string) string {
	return kind + "_" + size
}

func (f renderFixture) SnapshotString(t *testing.T, name string) string {
	t.Helper()

	return strings.TrimSuffix(string(f.ReadFile(t, name)), "\n")
}

func (f renderFixture) SnapshotPath(name string) string {
	return filepath.Join(f.path, name)
}

func (f renderFixture) ReadFile(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join(f.path, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func assertTextSnapshot(t *testing.T, fixture renderFixture, file string, got string) {
	t.Helper()

	want, ok := fixture.SnapshotStringIfExists(t, file)
	if !ok {
		path := fixture.SnapshotPath(file)
		updateSnapshot(t, path, got)
		t.Fatalf("render snapshot missing for fixture output %s", path)
	}
	if got == want {
		return
	}

	path := fixture.SnapshotPath(file)
	updateSnapshot(t, path, got)
	t.Fatalf("render mismatch for fixture output %s\nwant:\n%s\n\ngot:\n%s", path, want, got)
}

func assertSemanticSnapshot(t *testing.T, fixture renderFixture, file string, got string) {
	t.Helper()

	want, ok := fixture.SnapshotStringIfExists(t, file)
	if !ok {
		path := fixture.SnapshotPath(file)
		updateSnapshot(t, path, got)
		t.Fatalf("semantic snapshot missing for fixture output %s", path)
	}
	var wantJSON any
	if err := json.Unmarshal([]byte(want), &wantJSON); err != nil {
		t.Fatalf("unmarshal semantic snapshot %s: %v", fixture.SnapshotPath(file), err)
	}
	var gotJSON any
	if err := json.Unmarshal([]byte(got), &gotJSON); err != nil {
		t.Fatalf("unmarshal rendered semantic snapshot %s: %v", fixture.SnapshotPath(file), err)
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		path := fixture.SnapshotPath(file)
		updateSnapshot(t, path, got)
		t.Fatalf("semantic mismatch for fixture output %s\nwant:\n%s\n\ngot:\n%s", path, want, got)
	}
}

func (f renderFixture) SnapshotStringIfExists(t *testing.T, name string) (string, bool) {
	t.Helper()

	path := f.SnapshotPath(name)
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSuffix(string(data), "\n"), true
	}
	if os.IsNotExist(err) {
		return "", false
	}
	t.Fatalf("read %s: %v", path, err)
	return "", false
}

func updateSnapshot(t *testing.T, path string, got string) {
	t.Helper()

	if !snapshotUpdateRequested(t) {
		return
	}
	if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
		t.Fatalf("update snapshot %s: %v", path, err)
	}
	t.Fatalf("updated snapshot %s; review the git diff before accepting this change", path)
}

func snapshotUpdateRequested(t *testing.T) bool {
	t.Helper()

	value := os.Getenv(updateSnapshotsEnv)
	switch strings.ToLower(value) {
	case "", "0", "false":
		return false
	case "1", "true":
		return true
	default:
		t.Fatalf("invalid %s=%q; use 1/true to update TUI snapshots or leave unset for read-only tests", updateSnapshotsEnv, value)
		return false
	}
}
