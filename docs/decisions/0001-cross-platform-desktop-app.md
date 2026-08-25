# ADR 0001 — Cross-platform desktop app

**Status:** accepted · **Date:** 2026-08-25

## The real question

"Make it a desktop app" looks like a UI question. It isn't. Pnexus currently needs
Docker, PHP 8.4, Postgres 16 + pgvector, and Redis. That is a *server*. A desktop app
is a *client*. So there are two separable problems, and only one of them is about
windows:

1. **A native window instead of a browser tab** — easy, cosmetic.
2. **Running on a machine with none of that stack installed** — the actual problem.

Solving only (1) produces a browser wrapper that still demands `docker compose` and an
external drive. For personal use that is tolerable. For the stated goal — open source
on GitHub, installable by other people — it is fatal: nobody installs Docker to try a
personal assistant. Ollama, LM Studio and Obsidian are all one-click, and that is the
bar.

So the desktop requirement forces a decision about the **backend runtime**, and that is
what this ADR is really about.

## Options considered

| Option | Ships as | Verdict |
|---|---|---|
| Keep Docker, wrap in a webview | App + Docker Compose | Fine for Petar's own machines; unusable as a distributable product |
| Rewrite the core in Go | One binary | Best distribution story, but discards the entire Laravel core and lands the project in a language Petar maintains less fluently — the opposite of why Laravel was chosen |
| **Embed PHP in the app** | One binary | Keeps the Laravel core *and* removes every external dependency |

Two mature tools make the third option real, both verified current as of 2026:

- **[FrankenPHP embed](https://frankenphp.dev/docs/embed/)** compiles a Laravel app, the
  PHP interpreter and the Caddy web server into a single static binary with no
  dependencies at all.
- **[NativePHP](https://nativephp.com/docs/desktop/1/getting-started/installation)**
  (Pociot & Hamp) wraps Laravel in Electron, downloading a statically compiled PHP
  binary as part of `composer require`. Purpose-built for precisely this situation.

## Decision

Adopt the embedded-PHP path, and treat the two runtimes as **deployment targets of one
codebase**, not as separate applications:

- **Server mode** (today): Docker, Postgres + pgvector, Redis. Development and any
  future always-on/multi-device use.
- **Desktop mode** (target): one installable app. SQLite instead of Postgres, database
  queue instead of Redis, no Docker.

## What this actually costs

The blocker is not the window. It is that `MemoryStore` currently writes raw pgvector
SQL — `::vector` casts and the `<=>` operator — which SQLite cannot execute. **Vector
search is the only real coupling to Postgres**, so it must move behind a driver
interface with two implementations:

- `PgVectorSearch` — the existing indexed operator; server mode.
- `SqliteVectorSearch` — cosine computed in PHP over stored vectors; desktop mode.

Brute-force cosine is genuinely adequate at personal scale (there are 79 facts today;
a few thousand stays comfortably under a tenth of a second) and it degrades on a
predictable curve, so the ceiling is worth documenting rather than hiding: somewhere in
the tens of thousands of facts, desktop mode needs a real index — `sqlite-vec`, or
falling back to server mode.

Everything else in the app is already storage-agnostic Eloquent.

## Consequences

- The vector-search abstraction is a **prerequisite**, not a nicety — it is the one
  thing standing between "web app that needs Docker" and "app you can hand to someone".
- Redis becomes optional; the queue driver moves to `database` for desktop builds.
- Windows and macOS builds realistically happen in CI, not on this Linux machine.
- Electron bundles are large (~150MB). Tauri would be far smaller but needs a Rust
  toolchain that is not installed here; NativePHP's Tauri support is also less mature
  than its Electron support. Electron first, revisit later.
- NativePHP's desktop offering and its mobile offering are licensed differently. Check
  current terms before shipping commercially — this was not verified here.
