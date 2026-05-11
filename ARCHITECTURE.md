# Aila Architecture

This document defines the durable implementation architecture for Aila. It is meant to be referenced while building the project so that the user interface, workflow engine, agent runtime, tools, permissions, persistence, and background utility work all follow one consistent pattern.

Aila is intentionally a fixed-product coding agent. It is not a plugin host, workflow marketplace, MCP shell, or generic agent framework. The architecture should therefore favor explicit Go types, deterministic state transitions, observable events, and narrow integration boundaries over dynamic loading or framework-style extensibility.

## Architectural thesis

Aila is a **statechart-MVU application with split planes and explicit effects**.

In practical terms:

> Aila receives typed messages, updates typed state, emits explicit effects, interprets those effects at the boundary of the system, and feeds the results back as more messages.

The workflow finite-state machine owns lifecycle transitions. The policy layer recommends actions. Capability adapters perform built-in workflows. `go-agent` executes model/tool turns. The permission gate guards world-touching effects. The state store records durable project memory, history, artifacts, and provenance. The terminal UI renders state and collects input.

The central rule is:

> No subsystem mutates the outside world directly. Anything that touches files, shell, network, git, model providers, project artifacts, session history, or persistent context must be represented as an explicit effect and handled by the appropriate effect interpreter.

## Source authority

This document describes implementation patterns and boundaries. More specific product and protocol documents still apply:

| Document                        | Role                                                                                              |
| ------------------------------- | ------------------------------------------------------------------------------------------------- |
| `README.md`                     | Product intent, UX promise, commands, tools, configuration, and user-facing behavior              |
| `docs/workflow-architecture.md` | Detailed workflow protocol, FSM transitions, capability mapping, and testable protocol invariants |
| `go-agent` documentation        | Embedded agent runtime primitives and host-owned policy boundaries                                |

When this document conflicts with the user-facing product behavior in `README.md`, update one or both documents deliberately. When this document conflicts with the workflow transition rules in `docs/workflow-architecture.md`, the workflow reference owns the protocol detail.

## Non-goals

Aila should avoid architecture that implies a more generic product than intended.

Aila does not need:

- runtime plugins, extensions, hooks, or user-defined capabilities
- a workflow DSL
- a generic graph execution engine
- an MCP-first control model
- a model marketplace architecture
- dynamically loaded tools
- prompt-only phase routing
- hidden background mutation
- a hosted control plane

Aila may use normal Go interfaces internally where they improve boundaries, testing, and substitution, but those interfaces should not become a public extension system by accident.

## Core pattern: MVU with explicit effects

The primary implementation pattern is MVU, also known as The Elm Architecture:

```text
Message/Event
    ↓
Update state
    ↓
Emit effects
    ↓
Effect handlers perform IO
    ↓
Effect results become new messages/events
```

Every major layer should follow this shape where practical:

```go
type Model struct {
    // durable and runtime state owned by this layer
}

type Msg interface {
    msg()
}

type Effect interface {
    effect()
}

func Update(model Model, msg Msg) (Model, []Effect) {
    // Prefer deterministic state changes here.
    // Do not perform IO here.
}
```

Effects are interpreted outside the update function:

```go
type EffectHandler interface {
    Handle(ctx context.Context, effect Effect) <-chan Msg
}
```

The exact Go names may vary by package, but the rule should remain stable:

- updates decide
- effects do
- effect results report back as messages
- durable state changes are observable and persisted deliberately

### Why this pattern fits Aila

Aila combines a terminal UI, model streams, tool execution, approvals, queueing, interruptions, background utility work, session persistence, and workflow routing. These concerns become hard to reason about if each subsystem mutates shared state directly.

MVU gives Aila:

- deterministic state transitions
- testable routing and permission behavior
- clean Bubble Tea integration
- explicit cancellation points
- replayable runtime events
- consistent observability
- safe concurrency boundaries
- a natural adapter shape for `go-agent` streams

## Split planes

