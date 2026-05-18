package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jgabor/aila/internal/agent"
	"github.com/jgabor/aila/internal/permission"
)

func TestProductionAgentRunnerFactoryBuildsRealGoAgentRunnerForCustomConfig(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.LLM.BaseURL = "https://example.invalid/v1"
	config.LLM.Model = mustParseTestModelRef(t, "custom/deepseek-chat:high")
	selection := newAgentBuildRunnerFromConfig(t.TempDir(), string(permission.AutonomyRead), config, func(name string) (string, bool) {
		if name == agent.CredentialSourceOpenAIAPIKey {
			return "test-openai-key", true
		}
		return "", false
	}, nil)

	if _, ok := selection.Runner.(*agent.GoAgentRunner); !ok {
		t.Fatalf("runner = %T, want *agent.GoAgentRunner; selection=%+v", selection.Runner, selection)
	}
	if selection.Provider != "custom" || selection.Model != "custom/deepseek-chat:high" || !reflect.DeepEqual(selection.ToolNames, fixedBuildToolNames()) {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestProductionAgentRunnerFactoryBuildsRealGoAgentRunnerForOpenCodeGo(t *testing.T) {
	t.Parallel()

	selection := newAgentBuildRunnerFromConfig(t.TempDir(), string(permission.AutonomyRead), DefaultConfig(), func(name string) (string, bool) {
		if name == agent.CredentialSourceOpenCodeAPIKey {
			return "test-opencode-key", true
		}
		return "", false
	}, nil)

	if _, ok := selection.Runner.(*agent.GoAgentRunner); !ok {
		t.Fatalf("runner = %T, want *agent.GoAgentRunner; selection=%+v", selection.Runner, selection)
	}
	if selection.Provider != "opencode-go" || selection.Model != "opencode-go/deepseek-v4-pro:high" || !reflect.DeepEqual(selection.ToolNames, fixedBuildToolNames()) {
		t.Fatalf("selection = %+v", selection)
	}
}

func TestGoAgentBuildToolsRegisterAllFixedBuiltins(t *testing.T) {
	t.Parallel()

	tools, names, err := newGoAgentBuildTools(t.TempDir(), permission.AutonomyRead)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, fixedBuildToolNames()) {
		t.Fatalf("tool names = %#v, want %#v", names, fixedBuildToolNames())
	}
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		got = append(got, tool.Name())
	}
	if !reflect.DeepEqual(got, fixedBuildToolNames()) {
		t.Fatalf("registered tools = %#v", got)
	}
}

func TestBuildAgentInstructionsIncludeContextAndTools(t *testing.T) {
	t.Parallel()

	instructions := buildAgentInstructions("/workspace/project", string(permission.AutonomyWrite), fixedBuildToolNames())
	for _, want := range []string{"Workspace: /workspace/project", "Project: github.com/jgabor/aila", "Project summary:", "Workflow phase: build", "Active capability: build", "Autonomy level: write", "Tool definitions:", "read", "find", "grep", "bash", "fetch", "edit", "write"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("instructions missing %q:\n%s", want, instructions)
		}
	}
}

func fixedBuildToolNames() []string {
	return []string{"read", "find", "grep", "bash", "fetch", "edit", "write"}
}

func TestOpenAICompatibleBaseURLDefaultsOpenCodeGo(t *testing.T) {
	t.Parallel()

	if got := openAICompatibleBaseURL("opencode-go", ""); got != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("opencode-go base URL = %q", got)
	}
	if got := openAICompatibleBaseURL("opencode-go", " https://proxy.invalid/v1 "); got != "https://proxy.invalid/v1" {
		t.Fatalf("configured opencode-go base URL = %q", got)
	}
}

func TestProductionAgentRunnerFactoryReportsMissingCredentialWithoutFakeFallback(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.LLM.Model = mustParseTestModelRef(t, "openai/gpt-4.1")
	selection := newAgentBuildRunnerFromConfig(t.TempDir(), string(permission.AutonomyRead), config, func(string) (string, bool) { return "", false }, nil)
	unavailable, ok := selection.Runner.(agent.UnavailableRunner)
	if !ok {
		t.Fatalf("runner = %T, want agent.UnavailableRunner", selection.Runner)
	}
	if unavailable.Failure.Code != string(agent.FailureProviderAuth) || !strings.Contains(unavailable.Failure.Message, agent.CredentialSourceOpenAIAPIKey) {
		t.Fatalf("unavailable failure = %+v", unavailable.Failure)
	}
	stream, err := selection.Runner.Stream(context.Background(), agent.RunRequest{Prompt: "hello", Provider: selection.Provider, Model: selection.Model})
	if err != nil {
		t.Fatal(err)
	}
	event := <-stream
	if event.Kind != agent.EventError || event.Error.Code != string(agent.FailureProviderAuth) {
		t.Fatalf("event = %+v", event)
	}
}

func TestProductionAgentRunnerFactoryReportsUnsupportedAdapterWithoutFakeFallback(t *testing.T) {
	t.Parallel()

	config := DefaultConfig()
	config.LLM.Model = mustParseTestModelRef(t, "opencode-zen/zen-pro")
	selection := newAgentBuildRunnerFromConfig(t.TempDir(), string(permission.AutonomyRead), config, func(name string) (string, bool) {
		if name == agent.CredentialSourceOpenCodeAPIKey {
			return "test-opencode-key", true
		}
		return "", false
	}, nil)
	unavailable, ok := selection.Runner.(agent.UnavailableRunner)
	if !ok {
		t.Fatalf("runner = %T, want agent.UnavailableRunner", selection.Runner)
	}
	if unavailable.Failure.Code != string(agent.FailureModelUnavailable) || !strings.Contains(unavailable.Failure.Message, "production go-agent model adapter") {
		t.Fatalf("unavailable failure = %+v", unavailable.Failure)
	}
}

func TestInputRunnerUsesFakeAgentOnlyWhenExplicitlyRequested(t *testing.T) {
	t.Setenv("AILA_AGENT_RUNNER", "fake")

	runner := newInputRunnerWithAgentBuildContext(t.Context(), t.TempDir(), string(permission.AutonomyRead))
	if _, ok := runner.agent.runner.(agent.FakeBuildRunner); !ok {
		t.Fatalf("runner = %T, want agent.FakeBuildRunner", runner.agent.runner)
	}
}
