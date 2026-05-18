# TODO

## ⇶ Critical

Everything needed to get the interactive agent loop producing real work. Each
slice is gated on the slices before it.

### P0 — system prompt + full tool set

The model receives raw user text with no instructions, no tool definitions, and
no project context. Without this, no other wiring matters.

- [x] [feat:0.0.1] Assemble and send a system prompt through go-agent: wire
      `context.Build()` into prompt assembly before `agent.Runner.Stream()`, set
      `goagent.Agent.Instructions` (or `RunRequest.Instructions`) with tool
      definitions, behavioral instructions, workspace path, project summary from
      `.aila/`, current workflow phase, active capability, and autonomy boundary.
- [x] [feat:0.0.1] Register all 7 primitive tools with go-agent in
      `newGoAgentBuildTools`: wire `bash`, `grep`, `find`, `edit`, and `fetch`
      alongside the existing `read` and `write`, each with a complete
      `goagent.ToolDefinition` including schema, safety flags, and a function
      adapter over the already-implemented `internal/tools/` primitives.
- [x] [feat:0.0.1] Feed go-agent tool-call events back through `runtime.Update`
      instead of hand-processing them in `submitAgentPrompt`: the runtime already
      owns `AgentToolRequested` → tool-proposed messages → effect dispatch → result
      messages; wire the stream into that loop so every tool call goes through
      permission checks, history recording, and undo metadata.

### P1 — continuation and multi-turn conversations

- [x] [feat:0.0.2] Raise `MaxSteps` from the hardcoded 4 to an internally
      adjustable default and handle continuation from where the agent left off
      instead of stopping the stream.
- [x] [feat:0.0.2] Pass conversation history to the model using
      `goagent.Session` / `goagent.SessionStore` or by injecting prior turns into
      the system prompt so each run sees the full transcript instead of starting
      fresh.
- [ ] [feat:0.0.2] Handle `AgentTurnCompleted` gracefully: allow the user to
      continue the conversation after the agent stops, queue user messages while
      the agent is running, and support interrupt → cancel the active stream.

### P2 — capabilities call the model

All 12 capabilities return deterministic template output. They need to invoke
`agent.Runner` with capability-specific context.

- [ ] [feat:0.0.2] Wire `agent.Runner` into capability adapters so each
      `Capability.Run()` constructs a model prompt with capability instructions,
      project context, and relevant artifacts, then streams the agent response
      back as capability output with real exit payloads.
- [ ] [feat:0.0.2] Start with the four highest-impact capabilities: ≡ plan
      (model-generated work breakdown), ⧉ build (core implementation loop),
      ⛶ audit (model-driven code review), and ⌂ brief (status from current
      session and artifacts).

---

## ⇉ Degraded

These are viable to defer but block a polished dogfooding experience.

### P3 — workflow FSM drives agent behavior

- [ ] [feat:0.0.3] Phase-appropriate tool selection: expose only the tools the
      current workflow phase permits (IDLE: brief only, BUILD: all + capabilities,
      etc.), matching the tool registry contract from
      `docs/workflow-architecture.md`.
- [ ] [feat:0.0.3] Phase-appropriate system prompt: include the current phase,
      valid transitions, and exit signal expectations in the instructions sent to
      the model.
- [ ] [feat:0.0.3] Real phase transitions: replace `buildAgentEvidenceTurn`'s
      hardcoded `PhaseBuild` with the actual phase from the FSM, and route
      capability exit signals through the FSM to determine the next phase.

### P4 — git awareness and interactive completeness

- [ ] [feat:0.0.4] Expose safe git inspection commands (status, diff, log,
      branch) as allowed `bash` tool operations registered with go-agent, and
      render real git state in the TUI footer instead of static placeholders.
- [ ] [feat:0.0.4] Wire `!command` and `!!command` shell prefix through the
      runtime's bash tool effect dispatch: show output in chat and optionally feed
      summarized output to the agent for reasoning.
- [ ] [feat:0.0.4] Wire file reference (`@`) to actual project file search
      using `tools.ExecuteFind` or a real file index instead of the fake
      `discoverPromptFileReferences` data.

