# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

ImageFunnel is a web app for triaging AI-generated images. Users classify images as Keep (5-star) / Shelve (3-star) / Reject via a mobile-first UI with touch gestures. Results are stored in XMP sidecar files (`.xmp`) alongside the original images — the original files are never modified.

- **Tech stack**: Vue 3 + TypeScript + Vite + Tailwind CSS 4 (frontend), Go 1.24 + gqlgen + gorilla/mux (backend), GraphQL (Apollo Client + WebSocket subscriptions)
- **Data model**: All in-memory at runtime; XMP sidecar files are the persistence layer. No database.

## Commands

```bash
# Frontend
pnpm dev                 # Start Vite dev server (port 8080, proxies /api to backend)
pnpm build               # Production frontend build
pnpm check               # TypeScript type-check + ESLint (always run after frontend changes)
pnpm lint                # ESLint only

# Backend
go test --timeout 30s ./...      # Run all Go tests (always run after backend changes)
go test --timeout 30s ./internal/domain/session  # Run tests for a specific package

# Build & generate
pwsh scripts/build.ps1            # Full build (frontend + Go, outputs to build/latest/)
pwsh scripts/run.ps1              # Dev mode (runs both frontend and backend)
pwsh scripts/generate-graphql.ps1 # Regenerate GraphQL code (Go + TypeScript) after schema changes
```

## Architecture

The backend follows **hexagonal architecture** (ports & adapters):

```
internal/
├── domain/          # Core business logic — zero external dependencies
│   ├── session/     #   Session aggregate (queue, rounds, undo, stats)
│   ├── image/       #   Image entity & filters
│   ├── directory/   #   Directory entity, scanner/registry interfaces
│   ├── metadata/    #   XMP metadata repository interface
│   └── memo/        #   Memo entity & repository interface
├── application/     # Orchestration layer — wraps domain into use cases
│   ├── session/     #   Session app handler (MarkImage, Undo, Commit, etc.)
│   ├── directory/   #   Directory listing & caching
│   ├── image/       #   Image processing (resize, cache, URL signing)
│   ├── memo/        #   Memo CRUD
│   └── root.go      #   Root struct composing all handlers
├── infrastructure/  # Technology implementations (inmem, localfs, xmpsidecar, magick, stdimage)
├── interfaces/      # Adapters to the outside world
│   ├── graphql/     #   gqlgen resolvers (delegate to application handlers)
│   └── http/        #   gorilla/mux routing, middleware, static files
└── shared/          # Pure DTOs, enums, filters — can be imported by any layer
```

Data flow for marking an image:
1. User swipes/clicks → Apollo Client sends `markImage` mutation
2. GraphQL resolver → `application/session.Handler.MarkImage()`
3. App handler → `domain/session.Service.MarkImage()`
4. Session aggregate updates queue, undo stack, stats (via `sessionRepo.Acquire/Release` lock)
5. Changed session published via `pubsub.Topic` → WebSocket subscription → frontend UI updates

## Key conventions

### Go
- **No `Get` prefix** on query methods: `Session()` not `GetSession()`
- **`iter.Seq` / `iter.Seq2[T, error]`** patterns instead of returning slices
- **Constructor functions** use `New` prefix; return cleanup function as second return value when needed: `func NewFoo() (*Foo, func())`
- **Compile-time interface checks**: `var _ Interface = (*Impl)(nil)`
- **Never silently ignore errors** — if an error truly cannot occur, panic
- **Business errors** via `internal/apperror` package
- **Enums** in `internal/shared/enums.go`
- **Logging** (zap): log messages in lowercase sentence-fragment style; include `duration` field for timing; use "will X" / "did X" prefix for long-running operations
- **Tests**: add unit tests for new functionality; test file name mirrors the source file name
- **Errors package**: use `errors` package, avoid direct comparison
- **Context**: pass `context.Context` through request scope

### Vue / TypeScript
- **Declarative over imperative**: prefer `computed` over `watch` for derived state
- **Template refs**: use `useTemplateRef` (single) or `@/composables/useTemplateRefs` (array)
- **Composable parameters**: use `MaybeRefOrGetter` unless there's a specific reason not to
- **null vs undefined**: use `undefined` (not `null`) in return values; `null` is acceptable in parameters
- **GraphQL types**: import from `@/graphql/generated`, don't manually define GraphQL types
- **Loading states**: use CSS/animation, not text placeholders, to avoid layout shift
- **lodash**: use `es-toolkit` instead (assumes modern browser APIs)
- **`useStorage` composable**: use for localStorage; keys use format `name@randomSuffix`; define in `<script lang="ts">` block (not `<script setup>`) for shared state

### GraphQL
- **Fragments** to avoid repeating query fields; name without `Fragment` suffix
- **Schema files**: one file per field/type in `graph/`, named in `snake_case`
- **Extend types** for root object fields beyond the first
- After schema changes: run `pwsh scripts/generate-graphql.ps1`, then update resolvers
- No backwards compatibility needed (frontend and backend ship together)

### General
- **Comments**: use Chinese for explanatory comments that help understand context; avoid comments that just translate code
- **Region comments**: use `// #region {name}` / `// #endregion` for long related code blocks
- **IDs**: client should not parse IDs — format is intentionally opaque
- **Don't edit generated code** — regenerate with the appropriate script instead
- **Build via `scripts/build.ps1`** rather than running `go build` or `pnpm build` directly