Aila should be organized as separate planes. Planes describe ownership boundaries. MVU describes how those planes communicate.

```text
┌─────────────────────────────────────────────┐
│ Presentation Plane                           │
│ Bubble Tea TUI                               │
└───────────────────────┬─────────────────────┘
                        │ UI messages / view models
┌───────────────────────▼─────────────────────┐
│ Application Control Plane                    │
│ Runtime event loop, session controller       │
└───────────────────────┬─────────────────────┘
                        │ domain messages / effects
┌───────────────────────▼─────────────────────┐
│ Workflow Plane                               │
│ FSM, statechart, policy, capability routing  │
└───────────────────────┬─────────────────────┘
                        │ capability effects
┌───────────────────────▼─────────────────────┐
│ Agent Execution Plane                        │
│ go-agent adapter, model streams              │
└───────────────────────┬─────────────────────┘
                        │ tool effects
┌───────────────────────▼─────────────────────┐
│ Tool and Permission Plane                    │
│ tools, autonomy, approvals, undo metadata    │
└───────────────────────┬─────────────────────┘
                        │ store effects
┌───────────────────────▼─────────────────────┐
│ State and Memory Plane                       │
│ .aila store, artifacts, context, provenance  │
└─────────────────────────────────────────────┘
```

### Plane responsibilities

| Plane               | Owns                                                                                       | Must not own                                                         |
| ------------------- | ------------------------------------------------------------------------------------------ | -------------------------------------------------------------------- |
| Presentation        | rendering, input collection, keybindings, terminal layout, visible progress                | workflow transitions, model prompts, file mutation, permissions      |
| Application control | session loop, event routing, queues, cancellation, active runs, effect dispatch            | TUI layout, raw filesystem details, provider-specific model behavior |
| Workflow            | phases, transition validation, exit-signal routing, policy selection, capability ownership | shell execution, file writes, TUI rendering, persistence format      |
| Agent execution     | `go-agent` runner adapter, model stream mapping, run lifecycle events                      | Aila workflow authority, autonomy policy, artifact ownership         |
| Tool and permission | primitive tools, operation classification, approval gates, undo records                    | phase transitions, prompt routing, UI layout                         |
| State and memory    | `.aila/`, sessions, snapshots, artifacts, source refs, compacted context, provenance       | capability decisions, tool execution, rendering                      |
| Utility             | idle-only context prep, stale checks, compaction, summary refresh, suggestions             | file writes, git mutations, phase transitions, final judgment        |

## Workflow model

The workflow lifecycle is a finite-state machine with six top-level phases:

```text
IDLE
ENVISION
DELIBERATE
PLAN
BUILD
AUDIT
```

The workflow FSM owns phase transitions. No tool, TUI component, model response, capability, utility job, or permission decision may directly mutate the current phase.

A phase transition must go through the workflow update path:

```text
Capability exit payload
    ↓
FSM validates signal and recommended successor
    ↓
Runtime records transition decision
    ↓
State store persists transition and metadata
    ↓
TUI renders new phase
```

### Exit signals

Capabilities communicate lifecycle results using exit signals:

| Signal     | Meaning                        | Workflow behavior                                                         |
| ---------- | ------------------------------ | ------------------------------------------------------------------------- |
| `complete` | work completed successfully    | validate recommended successor or stop if terminal behavior allows        |
| `flagged`  | work completed with caveats    | same as `complete`, but preserve concerns visibly and in context          |
| `waiting`  | missing input or decision      | do not transition; pause current phase and ask for direction              |
| `stuck`    | hard blocker prevents progress | recover through a valid successor or park in `IDLE` with blocker metadata |

`waiting`, `stuck`, approval-pending, tool-running, compacting, and interrupted are not workflow phases. They are runtime states around the workflow.

## Runtime statechart

The protocol FSM is intentionally small. The application runtime needs additional state for activity, queueing, approvals, and background work.

Model those concerns as a lightweight statechart around the FSM rather than by inventing extra workflow phases.

Examples of runtime dimensions:

