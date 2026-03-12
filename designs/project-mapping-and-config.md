# Project Mapping and Configuration Design for aproc

## Purpose
Define a deterministic way for `aproc` to map any project workspace to hidden runtime state without requiring repository scaffolding. The design must keep runtime internals out of the workspace, allow multiple unrelated repositories to coexist without collisions, and keep project-local configuration optional and minimal.

## Approach
`aproc` identifies a project by its resolved project root and stores runtime state outside that workspace.

Project root detection:
- Start from the caller's current working directory.
- Walk upward until the first directory containing either `.git` or `.aproc.yml`.
- If neither marker exists before reaching the filesystem root, use the original working directory as the project root.
- Resolve symlinks before persisting identity so the same physical workspace does not get multiple runtime homes through different path aliases.

Canonical project identity and mapping key:
- The canonical identity is the normalized absolute path of the resolved project root after symlink resolution.
- The runtime mapping key is `basename + "-" + short_hash(canonical_path)`.
- The hash should come from a stable algorithm such as SHA-256, truncated only for directory naming. The full canonical path remains the source of truth in metadata when needed.
- This keeps the mapping deterministic across sessions while remaining readable in operator-facing paths.

Hidden runtime state location:
- Runtime-global state must live outside the repository in a user-owned hidden state root.
- Preferred base path is an OS-appropriate state directory, for example `~/.local/state/aproc/projects/<mapping-key>/` on Linux and similar user-state locations on other platforms.
- The workspace must not contain `.run`, `outputs`, caches, logs, or queue state by default.
- A project workspace may contain only source files plus an optional `.aproc.yml`.

Optional local config policy:
- `.aproc.yml` is optional and should exist only when a project needs overrides that cannot be expressed safely as global defaults.
- The local config surface should be limited to project-scoped behavior such as agent/layer selection, adapter overrides, allowlists, and policy toggles that are meaningful to collaborators.
- `.aproc.yml` must not store runtime-generated identifiers, run history, queue contents, logs, caches, or machine-specific absolute paths.
- If `.aproc.yml` is absent, `aproc` must still run with global defaults and inferred project identity.
- This keeps installable CLI usage zero-scaffold while still allowing explicit per-project behavior when justified.

Collision handling and multi-project coexistence:
- Two repositories with the same directory name but different canonical paths produce different mapping keys because the hash input is the full canonical path.
- Different subdirectories inside one repository map to the same runtime home when they resolve to the same detected project root.
- Moving a repository to a new absolute path creates a new mapping key by design. Old runtime state may remain as orphaned history until cleaned up by retention tooling; it must not be silently rebound to a different path.
- If a user wants stable identity across path moves, that should be a future explicit feature, not the default mapping rule.

Config placement decision:
- Project-local config belongs at `<project-root>/.aproc.yml` because that is the smallest discoverable surface that can participate in root detection without requiring additional scaffolding.
- All other config should remain in user-global config locations, leaving the repository clean unless the team deliberately opts into local overrides.

## Task list
- Detect project root by nearest `.git` or `.aproc.yml`, with fallback to the invocation directory.
- Resolve the canonical project identity from the normalized absolute root path after symlink resolution.
- Derive the hidden runtime directory from `basename + short_hash(canonical_path)` under an OS-appropriate user state root.
- Keep runtime internals entirely outside the workspace.
- Support an optional `.aproc.yml` only for project-scoped overrides and forbid runtime-generated state in that file.
- Treat path moves as new project identities unless a future explicit portability feature is added.

## Done condition
- Workspace-to-runtime mapping is deterministic from the canonical project root.
- Project-local state and runtime-global state are clearly separated.
- Multi-project coexistence is collision-safe even for repositories with the same basename.
- The default experience requires no repository scaffolding.
- The only allowed workspace surface introduced by `aproc` is an optional `.aproc.yml`.