### P5 — session persistence during interactive runs

- [ ] [feat:0.0.5] Persist session snapshots to `.aila/sessions/` after each
      agent turn, write events to the append-only event log, and record undo
      metadata for file mutations.
- [ ] [feat:0.0.5] Wire `/continue` to restore full session state including
      agent conversation context, not just the current snapshot metadata.

---

## → Normal

### P6 — utility worker scheduling

- [ ] [feat:0.0.6] Schedule utility work when the primary agent is idle:
      context prefetch and ranking, stale context checks, summary refresh, and
      next-action suggestions.
- [ ] [feat:0.0.6] Surface utility results in the TUI without hiding them: show
      prepared context, stale warnings, and suggestions as visible hints.

### P7 — go-agent policy, retry, and observability

- [ ] [feat:0.0.6] Configure `goagent.Policy` for rate limiting awareness,
      token budget management, and tool call limits.
- [ ] [feat:0.0.6] Wire `goagent.SessionStore` so go-agent manages conversation
      turn history natively instead of Aila reconstructing it each run.
- [ ] [feat:0.0.6] Set `Retry` policy for transient model and tool failures,
      and add `EventSinks` for structured observability.

### P8 — clean up fake events from main paths

Once real agent integration is stable, remove the fake/deterministic paths that
leak into production code.

- [ ] [chore:0.0.6] Remove `commandOutput` stubs that return `deferred` status
      text for `run`, `continue`, `config`, and `help` when the real handlers are
      wired and confirmed through PTY smoke tests.
- [ ] [chore:0.0.6] Remove `"deterministic read-only run; provider model
execution deferred"` caveat and fake file inspection from
      `noninteractive_run.go` once non-interactive runs use the real agent runner.
- [ ] [chore:0.0.6] Remove the `AILA_FAKE_PROMPT_ECHO`,
      `AILA_FAKE_APPROVAL_WRITE`, `AILA_FAKE_APPROVAL_PROPOSAL`, and
      `AILA_AGENT_RUNNER=fake` environment-variable entry points from
      `internal/app/app.go` and `internal/app/shutdown.go`; keep fake runners
      accessible only through test packages.
- [ ] [chore:0.0.6] Audit `internal/app/` for remaining `Fake*` type usage in
      non-test paths and route through the real agent runner exclusively.

### P9 — optimize the test suite

The test suite is comprehensive but the PTY smoke tests dominate runtime.

- [ ] [perf:0.0.6] Profile PTY smoke test duration: identify the slowest tests
      with `go test -timeout 300s -count=1 ./cmd/aila/...` and classify them by
      root cause (Bubble Tea startup, tmux setup, I/O polling, etc.).
- [ ] [perf:0.0.6] Reduce Bubble Tea startup overhead across PTY tests: share
      one longer-running PTY session across multiple test cases where isolation
      semantics allow, or use a session pool with pre-warmed terminals.
- [ ] [perf:0.0.6] Move purely logical assertions out of PTY tests into
      `internal/app/` and `internal/tui/` unit tests so PTY tests only verify
      actual terminal I/O behavior; fixture and semantic-snapshot tests should
      carry the bulk of correctness coverage.
- [ ] [perf:0.0.6] Add a `go test -short` flag gate to skip PTY tests in
      `cmd/aila/` so `mage check` and CI pre-merge gates can run fast (unit tests
      only) while PTY tests run in a dedicated parallel job.
- [ ] [perf:0.0.6] Consolidate overlapping PTY scenarios: group tests by
      session type (startup, run, continue, commands) and deduplicate repeated
      terminal setup with shared fixtures.

### P10 — 0.1.0 boundary

Cleared after P0–P9. Cut the first dogfoodable release.

---

## Resolved

- [feat:0.0.2] Interactive agent runs now use an internal step-budget default,
  preserve step-limit pauses as resumable state, and pass in-process prior
  prompt/tool/assistant context through go-agent Session for continuation and
  normal next turns.
- [feat:0.0.1] P0 system prompt, fixed built-in go-agent tools, and generic
  registered tool dispatch are wired for dogfood-capable agent turns.
