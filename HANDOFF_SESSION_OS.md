# Session OS Handoff

## Why This Exists

This note captures the reasoning behind the current direction so the next worker does not have to reconstruct it from chat history.

The original `agentproc` idea was:

- process-like orchestration
- agent collaboration through `process -> subprocess -> thread`
- file-based task execution

During discussion, the main blocker became session continuity:

- users do not want to re-explain the same task when switching runtimes
- runtime-native sessions are inconsistent across Claude, Codex, Gemini, and OpenCode
- agent-to-agent work breaks down without persistent shared task state

This led to the `session OS` framing.

## Current Architectural Direction

### Core Principle

`agentproc` should remain the orchestration kernel and become the canonical work-session store.

That means:

- `agentproc` owns canonical work state
- vendor/runtime sessions are attachments, not the source of truth
- the same work session should survive runtime changes

### Canonical vs Vendor State

What should be canonical:

- goal
- constraints
- decisions
- plan
- open questions
- artifacts
- handoffs
- worker status

What should remain vendor-private:

- transcript wire format
- runtime session IDs
- approval/tool state
- provider-specific message objects

### Important Terminology

- `work session`: the persistent task-centered state users actually want to continue
- `runtime session`: a vendor-specific conversation/session object
- `worker session`: a role-bound active unit of work inside a work session
- `handoff`: structured transfer from human/process/worker to another worker

## Runtime Findings

### Codex

- Good transcript portability
- Local sessions stored as JSONL
- Strong candidate for full adapter support

### Claude

- Good transcript portability
- Local sessions stored as JSONL
- Strong candidate for full adapter support

### OpenCode

- Good integration surface
- Session/message/part state stored in SQLite
- Has share/export/session affordances
- Can act as UI surface and possibly backend-facing integration surface

### Gemini

- Weak transcript portability
- Local session storage is protobuf-heavy
- Some recoverable task state exists through `~/.gemini/antigravity/brain/`
- Treat as artifact-first or summary-first adapter for now
- Do not assume full transcript portability

## Product Direction

There are two layers now. They should not be confused.

### 1. `agentproc` Kernel

Owns:

- canonical work session
- orchestration
- task routing
- handoffs
- adapter boundaries
- event model

### 2. Optional Surface Layer

Can be built later on top:

- single-user session dashboard
- team-scoped shared session view
- comments / mentions / presence
- Figma-like inspection and intervention experience

This UI layer is downstream of the kernel, not the other way around.

## Why OpenCode Matters

OpenCode is important because it is the most realistic product surface.

Reasons:

- can potentially be used as UI and backend-facing server surface
- more controllable than vendor-owned CLIs
- better candidate for service packaging than Claude/Codex/Gemini directly

Current conclusion:

- `agentproc` should continue as the backend kernel
- `opencode` is the most likely primary surface
- integration should prefer extension/plugin/SDK boundaries over deep patching

## Design Constraints

- Avoid deep customization inside vendor runtimes
- Avoid designs that depend on raw transcript portability across all runtimes
- Prefer event-driven integration over invasive patching
- Prefer canonical state projection over transcript normalization
- Minimize patch surface because upstream changes will break deep integrations

## Immediate Next Steps

1. Define canonical `work session` schema
2. Define canonical event schema
3. Define adapter capability matrix
4. Define `agentproc <runtime>` entrypoint contract
5. Define OpenCode integration boundary
6. Decide how much transcript import is necessary for Claude/Codex/OpenCode
7. Keep Gemini in artifact-first mode until proven otherwise

## What Not To Do Yet

- Do not build the collaboration UI first
- Do not assume public multi-user sharing
- Do not assume Gemini transcript reconstruction is good enough
- Do not move source of truth into OpenCode or any vendor runtime

## One-Sentence Summary

The current plan is to evolve `agentproc` into the canonical session kernel for cross-runtime work, while treating OpenCode as the likely primary product surface and keeping vendor runtimes as adapters rather than sources of truth.
