package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/tui"
)

func TestDefaultDisplayConfigMatchesReadmeDefaults(t *testing.T) {
	t.Parallel()

	got := DefaultDisplayConfig()
	want := DisplayConfig{
		PrimaryModel: "opencode-go/deepseek-v4-pro:high",
		UtilityModel: "opencode-go/deepseek-v4-flash:max",
		Autonomy:     "yolo",
	}
	if got != want {
		t.Fatalf("display config = %+v, want %+v", got, want)
	}
}

func TestInjectedDisplayConfigRendersWithoutGlobalState(t *testing.T) {
	t.Parallel()

	first := NewDisplayState(tui.IdleEmptyState(), DisplayConfig{
		PrimaryModel: "test-provider/primary:low",
		UtilityModel: "test-provider/utility:none",
		Autonomy:     "read-only-label",
	})
	second := NewDisplayState(tui.IdleEmptyState(), DisplayConfig{
		PrimaryModel: "other-provider/primary:max",
		UtilityModel: "other-provider/utility:min",
		Autonomy:     "write-label",
	})

	firstRender := tui.RenderPlain(first, tui.Size{Width: 120, Height: 32})
	secondRender := tui.RenderPlain(second, tui.Size{Width: 120, Height: 32})
	if !containsAll(firstRender, []string{"Model test-provider/primary:low", "Utility test-provider/utility:none", "Auto read-only-label"}) {
		t.Fatalf("first render missing injected labels:\n%s", firstRender)
	}
	if !containsAll(secondRender, []string{"Model other-provider/primary:max", "Utility other-provider/utility:min", "Auto write-label"}) {
		t.Fatalf("second render missing injected labels:\n%s", secondRender)
	}
	if strings.Contains(firstRender, "other-provider") || strings.Contains(secondRender, "test-provider") {
		t.Fatalf("injected display labels leaked between renders:\nfirst:\n%s\nsecond:\n%s", firstRender, secondRender)
	}
	if got := tui.IdleEmptyState(); got.PrimaryModel != "placeholder" || got.UtilityModel != "placeholder" || got.Autonomy != "placeholder" {
		t.Fatalf("idle state changed globally: %+v", got)
	}
}

func TestInitialDisplayStateUsesAppOwnedDefaults(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	state, err := initialDisplayState()
	if err != nil {
		t.Fatalf("load initial display state: %v", err)
	}
	render := tui.RenderPlain(state, tui.Size{Width: 120, Height: 32})
	for _, token := range []string{
		"Model opencode-go/deepseek-v4-pro:high",
		"Utility opencode-go/deepseek-v4-flash:max",
		"Auto yolo",
	} {
		if !strings.Contains(render, token) {
			t.Fatalf("initial app display state render missing %q:\n%s", token, render)
		}
	}
	if _, err := os.Stat(filepath.Join(configHome, "aila", "config.toml")); err != nil {
		t.Fatalf("startup did not create default config: %v", err)
	}
}

func TestInitialDisplayStateUsesLoadedConfigInRenderAndSemantics(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	writeFile(t, filepath.Join(configHome, "aila", "config.toml"), `[llm]
model = "test-provider/primary:low"

[llm.utility]
model = "test-provider/utility:min"

[autonomy]
level = "read"
`)

	state, err := initialDisplayState()
	if err != nil {
		t.Fatalf("load initial display state: %v", err)
	}
	render := tui.RenderPlain(state, tui.Size{Width: 120, Height: 32})
	for _, token := range []string{
		"Model test-provider/primary:low",
		"Utility test-provider/utility:min",
		"Auto read",
		"primary model: test-provider/primary:low",
		"utility model: test-provider/utility:min",
		"autonomy: read (display-only)",
	} {
		if !strings.Contains(render, token) {
			t.Fatalf("startup render missing loaded token %q:\n%s", token, render)
		}
	}

	semantic := tui.Semantic(state, tui.Size{Width: 120, Height: 32})
	if semantic.Session.PrimaryModel != "test-provider/primary:low" || semantic.Session.UtilityModel != "test-provider/utility:min" || semantic.Session.Autonomy != "read" {
		t.Fatalf("startup semantics did not use loaded config: %+v", semantic.Session)
	}
}

