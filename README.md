<h1 align="center">Aila</h1>
<p align="center">A minimal and uncompromising coding agent made for me.</p>
<p align="center">
  <a href="https://github.com/jgabor/aila/releases/tag/v0.0.0"><img alt="Version" src="https://img.shields.io/badge/version-0.0.0-black?style=flat-square" /></a>
  <a href="https://go.dev/"><img alt="Go" src="https://img.shields.io/badge/go-1.26%2B-blue?style=flat-square" /></a>
</p>

```text
[screenshot goes here]
```

Aila is a minimal and highly opinionated terminal coding agent, built on top of [go-agent](https://github.com/jgabor/go-agent) and [bubbletea](https://charm.land). It incorporates the workflow I have developed in [agentera](https://github.com/jgabor/agentera), but adapted into a finite-state machine in the form of a state diagram.

Aila does not, and will not, support extensions, plugins, MCP servers, or any other type of customizations. It comes with a fixed system prompt, fixed set of tools, and a workflow that works for me. Feel free to use Aila if it suits you, but if not, there are [many](https://github.com/charmbracelet/crush) [other](https://github.com/0xku/kon) [coding](https://github.com/anomalyco/opencode) [agents](https://github.com/earendil-works/pi/tree/main/packages/coding-agent) to choose from.

## Table of contents

- [Quick start](#quick-start)
- [Philosophy](#philosophy)
- [Configuration](#configuration)
- [Workflow](#workflow)
- [Built-in tools and capabilities](#built-in-tools-and-capabilities)
- [User interface](#user-interface)
- [Data and privacy](#data-and-privacy)

---

## Quick start

### Install

```bash
go install github.com/jgabor/aila/cmd/aila@latest
```

### Usage

```text
usage: aila [command] [args]

Commands:
  run             	Run non-interactively
  continue          Continue a saved session, defaults to previous
  config            Opens configuration UI, `--all` prints full config
  models			Show available models, filter with `aila models *model*`
  help              Show command help

Options:
  --model, -m       Model to use, formatted as provider/model[:reasoning]
  --continue, -c    Continue the most recent session
  --version, -V     Show Aila version
```

Examples:

```bash
aila run explain the architecture of this repo
git diff | aila run review this change
```

### Build from source

```bash
git clone https://github.com/jgabor/aila
cd aila
mage build # `mage install` for installing to $GOBIN
```

> [!WARNING]
> Aila targets Linux, as that is what I run. It _should_ technically work under macOS and Windows/WSL, but no guarantees are made.

## Philosophy

Aila is intentionally opinionated, based on a set of non-negotiables. Anything missing is built into Aila, and never pushed into hooks, modes, plugins, skills, or any other type of customization.

- **Low latency** without sacrificing a rich user experience
- **Cache optimized** by combining intelligent compaction with stable context-prefixing
- **Mobile friendly** with responsiveness to 80 columns for those late night vibe sessions
- **Built-in tools** that just work and transparently compress results
- **Subagents** for aggressive parallelism, wider exploration, and deeper reasoning
- **YOLO** by default, but with configurable levels of autonomy
- **Single mode** to avoid context switching between planning and building

**tl;dr:** Aila will always be immediately productive out of the box.

## Configuration

Aila stores user configuration at:

```text
$XDG_CONFIG_HOME/aila/config.toml
```

If `XDG_CONFIG_HOME` is unset, that means `~/.config/aila/config.toml`.

Project state lives under `.aila/` in the workspace, and global state lives at `$XDG_DATA_HOME/aila/` or `~/.local/share/aila/`.

`.aila/` is project state, not throwaway cache. Commit it. It keeps the project memory, saved work state, compacted context, and session metadata that lets Aila resume without pretending every run starts from nothing.

The default config is created on first run and intentionally simple.

```toml
[llm]
model = "opencode-go/deepseek-v4-pro:high" # <provider>/<model>[:reasoning]

[llm.utility]
model = "opencode-go/deepseek-v4-flash:max"

[autonomy]
level = "yolo"
```

### Config reference

| Key                 | Default                             | Meaning                                           |
| ------------------- | ----------------------------------- | ------------------------------------------------- |
| `llm.model`         | `opencode-go/deepseek-v4-pro:high`  | Primary model as `<provider>/<model>[:reasoning]` |
| `llm.base_url`      | unset                               | OpenAI-compatible endpoint for `custom`           |
| `llm.utility.model` | `opencode-go/deepseek-v4-flash:max` | Smaller model used for background work            |
| `autonomy.level`    | `yolo`                              | One of `off`, `read`, `write`, or `yolo`          |

Model names may include a reasoning suffix, such as `:high` or `:max`. Use `aila models` as the source of truth for what a provider supports.

For local models, configure an OpenAI-compatible endpoint by setting `custom` as the provider:

```toml
[llm]
model = "custom/qwen3.6:high" # <provider>/<model>[:reasoning]
base_url = "http://localhost:11434/v1"
```

### Providers

#### API

API providers use API keys or OpenAI-compatible local endpoints.

- `custom`: OpenAI-compatible API (`OPENAI_API_KEY`)
- `openai`: OpenAI Realtime API (`OPENAI_API_KEY`)
- `opencode-zen`: OpenCode Zen (`OPENCODE_API_KEY`)

#### Plans

Plan providers use device code authentication.

- `codex`: OpenAI Codex
- `copilot`: GitHub Copilot
- `opencode-go`: OpenCode Go
- `xiaomi-plan`: Xiaomi Token Plan
- `zai-plan`: Z.Ai Coding Plan

### Autonomy level

| Level   | Meaning                                                                 |
| ------- | ----------------------------------------------------------------------- |
| `off`   | Every tool call must be approved                                        |
| `read`  | `read`, `find`, `fetch`, `grep`, `bash[git status, git diff, pwd, ls]`  |
| `write` | Everything above plus `edit`, `write`, and file-mutating shell commands |
| `yolo`  | All tool calls are granted. This is the default.                        |

## Workflow

Aila uses a small state machine, but you should not have to think about it while using the app.

The idea is simple: Aila keeps track of whether the work is currently about shaping the goal, deciding what to do, making a plan, building, or checking the result. That gives it enough structure to suggest the next useful move without turning the chat into a ceremony.

The workflow is based on [agentera](https://github.com/jgabor/agentera), adapted into Aila as six built-in states:

| State          | What it means                                               |
| -------------- | ----------------------------------------------------------- |
| **IDLE**       | Nothing is active yet; orient, resume, or route the request |
| **ENVISION**   | Clarify what the project or feature should become           |
| **DELIBERATE** | Think through a consequential choice before committing      |
| **PLAN**       | Turn intent into scoped work with acceptance criteria       |
| **BUILD**      | Make the change, document it, design it, or optimize it     |
| **AUDIT**      | Check architecture, tests, dependencies, and project health |

Most requests do not walk through every state. If you ask for a tiny edit, Aila can go straight to BUILD. If the task is vague or risky, it can slow down and ask for vision, discussion, or planning first. If an audit finds something important, it can loop back to the right earlier state instead of blindly continuing.

Under the hood, the state machine only owns the lifecycle. Tool choice still happens inside the current state. That keeps the workflow strict enough to avoid drift, but flexible enough for normal coding-agent work.

Exit signals are handled as signals, not fake states. `complete` and `flagged` move the work forward, `waiting` pauses for input, and `stuck` parks the work with blocker details so it can be resumed or redirected later.

The development reference is in [`docs/workflow-architecture.md`](docs/workflow-architecture.md).

### Utility model

A smaller model runs hidden utility capabilities in the background:

- prepare context for likely next work
- check whether saved context is stale
- compact context continuously without interrupting the current turn
- refresh summaries missing important details

Utility capabilities cannot silently change files and only run when primary model is idle.

## Built-in tools and capabilities

Everything here is built in. A few words below are intentional:

- **Commands** are what you type, such as `/compact`, `/review`, or `ctrl+x k`.
- **Tools** touch the world: files, shell, search, and fetch.
- **Capabilities** are the agent workflows built on top.
- **Utility capabilities** are background chores run by the smaller model.

| Tool  | What it does                                                   |
| ----- | -------------------------------------------------------------- |
| read  | Reads files with line ranges and safe previews                 |
| edit  | Applies approved text edits safely                             |
| write | Creates or overwrites files with permission checks and history |
| bash  | Runs local commands with visible scope and expected effect     |
| grep  | Searches project content and returns matching files and lines  |
| find  | Finds project files by path patterns                           |
| fetch | Fetches remote content and returns it as Markdown              |

Capabilities are the higher-level parts of a coding session:

| Glyph | Capability  | What it does                                                                  |
| ----- | ----------- | ----------------------------------------------------------------------------- |
| ⌂     | brief       | Briefs on project status, current plan, known gaps, and suggested next action |
| ⛥     | vision      | Helps you shape the project's vision and long-term goals                      |
| ❈     | discuss     | Structured deliberation before consequential choices                          |
| ⬚     | research    | Assists with adapting concepts, patterns, or solutions                        |
| ≡     | plan        | Scoped planning with behavioral acceptance criteria                           |
| ⧉     | build       | Executes a single task or step of a plan, and then holds                      |
| ⎘     | optimize    | Helps you research and design a locked harness that optimizes a metric        |
| ▤     | document    | Keeps documentation aligned with the actual project                           |
| ◰     | design      | Creates a design system that is durable and understood by agents              |
| ⛶     | audit       | Architecture, test, dependency, and project health audits                     |
| ♾     | profile     | Profiles your decision thought processes from previous conversations          |
| ⎈     | orchestrate | Autonomous plan execution with parallel agents, evaluation, and retry checks  |

Utility capabilities are hidden background work run by the utility model. They are never slash commands.

| Utility capability    | What it does                                                  |
| --------------------- | ------------------------------------------------------------- |
| context prep          | Prepares context for likely next work                         |
| stale-context check   | Checks whether saved context is stale                         |
| continuous compaction | Keeps background context compact without replacing `/compact` |
| summary refresh       | Refreshes summaries missing important details                 |

You do not need to remember these names. Say what you want. "Help me decide" routes to the `discuss` capability and Aila guides you from there.

## User interface

Aila is built as a rich terminal UI that always stays responsive and renders quickly.

### Chat interface

| Feature           | How it works                                                                 |
| ----------------- | ---------------------------------------------------------------------------- |
| Command shortcuts | Slash commands and `ctrl+x` shortcuts trigger the same handlers              |
| File reference    | Type `@` to search project files and insert exact file links                 |
| Paste format      | Pasting >2 lines results in a formatted `[Pasted lines +X]`                  |
| Message queue     | Messages are queued while work is active, but can optionally interrupt/steer |
| Diff viewer       | Review the current uncommitted changes as side-by-side or stacked diffs      |
| History           | Rewind the conversation or undo a file change with `/undo` or `ctrl+x u`     |
| Header            | See primary model, utility model, context window, and autonomy level         |
| Footer            | See git repo, branch, diff, worktree, and other useful git data              |

### Slash commands

Type `/` at the start of the input box to see available commands. Slash commands and `ctrl+x` shortcuts trigger the same command handlers.

| Command                 | Shortcut   | Description                                                              |
| ----------------------- | ---------- | ------------------------------------------------------------------------ |
| `/new` / `/clear`       | `ctrl+x n` | Start a new session and reload project memory                            |
| `/continue`             | `ctrl+x c` | Browse and restore saved sessions                                        |
| `/review`               | `ctrl+x i` | Review the current change set, risks, and sources                        |
| `/history`              | `ctrl+x h` | Browse runs, edits, checks, undo data, and reviews                       |
| `/undo`                 | `ctrl+x u` | Rewind the conversation or undo a file change                            |
| `/redo`                 | `ctrl+x r` | Redo the last undone conversation or file change                         |
| `/diff`                 | `ctrl+x d` | Review the current uncommitted changes                                   |
| `/editor`               | `ctrl+x e` | Open the current prompt in an editor                                     |
| `/compact`              | `ctrl+x k` | Immediately compact the current conversation                             |
| `/model`                | `ctrl+x m` | Switch primary model (`/model --utility` for switching utility model)    |
| `/status`               | `ctrl+x s` | Show utility model status, suggestions, and overall health               |
| `/auto`                 | `ctrl+x a` | Switch autonomy level (`off\|read\|write\|yolo`) for the current session |
| `/help`                 | `ctrl+x ?` | Show help and keybindings                                                |
| `/quit` (`/exit`, `/q`) | `ctrl+x q` | Quit Aila                                                                |

### Shell commands

Aila supports two shell prefixes:

| Prefix      | Behavior                                                           |
| ----------- | ------------------------------------------------------------------ |
| `!command`  | Runs the command and shows the result in the chat interface        |
| `!!command` | Runs the command, summarizes the output, and sends it to the agent |

Examples:

```bash
!go test ./...                         # Run tests after command approval
!git status --short                    # Inspect git state after Aila asks
!!go test ./internal/runtime -run Test # Run and analyze targeted output
!!rg "panic|TODO" internal/            # Search and ask Aila to reason over the matches
```

Aila saves important command output. Summaries keep the conversation moving, but exact failures, diffs, paths, commands, stack traces, and user constraints stay available when they matter.

## Data and privacy

Assume anything Aila needs to answer may be sent to the configured provider: prompts, repository context, diffs, tool results, and summarized command output. Do not use it on code or data you cannot send there.

`.aila/` is meant to be committed, so treat it as project-visible state.

---

## Acknowledgements

Aila builds upon concepts and ideas from terminal coding agents such as:

- [Crush](https://github.com/charmbracelet/crush)
- [Kon](https://github.com/0xku/kon)
- [OpenCode](https://github.com/anomalyco/opencode)
- [Pi.dev](https://github.com/earendil-works/pi/tree/main/packages/coding-agent)

Please give them a star!

---

### License

MIT

### Author

Jonathan Gabor ([jgabor.se](https://jgabor.se))
