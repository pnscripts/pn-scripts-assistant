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
- **Phase 7** — Voice interface, other integrations.

### Next up

1. ~~**Agent loop**~~ — **done**. `App\Brain\Agent\AgentLoop` lets the model choose
   tools and read results back. Provider is chosen per request, since a 3B local
   model and a frontier API model are not interchangeable at tool use.
2. **Host agent (Go)** — a small daemon running on the host, outside Docker, so
   the brain can reach the whole machine and the LAN. Where Go genuinely earns
   its place: no runtime to install, and the container cannot do this job. Every
   call it accepts still routes through the same permission gate.
3. **Web tools** — fetch and search, `Safe` for reads.
4. **Integration adapters** — one interface so smart-home platforms (Home
   Assistant, Tuya, Hue, Zigbee) and other services plug in without touching the
   core. Device control is `Mutating` by definition: switching on a heater is not
   a reversible read.

## Two deployment targets, one codebase

| | Server mode (today) | Desktop mode (`.env.desktop.example`) |
|---|---|---|
| Runtime | Docker Compose | One binary |
| Database | Postgres + pgvector | SQLite |
| Vector search | `<=>` operator, indexed | Cosine in PHP |
| Queue | Redis | Database |
| Scan paths | read-only mounts | absolute host paths |

Both run the same code; the differences are configuration and two swappable
drivers. `SCAN_*_PATH` is the host side of a compose mount, while
`SCAN_*_READ_PATH` is where the app actually reads — the same folder in server
mode, seen through a mount.

**Release builds run in CI, not locally** (`.github/workflows/release.yml`).
Compiling PHP statically needs ~10GB of scratch space; this development machine
has under 9GB free on root, and filling root on Linux breaks more than the build.
CI is also where cross-platform release artifacts belong. The workflow produces
the self-contained brain binary plus desktop and CLI clients for Linux, macOS
(Intel and Apple Silicon) and Windows.

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

## Capabilities and the permission gate

Anything Pnexus can *do* is a Tool (`App\Brain\Tools\Contracts\Tool`), so one
permission rule and one audit trail cover every capability — filesystem, web,
smart home, anything added later — instead of each integration inventing its own
rules.

Each tool declares a `Risk`, and the line is drawn at **observable effect**, not
at how alarming the name sounds:

| Risk | Meaning | Behaviour |
|---|---|---|
| `Safe` | Observes only; reversible | Runs immediately |
| `Mutating` | Leaves the world changed | Queued, waits for human approval |

`ToolExecutor` is the only path from intention to action. A mutating call is
recorded *before* it can run and executes only via `approve()`, so there is no
route from "the model decided to" to "it happened" that skips the record. Failures
are recorded rather than thrown — the model needs to read what went wrong, and the
audit trail should show attempts, not just successes.

Approvals appear at `/admin/tool-invocations` with a badge counting what's waiting.
Each shows a concrete summary ("OVERWRITE /x/y.php (40 lines)") rather than raw
JSON, because nobody can meaningfully approve a blob. Every registered capability
is listed in one readable file, `App\Providers\ToolServiceProvider` — adding one
should be a visible, deliberate act.

Verified end to end: safe tool auto-ran; mutating tool stayed `pending` with no
file on disk; approval wrote it; rejection never wrote at all.

**Writable area:** `PNEXUS-DATA/workspace` (mounted rw) is the only place the brain
can currently write. It lives inside the portable data root, so what the brain
creates travels with it rather than scattering across the host.

**Ordering note:** the permission gate was built *before* any capability that
needs it. A host-level agent holding shell access must not exist before the
mechanism that can refuse it.

## Naming

Pnexus is an independent project — no business or brand tie to PN Scripts beyond where
the name came from. `app/Brain/` stays as the internal namespace for the core logic (a
generic architectural term, like "core" or "engine"), which is normal even for a named
product — it isn't user-facing.

Before any public launch (GitHub, etc.), run an actual trademark search — this was only
screened against obviously conflicting major brands, not professionally cleared.