func TestInitialDisplayStateMalformedConfigErrorsBoundedlyAndPreservesFile(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	path := filepath.Join(configHome, "aila", "config.toml")
	malformed := `[llm]
model = test-provider/primary:low
`
	writeFile(t, path, malformed)

	_, err := initialDisplayState()
	if err == nil {
		t.Fatal("malformed startup config succeeded")
	}
	if !strings.Contains(err.Error(), "load startup config: load config") || !strings.Contains(err.Error(), "value must be a quoted string") {
		t.Fatalf("malformed startup error = %v", err)
	}
	if len(err.Error()) > 300 {
		t.Fatalf("malformed startup error is not bounded: %d bytes: %v", len(err.Error()), err)
	}
	if after := readFile(t, path); after != malformed {
		t.Fatalf("malformed startup config was modified:\n%s", after)
	}
}

func TestDisplayConfigSourceBoundaryStaysInMemory(t *testing.T) {
	t.Parallel()

	imports := parseDisplayConfigImports(t)
	for _, forbidden := range []string{
		"context",
		"io",
		"os",
		"os/exec",
		"net",
		"net/http",
		"path/filepath",
		"github.com/jgabor/aila/internal/agent",
		"github.com/jgabor/aila/internal/capability",
		"github.com/jgabor/aila/internal/context",
		"github.com/jgabor/aila/internal/history",
		"github.com/jgabor/aila/internal/permission",
		"github.com/jgabor/aila/internal/policy",
		"github.com/jgabor/aila/internal/runtime",
		"github.com/jgabor/aila/internal/state",
		"github.com/jgabor/aila/internal/tools",
		"github.com/jgabor/aila/internal/utility",
		"github.com/jgabor/aila/internal/workflow",
	} {
		if imports[forbidden] {
			t.Fatalf("display config source imports forbidden runtime dependency %q", forbidden)
		}
	}

	source := readDisplayConfigSource(t)
	for _, forbidden := range []string{
		"ReadFile",
		"Open(",
		"LookupEnv",
		"Getenv",
		"XDG_",
		"config.toml",
		".aila",
		"Provider",
		"Credential",
		"Session",
		"Workflow",
		"Tool",
		"Permission",
		"Persist",
		"http",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("display config source contains forbidden boundary token %q", forbidden)
		}
	}
}

func TestDisplayConfigAutonomyIsLabelOnly(t *testing.T) {
	t.Parallel()

	field, ok := reflect.TypeOf(DisplayConfig{}).FieldByName("Autonomy")
	if !ok {
		t.Fatal("DisplayConfig.Autonomy field missing")
	}
	if field.Type.Kind() != reflect.String {
		t.Fatalf("DisplayConfig.Autonomy type = %s, want string label", field.Type)
	}

	source := readDisplayConfigSource(t)
	for _, forbidden := range []string{
		"Classify",
		"Approval",
		"Approve",
		"Policy",
		"Operation",
		"Allowed",
		"Denied",
		"permission.",
		"policy.",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("autonomy display contract contains behavior token %q", forbidden)
		}
	}
}

func parseDisplayConfigImports(t *testing.T) map[string]bool {
	t.Helper()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "display_config.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse display config boundary: %v", err)
	}
	imports := map[string]bool{}
	for _, spec := range parsed.Imports {
		imports[strings.Trim(spec.Path.Value, "\"")] = true
	}
	return imports
}

func readDisplayConfigSource(t *testing.T) string {
	t.Helper()

	source, err := os.ReadFile("display_config.go")
	if err != nil {
		t.Fatalf("read display config source: %v", err)
	}
	return string(source)
}

func containsAll(value string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(value, token) {
			return false
		}
	}
	return true
}