```text
agent:      idle | running | canceling
input:      idle | queued | interrupting
approval:   none | pending | resolved
store:      clean | dirty | persisting
utility:    idle | preparing | compacting | refreshing
context:    fresh | stale | rebuilding
```

These dimensions can be active at the same time. For example, Aila may be in workflow phase `BUILD`, with an agent run active, one user message queued, a shell command approval pending, and the utility worker idle.

Do not collapse that into a fake workflow state such as `BUILD_WAITING_FOR_APPROVAL`.

## Application control plane

The application control plane is the central event loop. It owns the runtime model and dispatches effects.

A representative model:

```go
type AppModel struct {
    SessionID        string
    Workflow         WorkflowModel
    Queue            MessageQueue
    ActiveRun        *RunState
    PendingApprovals map[string]ApprovalRequest
    Utility          UtilityState
    Store            StoreState
    View             ViewModel
}
```

Representative messages:

```go
type UserPromptReceived struct {
    Text string
}

type SlashCommandReceived struct {
    Name string
    Args []string
}

type AgentEventReceived struct {
    RunID string
    Event AgentEvent
}

type ToolCallRequested struct {
    RunID string
    Call ToolCall
}

type CapabilityExited struct {
    RunID   string
    Payload ExitPayload
}

type ApprovalResolved struct {
    ApprovalID string
    Decision   ApprovalDecision
}

type StorePersisted struct {
    Revision string
}

type UtilityResultReceived struct {
    JobID  string
    Result UtilityResult
}
```

Representative effects:

```go
type RoutePromptEffect struct { /* ... */ }
type RunCapabilityEffect struct { /* ... */ }
type RunAgentEffect struct { /* ... */ }
type RunToolEffect struct { /* ... */ }
type RequestApprovalEffect struct { /* ... */ }
type PersistSessionEffect struct { /* ... */ }
type CancelRunEffect struct { /* ... */ }
type StartUtilityJobEffect struct { /* ... */ }
type CompactContextEffect struct { /* ... */ }
```

The control plane should be the only place where unrelated subsystems are coordinated. It may call workflow, policy, state, and effect-dispatch functions, but it should not contain low-level tool implementation or TUI layout logic.

## Policy layer

The policy layer selects what should happen inside the current workflow context. It recommends; it does not transition.

Policy may consider:

- explicit slash commands
- natural-language intent
- current workflow phase
- active plan
- project artifacts
- autonomy level
- available tools
- prior blockers
- queued user steering
- utility suggestions

Policy output should be explicit:

```go
type PolicyDecision struct {
    CapabilityName string
    OwningPhase    Phase
    CrossCutting   bool
    Reason         string
    Confidence     float64
}
```

The policy layer may be behavior-tree-shaped internally:

```text
explicit slash route?
    yes → direct capability candidate
high-confidence natural-language route?
    yes → candidate capability
active phase can continue?
    yes → current phase policy
otherwise
    → brief/status orientation
```

But a formal behavior-tree framework is not required unless the code clearly benefits from it. Prefer small, testable Go selectors over generic machinery.

### Policy rules

- Cross-cutting capabilities do not transition workflow phases by themselves.
- State-primary capabilities may recommend successors through exit payloads.
- The FSM validates successors.
- Policy may recommend a path to a phase; it may not mutate the phase directly.
- Slash commands and keyboard shortcuts should route through the same command handlers.
- Borderline or low-confidence routing may emit `waiting` rather than guessing.

## Capability adapters

Capabilities are fixed, built-in workflows. They are not loaded dynamically at runtime.

A capability has one owning phase, except for explicitly cross-cutting capabilities.

```go
type Capability interface {
    Name() string
    OwningPhase() Phase
    Run(context.Context, CapabilityRequest) (ExitPayload, error)
}
```

Each invocation emits exactly one exit payload:

```go
type ExitPayload struct {
    Signal               ExitSignal
    Summary              string
    Concerns             []string
    Blocker              string
    NeededInput          string
    Attempted            []string
    RecommendedSuccessor Phase
    ArtifactRefs         []ArtifactRef
    SourceRefs           []SourceRef
}
```

Complex capabilities may use internal state machines, subagents, retries, and local update/effect loops. That internal complexity must not leak into the top-level workflow FSM.

### Capability rules

- A capability may call the model through the agent execution plane.
- A capability may request tool execution through effects.
- A capability may read and write artifacts only through the artifact resolver and store effects.
- A capability may not bypass the permission gate for mutations.
- A capability may not directly change the current workflow phase.
- A capability that needs input returns `waiting` with `NeededInput`.
- A capability that cannot proceed returns `stuck` with `Blocker` and `Attempted` populated.

## Agent execution plane

Aila embeds `go-agent` as the model/tool runtime. Aila owns product policy, persistence, workflow, permissions, prompt assembly, and UI.

Use an Aila-owned adapter around `go-agent`:

```go
type AgentRunner interface {
    Stream(ctx context.Context, req AgentRunRequest) (<-chan AgentRunEvent, error)
}
```

The adapter maps Aila requests to `go-agent` requests and maps `go-agent` stream events back to Aila messages.

```text
RunAgentEffect
    ↓
Aila go-agent adapter
    ↓
go-agent Runner.Stream(...)
    ↓
go-agent events
    ↓
Aila AgentEventReceived messages
    ↓
Application update
```

### Event mapping

| Runtime observation           | Aila message             |
| ----------------------------- | ------------------------ |
| model response starts         | `AgentResponseStarted`   |
| text delta arrives            | `AgentTextDeltaReceived` |
| tool call requested           | `ToolCallRequested`      |
| tool result observed          | `ToolResultReceived`     |
| retry considered or attempted | `AgentRetryObserved`     |
| model run stops               | `AgentStopped`           |
| runtime error occurs          | `AgentErrored`           |

The TUI should consume Aila view models and Aila events, not raw provider or `go-agent` internals.

### Agent execution rules

- `go-agent` runs model/tool loops; it does not own Aila workflow.
- Aila supplies the tool registry appropriate to the current phase and capability.
- Aila supplies host-owned policy hooks for approval, limits, validation, and cancellation.
- Aila persists session and project state through its own state plane.
- Aila records enough event data to reconstruct surprising behavior.

## Tool and permission plane

Primitive tools are fixed and built in:

```text
read
edit
write
bash
grep
find
fetch
```

Tool execution must be represented as an effect. The effect is classified, checked against autonomy policy, optionally approved by the user, executed, recorded, and returned as a message.

```text
Tool call proposal
    ↓
Classify operation
    ↓
Autonomy policy decision
    ↓
Approval prompt if needed
    ↓
Recheck immediately before mutation
    ↓
Execute tool
    ↓
Record history and undo metadata
    ↓
Return tool result message
```

### Autonomy levels

Autonomy controls pace, not workflow shape.

| Level   | Meaning                                                       |
| ------- | ------------------------------------------------------------- |
| `off`   | every tool call requires approval                             |
| `read`  | safe read-only operations may proceed automatically           |
| `write` | read and permitted write operations may proceed automatically |
| `yolo`  | all tool calls are granted by default                         |

Even in `yolo`, Aila should record the automatic decision. This keeps audit history consistent.

### Operation proposal

Permission decisions should be tied to exact proposal data:

```go
type ProposedOperation struct {
    Kind           OperationKind
    Tool           string
    TargetPath     string
    TargetVersion  string
    Command        []string
    WorkingDir     string
    ExpectedEffect string
    DiffPreview    string
    Reversible     bool
    RunID          string
    Capability     string
}
```

### Permission rules

- Recheck permission immediately before mutation.
- Denied operations return results to the active capability; they do not silently disappear.
- File edits and writes must produce undo metadata where possible.
- Shell commands must have visible working directory, command text, and expected effect.
- Tool results should include compressed summaries plus exact source references when correctness depends on details.
- Tools must not decide workflow transitions.

