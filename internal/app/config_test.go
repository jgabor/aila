package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigPathUsesXDGConfigHome(t *testing.T) {
	t.Parallel()

	xdg := filepath.Join(t.TempDir(), "xdg-config")
	got, err := resolveConfigPath(func(key string) (string, bool) {
		switch key {
		case "XDG_CONFIG_HOME":
			return xdg, true
		case "HOME":
			return filepath.Join(t.TempDir(), "home"), true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatalf("resolve config path: %v", err)
	}
	want := filepath.Join(xdg, "aila", "config.toml")
	if got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
}

func TestResolveConfigPathFallsBackToHome(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "home")
	got, err := resolveConfigPath(func(key string) (string, bool) {
		switch key {
		case "XDG_CONFIG_HOME":
			return "", false
		case "HOME":
			return home, true
		default:
			return "", false
		}
	})
	if err != nil {
		t.Fatalf("resolve config path: %v", err)
	}
	want := filepath.Join(home, ".config", "aila", "config.toml")
	if got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
}

func TestLoadConfigFileCreatesReadmeDefaultsWhenAbsent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config-home", "aila", "config.toml")
	got, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("load absent config: %v", err)
	}
	if got != DefaultConfig() {
		t.Fatalf("created config = %+v, want %+v", got, DefaultConfig())
	}
	content := readFile(t, path)
	for _, token := range []string{
		`[llm]`,
		`model = "opencode-go/deepseek-v4-pro:high"`,
		`[llm.utility]`,
		`model = "opencode-go/deepseek-v4-flash:max"`,
		`[autonomy]`,
		`level = "yolo"`,
	} {
		if !strings.Contains(content, token) {
			t.Fatalf("created config missing %q:\n%s", token, content)
		}
	}
	if strings.Contains(content, "base_url") {
		t.Fatalf("created default config unexpectedly wrote base_url:\n%s", content)
	}
	assertNoStepBudgetConfigSurface(t, content)
}

func TestConfigCommandOutputDoesNotExposeAgentStepBudget(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	output, err := ConfigCommandOutput(true)
	if err != nil {
		t.Fatalf("config command output: %v", err)
	}

	assertNoStepBudgetConfigSurface(t, output)
}

func TestLoadConfigFilePreservesPresentConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "aila", "config.toml")
	writeFile(t, path, `[llm]
	base_url = "http://localhost:11434/v1"
model = "test/primary:low"

[llm.utility]
model = "test/utility:min"

[autonomy]
level = "read"
`)
	before := readFile(t, path)
	got, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("load present config: %v", err)
	}
	want := Config{
		LLM: LLMConfig{
			BaseURL: "http://localhost:11434/v1",
			Model:   mustParseTestModelRef(t, "test/primary:low"),
			Utility: UtilityLLMConfig{
				Model: mustParseTestModelRef(t, "test/utility:min"),
			},
		},
		Autonomy: AutonomyConfig{Level: "read"},
	}
	if got != want {
		t.Fatalf("loaded config = %+v, want %+v", got, want)
	}
	if after := readFile(t, path); after != before {
		t.Fatalf("present config was modified:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestLoadConfigFileLeavesBaseURLAbsentWhenUnset(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "aila", "config.toml")
	writeFile(t, path, `[llm]
model = "test/primary:low"

[llm.utility]
model = "test/utility:min"

[autonomy]
level = "read"
`)
	got, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("load present config: %v", err)
	}
	if got.LLM.BaseURL != "" {
		t.Fatalf("base_url = %q, want empty", got.LLM.BaseURL)
	}
}

func TestParseModelRefParsesTypedFieldsAndPreservesLabel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		label     string
		provider  string
		model     string
		reasoning string
	}{
		{label: "openai/gpt-4.1", provider: "openai", model: "gpt-4.1"},
		{label: "opencode-go/deepseek-v4-pro:high", provider: "opencode-go", model: "deepseek-v4-pro", reasoning: "high"},
	} {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()

			got, err := ParseModelRef(tc.label)
			if err != nil {
				t.Fatalf("parse model ref: %v", err)
			}
			if got.Label != tc.label || got.String() != tc.label || got.Provider != tc.provider || got.Model != tc.model || got.Reasoning != tc.reasoning {
				t.Fatalf("model ref = %+v, want label=%q provider=%q model=%q reasoning=%q", got, tc.label, tc.provider, tc.model, tc.reasoning)
			}
		})
	}
}

