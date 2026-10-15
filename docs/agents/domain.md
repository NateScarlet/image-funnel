# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT-MAP.md`** at the repo root — it points at one `CONTEXT.md` per context. Read each one relevant to the topic.
- **`docs/adr/`** (Main App) and **`example_hooks/docs/adr/`** (Example Hooks) — read ADRs that touch the area you're about to work in.

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and `/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

This repository is configured as a **multi-context** codebase (indicated by the presence of `CONTEXT-MAP.md` at the root):

```
/
├── CONTEXT-MAP.md                      ← Points to context definitions
├── CONTEXT.md                          ← Context definition for Main App
├── docs/adr/                           ← Main App decisions
└── example_hooks/
    ├── CONTEXT.md                      ← Context definition for Example Hooks
    └── docs/adr/                       ← Example Hooks decisions
```

## ADR conventions

Numbering is **unique across the whole project**, not per context. When adding an ADR, scan every `docs/adr/` (root and per-context) and take the global maximum + 1; file the new ADR under the `docs/adr/` of its own context. Numbers are stable identifiers: moving an ADR between contexts does **not** renumber it, so gaps are expected inside a context's directory. This keeps existing references in code comments and docs valid.

## Available Contexts

1. **主应用 / Main App**: Core logic of ImageFunnel, including backend session routing, XMP sidecar reading, and frontend UI.
   - Context Location: [CONTEXT.md](../../CONTEXT.md)
2. **外部钩子示例 / Example Hooks**: Implementation logic for ComfyUI workflow adjustments, Danbooru autocomplete, etc.
   - Context Location: [example_hooks/CONTEXT.md](../../example_hooks/CONTEXT.md)

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (event-sourced orders) — but worth reopening because…_