## State and memory plane

Aila owns project state under `.aila/`. This is project-visible state, not disposable cache.

The state plane should support:

- session snapshots
- append-only event logs for durable behavior
- model and tool run history
- approval records
- edit/write undo data
- artifact storage
- compacted context
- source provenance
- stale-context checks
- resumable work state

A representative layout:

```text
.aila/
  project.toml
  sessions/
    current.json
    <session-id>/
      snapshot.json
      events.jsonl
      messages.jsonl
      tool-calls.jsonl
      approvals.jsonl
      edits.jsonl
      undo/
  context/
    compacted.md
    sources.jsonl
    stale-checks.jsonl
  artifacts/
    vision.md
    decisions.md
    plan.md
    progress.md
    health.md
  indexes/
    files.json
    source-provenance.jsonl
```

The exact layout may evolve. The stable requirement is that logical state access goes through a resolver or store interface rather than hardcoded paths spread across the codebase.

### Artifact resolver

All artifact reads and writes should go through an artifact resolver:

```go
type ArtifactResolver interface {
    Resolve(ctx context.Context, logicalName string) (ArtifactPath, error)
}
```

The resolver should:

- prefer Aila-native `.aila/` paths
- return provenance with resolved paths
- understand logical artifact names
- reject writes to artifacts not owned by the invoking capability
- support migration/import behavior deliberately when needed

### Event log versus snapshots

Use event logs where replay and trust matter. Use snapshots where full replay would be expensive or unnecessary.

| Area                            | Recommended persistence                                     |
| ------------------------------- | ----------------------------------------------------------- |
| workflow transitions            | event log plus session snapshot                             |
| tool calls                      | event log                                                   |
| approvals                       | event log                                                   |
| file edits/writes               | event log plus undo metadata                                |
| shell commands                  | event log with summarized and exact important output        |
| model runs                      | event log with provider-safe metadata and source references |
| compacted context               | snapshot plus source references                             |
| TUI cursor and transient layout | not durable                                                 |

## Context system

Context is a first-class product feature. Aila should not pretend every run starts from nothing.

The context system owns:

- prompt assembly
- stable context prefixing
- project memory inclusion
- relevant file and artifact selection
- source references
- compaction
- stale checks
- summary refresh
- token/context budget management

Context builders should return structured context, not just strings:

```go
type BuiltContext struct {
    Blocks     []ContextBlock
    SourceRefs []SourceRef
    Budget     ContextBudget
    Warnings   []string
}
```

Context rules:

- Preserve exact paths, diffs, commands, errors, and user constraints when correctness may depend on them.
- Compacted summaries must carry source references.
- Utility-generated context may suggest and prepare, but foreground capabilities decide how to use it.
- Stale context should be surfaced rather than silently trusted.

## Utility model plane

The utility model prepares work while the primary model is idle. It does not decide consequential behavior.

Allowed utility jobs:

- context prefetch and ranking
- stale-context checks
- safe compaction
- summary refresh
- next-action suggestions with evidence

Forbidden utility actions:

- file writes
- shell commands with side effects
- git mutations
- hidden artifact changes
- permission approvals
- workflow transitions
- final judgment on consequential tradeoffs

Utility jobs should follow the same message/effect pattern:

```text
StartUtilityJobEffect
    ↓
Utility worker performs allowed work
    ↓
UtilityResultReceived
    ↓
Runtime folds result into visible state or prepared context
```

Utility suggestions should be visible as suggestions, not silently executed plans.

## Presentation plane

The TUI uses Bubble Tea's model/update/view shape. It should remain a presentation layer.

The TUI owns:

- input box behavior
- slash command UI
- keybindings
- chat rendering
- diff views
- history views
- approval prompts
- status/header/footer display
- progress indicators

The TUI must not own:

- workflow transitions
- capability selection
- tool execution
- permission classification
- session persistence
- model prompt construction

TUI messages should be converted into application messages:

```go
type UserSubmittedPrompt struct {
    Text string
}

type UserPressedInterrupt struct{}

type UserApprovedOperation struct {
    ApprovalID string
}

type UserDeniedOperation struct {
    ApprovalID string
}

type UserSelectedSlashCommand struct {
    Name string
    Args []string
}
```

The TUI may render `BUILD -> AUDIT`. It may not decide that `BUILD -> AUDIT` should happen.

## Commands and shortcuts

Slash commands and keyboard shortcuts are input aliases. They should resolve to the same command handler.

```text
/user slash command
keyboard shortcut
        ↓
CommandReceived message
        ↓
Command router
        ↓
Application effects/messages
```

Commands are not primitive tools and are not workflow phases. For example, `/review`, `/compact`, `/status`, and `/model` are runtime commands that may invoke capabilities, utility jobs, state reads, or UI flows depending on context.

## Subagents and orchestration

Subagents are supervised concurrent work, not a separate architecture.

Represent subagent work as effects:

```go
type SpawnSubagentEffect struct {
    ParentRunID string
    Purpose     string
    Input       string
    Tools       []ToolName
    Budget      Budget
}
```

Subagent results return as messages:

```go
type SubagentCompleted struct {
    ParentRunID string
    Result      SubagentResult
}
```

`orchestrate` is a BUILD-phase conductor. It may dispatch subagents, update plan status, request audits, evaluate results, and enforce retry budgets. It must not silently implement source changes itself outside the normal tool, permission, and capability paths.

Subagent rules:

- Child work must inherit explicit budget, tool, and permission constraints.
- Child outputs must include source references.
- Parent capabilities decide how to incorporate child results.
- Failed subagents report failure as events; they do not disappear.
- Parallelism should be supervised and cancelable.

## Concurrency and cancellation

Aila should be responsive even while work is active. Concurrency should be explicit and observable.

Use `context.Context` for cancellation across:

- model streams
- tool calls
- shell commands
- utility jobs
- subagents
- persistence operations where appropriate

Runtime rules:

- A user interrupt should cancel active work if safe and persist a checkpoint.
- A queued user message should be visible.
- An interrupting user message should either steer the active run through a defined path or cancel and restart deliberately.
- No active capability should become unobservable.
- Long-running operations should emit progress events where possible.
- Canceled effects should report cancellation as messages.

## Observability

Every surprising behavior should be reconstructable from recorded events and source references.

Record enough information to answer:

- What did the model see?
- Which phase was active?
- Which capability was running?
- Which tools were exposed?
- Which tool was requested?
- What exact operation was proposed?
- Was permission automatic or user-approved?
- What result came back?
- Why did the run stop?
- What changed in the workspace?
- Which source refs support the final answer?

Observability should serve local development, user trust, test replay, and future debugging.

## Package boundary sketch

Package names may evolve, but the boundaries should remain stable.

```text
cmd/aila
  CLI entrypoint, config loading, app startup

internal/app
  composition root and top-level wiring

internal/tui
  Bubble Tea models, views, keybindings, terminal rendering

internal/runtime
  central event loop, AppModel, messages, effects, dispatcher

internal/workflow
  phases, FSM, transition table, exit-signal routing, runtime statechart helpers

internal/policy
  intent routing, state-local selectors, command routing recommendations

internal/capability
  fixed built-in capability adapters

internal/agent
  go-agent adapter, event mapping, model/provider configuration

internal/tools
  read/edit/write/bash/grep/find/fetch implementations, result compression

internal/permission
  autonomy levels, operation classification, approvals

internal/state
  .aila store, sessions, snapshots, event logs, artifact resolver, provenance

internal/context
  context builder, compaction, source refs, stale checks

internal/utility
  utility worker, idle-only jobs, suggestions

internal/history
  undo/redo, edit records, command records, replay helpers
```

Hard boundaries:

- `internal/tui` must not import tool implementations to perform mutations.
- `internal/tools` must not import workflow code to transition phases.
- `internal/capability` must not bypass `internal/state` for artifacts.
- `internal/policy` may use workflow types but must not mutate workflow state directly.
- `internal/agent` must not own Aila persistence or workflow decisions.
- `internal/utility` must not perform workspace mutations.
- `internal/state` must not perform model calls or command execution.

## Testing strategy

The architecture should be tested at the level of invariants, not only examples.

### Workflow tests

- The transition table matches the protocol reference.
- Invalid transitions are rejected.
- `waiting` does not change phase.
- `stuck` preserves blocker metadata.
- `flagged` preserves concerns and routes through the same validation path as `complete`.
- Cross-cutting capabilities do not trigger transitions by themselves.

### Policy tests

- Explicit slash routes resolve to the intended capability.
- Keyboard shortcuts and slash commands hit the same command handler.
- Low-confidence routing asks or falls back instead of guessing.
- Policy recommendations do not mutate workflow state.

### Permission tests

- Each autonomy level allows and blocks the expected operation classes.
- Mutations are rechecked immediately before execution.
- Denied approvals return results to the active capability.
- Automatic `yolo` decisions are still recorded.

### Runtime tests

- Queued messages remain visible and ordered.
- Interrupts cancel or steer active runs through defined paths.
- Canceled effects report cancellation.
- No active capability can become unobservable.
- go-agent events are mapped into Aila messages consistently.

### State tests

- Artifact writes require ownership.
- Stored events can reconstruct run history.
- Source refs survive compaction.
- Undo metadata is produced for supported edits/writes.
- Session snapshots can resume work without losing blockers, plans, or concerns.

### Utility tests

- Utility jobs run only when allowed by runtime state.
- Utility jobs cannot mutate files, git state, or project artifacts.
- Utility suggestions do not transition workflow phases.
- Stale-context results are surfaced explicitly.

## Implementation checklist

When adding or changing a feature, check it against these questions:

1. What message starts this behavior?
2. Which model owns the state being changed?
3. Is the state update deterministic and testable?
4. What effects are emitted?
5. Which effect handler performs IO?
6. How does the result come back as a message?
7. Does any mutation pass through the permission gate?
8. Does any artifact access pass through the resolver/store?
9. Are source references preserved where correctness depends on them?
10. Can the behavior be canceled or interrupted safely?
11. Is the behavior visible in the TUI when active?
12. Is enough history recorded to debug or replay it?
13. Does this accidentally create a plugin or extension surface?
14. Does this accidentally let the model, TUI, or tool layer own workflow transitions?

## Anti-patterns

Avoid these patterns even if they appear convenient in the short term.

### Prompt-owned workflow

Do not let model text directly change workflow phase. The model may recommend; the FSM validates.

```text
bad:  model says "now audit" → phase = AUDIT
good: capability emits recommended successor → FSM validates → runtime persists
```

### Hidden mutation

Do not let background jobs, utility workers, or helper functions modify workspace files or project artifacts without explicit effects and records.

### Fake workflow states

Do not create phases such as `BUILD_WAITING_FOR_APPROVAL` or `AUDIT_RUNNING_TOOL`. Use runtime state around the workflow FSM.

### Generic framework drift

Do not introduce plugin loaders, workflow DSLs, generic graph engines, or dynamic capability schemas unless the product direction changes deliberately.

### Actor model everywhere

Subagents and long-running jobs may behave like supervised workers. The main application should remain a deterministic event loop, not a swarm of uncoordinated actors.

### Event sourcing everything

Persist durable behavior that matters for trust, replay, undo, and debugging. Do not persist every transient TUI detail.

### go-agent as product owner

`go-agent` is the embedded runtime. Aila owns the product behavior, workflow, permissions, state, and UI.

## Final rule

Aila should feel fast and direct to the user, but its internals should be explicit and conservative.

The implementation should repeatedly return to this simple loop:

```text
typed message
    → deterministic update
    → explicit effect
    → guarded IO
    → recorded result
    → typed message
```

That loop is the architecture.