func TestLoadConfigFileMalformedModelRefsErrorBoundedlyAndPreserveFile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		model   string
		wantErr string
	}{
		{name: "missing provider", model: "/primary", wantErr: "missing provider"},
		{name: "missing model", model: "test/", wantErr: "missing model"},
		{name: "missing separator", model: "test-primary", wantErr: "<provider>/<model>[:reasoning]"},
		{name: "empty reasoning", model: "test/primary:", wantErr: "empty reasoning suffix"},
		{name: "nested parts", model: "test//primary", wantErr: "empty or nested parts"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "aila", "config.toml")
			content := `[llm]
model = "` + tc.model + `"

[llm.utility]
model = "test/utility:min"

[autonomy]
level = "yolo"
`
			writeFile(t, path, content)
			_, err := LoadConfigFile(path)
			if err == nil {
				t.Fatal("load malformed model ref succeeded")
			}
			if !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "llm.model") {
				t.Fatalf("malformed model ref error = %v, want %q", err, tc.wantErr)
			}
			if len(err.Error()) > 300 {
				t.Fatalf("malformed model ref error is not bounded: %d bytes: %v", len(err.Error()), err)
			}
			if after := readFile(t, path); after != content {
				t.Fatalf("malformed config was modified:\n%s", after)
			}
		})
	}
}

func TestLoadConfigFileMalformedErrorsAndPreservesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "aila", "config.toml")
	malformed := `[llm]
model = opencode-go/deepseek-v4-pro:high
`
	writeFile(t, path, malformed)
	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("load malformed config succeeded")
	}
	if !strings.Contains(err.Error(), "value must be a quoted string") {
		t.Fatalf("malformed error = %v", err)
	}
	if after := readFile(t, path); after != malformed {
		t.Fatalf("malformed config was modified:\n%s", after)
	}
}

func TestLoadConfigFileMissingKeyErrorsAndPreservesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "aila", "config.toml")
	missing := `[llm]
model = "test/primary:low"

[autonomy]
level = "yolo"
`
	writeFile(t, path, missing)
	_, err := LoadConfigFile(path)
	if !errors.Is(err, errConfigMissingRequiredKey) {
		t.Fatalf("missing key error = %v, want %v", err, errConfigMissingRequiredKey)
	}
	if !strings.Contains(err.Error(), "llm.utility.model") {
		t.Fatalf("missing key error does not name key: %v", err)
	}
	if after := readFile(t, path); after != missing {
		t.Fatalf("missing-key config was modified:\n%s", after)
	}
}

func TestLoadConfigFileRejectsUnsupportedKeys(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "aila", "config.toml")
	unsupported := `[llm]
model = "test/primary:low"
temperature = "0.2"

[llm.utility]
model = "test/utility:min"

[autonomy]
level = "yolo"
`
	writeFile(t, path, unsupported)
	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("load config with unsupported key succeeded")
	}
	if !strings.Contains(err.Error(), `unsupported key "llm.temperature"`) {
		t.Fatalf("unsupported key error = %v", err)
	}
	if after := readFile(t, path); after != unsupported {
		t.Fatalf("unsupported-key config was modified:\n%s", after)
	}
}

func TestLoadConfigDoesNotCreateWorkspaceAilaStateOrTouchSecrets(t *testing.T) {
	workspace := t.TempDir()
	secretPath := filepath.Join(workspace, ".env")
	secretContent := "OPENAI_API_KEY=do-not-read-or-print\n"
	writeFile(t, secretPath, secretContent)
	configHome := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(current); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	_, path, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	wantPath := filepath.Join(configHome, "aila", "config.toml")
	if path != wantPath {
		t.Fatalf("loaded path = %q, want %q", path, wantPath)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".aila")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace .aila stat error = %v, want not exist", err)
	}
	if after := readFile(t, secretPath); after != secretContent {
		t.Fatalf("secret file was modified:\n%s", after)
	}
}

func mustParseTestModelRef(t *testing.T, label string) ModelRef {
	t.Helper()
	ref, err := ParseModelRef(label)
	if err != nil {
		t.Fatalf("parse test model ref %q: %v", label, err)
	}
	return ref
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(content)
}

func assertNoStepBudgetConfigSurface(t *testing.T, content string) {
	t.Helper()

	lower := strings.ToLower(content)
	for _, forbidden := range []string{"maxsteps", "max_steps", "max-steps", "step_budget", "step-budget", "step budget", "agent_turn_limit", "agent-turn-limit", "agent turn limit"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("config surface exposes agent step budget token %q:\n%s", forbidden, content)
		}
	}
}
