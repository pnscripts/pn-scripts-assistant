# Roadmap

Full plan: see the approved plan this repo was built from (Phase 0 + Phase 1 are done;
everything below is what comes next, in order).

- ~~**Phase 2** — Validator/Curator/Promotion pipeline; semantic recall.~~ **Done**, see below.
- **Phase 3** — The real Jarvis-style web dashboard (chat + memory browser), replacing
  the temporary smoke-test page at `/`.
- **Phase 4** — Opt-in knowledge ingestion, reviewed before anything is promoted.
  Projects and Documents: done (see below). Still open: browser history, email
  (requires OAuth setup, not something to build silently).
- **Phase 5** — Desktop app (Tauri) and mobile client (PWA or Flutter first), and an
  optional Godot-built animated HUD front end for the visual "Jarvis" feel.
- **Phase 6** — Multi-drive expansion: a drive registry in `PNEXUS-DATA`, so when more
  drives get connected, Pnexus can extend storage across them instead of requiring a
  rebuild.
- **Phase 7** — Voice interface, home-automation hooks, other integrations, as needed.

## What's already built (Phase 0 + Phase 1)

- Portable data root (`PNEXUS-DATA/`, marked with `.brain-root.json`) — survives
  moving to a new computer or a new drive. See `scripts/start-brain.sh`.
- Dockerized stack (Laravel Sail + `pgvector/pgvector:pg16` + Redis), so it runs the
  same regardless of what's installed on the host.
- Hybrid LLM routing (`App\Brain\Llm\LlmRouter`) between local Ollama and the
  Anthropic API, behind one `Provider` contract.
- Conversation/message persistence, plus a capture-only learning pipeline: every
  conversation turn queues `App\Brain\Learning\ExtractLessonJob`, which proposes a
  "Lesson" (quarantined, unpromoted) when something reusable came up.
- A persona layer (`App\Brain\Persona`) — every new conversation opens with a system
  prompt establishing who Pnexus is, so the personality is consistent across every
  client instead of each one having to know or repeat it. Configurable via
  `BRAIN_NAME` / `BRAIN_OWNER` in `.env`.
- Filament admin at `/admin` for browsing Conversations and reviewing proposed Lessons.
- `clients/cli` — a Go terminal client for `/api/chat`. First proof that Pnexus's API
  is genuinely client-agnostic: any language can talk to it without touching the
  Laravel core. Phase 5's Godot HUD, desktop, and mobile clients follow the same
  pattern — thin, language-appropriate, all hitting the same API.
- `App\Brain\Learning\ProjectScanner` + `php artisan brain:ingest-projects` — read-only
  scan (see `compose.yaml` `:ro` mounts of `SCAN_DEV_PROJECTS_PATH` /
  `SCAN_HOME_PROJECTS_PATH`) that walks project directories, detects the tech stack per
  project (composer.json/package.json/go.mod/etc.), and proposes one Lesson per project
  found. Same quarantine as chat-derived Lessons — nothing is promoted automatically.
  Known limitation: can't yet tell a hand-written module from a bundled third-party one
  inside a CMS's flat modules/plugins folder (WordPress's `wp-content/plugins` is
  excluded outright; PrestaShop-style `modules/` folders are not, since some of those
  actually are custom work) — review before promoting anything from there.
- `App\Brain\Learning\DocumentScanner` + `php artisan brain:ingest-documents` — same
  pattern, mounted at `SCAN_DOCUMENTS_PATH`, but deliberately **metadata-only**:
  filename, extension, size, modified date. Content excerpts only for plain `.txt`/
  `.md` files; `.docx`/`.odt`/`.pdf`/`.xlsx` are never parsed, since those are exactly
  the types most likely to hold financial or identity documents (confirmed on this
  machine — there's a real accounting/invoices folder). Extend to real content
  extraction only for specific files/folders if actually needed, never as a default.

## The learning loop (Phase 2, done)

`php artisan brain:evolve` (add `--dry-run` to preview) walks quarantined Lessons
through two roles, mirroring the reference system:

1. **Validator** (`App\Brain\Learning\Validator`) — splits claims by *how they were
   obtained*. Filesystem observations are re-checked against disk, so this is real
   verification: if a project was deleted since the scan, the Lesson is rejected
   instead of promoted. Chat-derived Lessons are model *inferences* and cannot be
   machine-checked, so they stay quarantined for human approval — otherwise a 3B
   local model would quietly write its own guesses into long-term memory as fact.
2. **Curator** (`App\Brain\Learning\Curator`) — embeds each validated Lesson and
   promotes it to `knowledge_facts`, skipping anything within 0.95 cosine similarity
   of existing knowledge. Dedupe is semantic because the same fact genuinely arrives
   in different words (many projects are mirrored between the external drive and
   `~/Projects`).

**Recall** closes the loop: `App\Brain\Memory\MemoryStore::recall()` embeds each
incoming message and injects the most relevant knowledge into that turn's prompt.
Injected per-turn rather than persisted, so corrected or deleted facts stop being
repeated. Recall failure is non-fatal — the brain answers without memory rather than
erroring.

Embeddings are always local (`nomic-embed-text` via Ollama, 768-dim): every stored
memory gets embedded, so a paid API would be both costly and a needless disclosure of
everything the brain knows.

First real run: 122 filesystem Lessons validated, 79 promoted, 43 caught as duplicates,
10 chat Lessons held for review. Verified end-to-end by asking the brain which projects
are written in Go — it answered from self-taught knowledge alone. Note: all 4 Go
projects ranked top-4 in retrieval, but `llama3.2:3b` dropped one when summarizing;
retrieval quality is not the limiting factor, model size is.

## Naming

Pnexus is an independent project — no business or brand tie to PN Scripts beyond where
the name came from. `app/Brain/` stays as the internal namespace for the core logic (a
generic architectural term, like "core" or "engine"), which is normal even for a named
product — it isn't user-facing.

Before any public launch (GitHub, etc.), run an actual trademark search — this was only
screened against obviously conflicting major brands, not professionally cleared.
