# Roadmap

Full plan: see the approved plan this repo was built from (Phase 0 + Phase 1 are done;
everything below is what comes next, in order).

- **Phase 2** — Validator/Curator/Promotion pipeline with real confidence gates; an
  "evolve brain" command; Lessons start actually becoming durable `knowledge_facts`.
  This is where semantic recall (the `embedding` columns already in the schema) gets
  populated and used.
- **Phase 3** — The real Jarvis-style web dashboard (chat + memory browser), replacing
  the temporary smoke-test page at `/`.
- **Phase 4** — Opt-in knowledge ingestion from existing Projects/Documents, reviewed
  before anything is promoted.
- **Phase 5** — Desktop app (Tauri) and mobile client (PWA or Flutter first), and an
  optional Godot-built animated HUD front end for the visual "Jarvis" feel.
- **Phase 6** — Multi-drive expansion: a drive registry in `VESPER-DATA`, so when more
  drives get connected, Vesper can extend storage across them instead of requiring a
  rebuild.
- **Phase 7** — Voice interface, home-automation hooks, other integrations, as needed.

## What's already built (Phase 0 + Phase 1)

- Portable data root (`VESPER-DATA/`, marked with `.brain-root.json`) — survives
  moving to a new computer or a new drive. See `scripts/start-brain.sh`.
- Dockerized stack (Laravel Sail + `pgvector/pgvector:pg16` + Redis), so it runs the
  same regardless of what's installed on the host.
- Hybrid LLM routing (`App\Brain\Llm\LlmRouter`) between local Ollama and the
  Anthropic API, behind one `Provider` contract.
- Conversation/message persistence, plus a capture-only learning pipeline: every
  conversation turn queues `App\Brain\Learning\ExtractLessonJob`, which proposes a
  "Lesson" (quarantined, unpromoted) when something reusable came up.
- A persona layer (`App\Brain\Persona`) — every new conversation opens with a system
  prompt establishing who Vesper is, so the personality is consistent across every
  client instead of each one having to know or repeat it. Configurable via
  `BRAIN_NAME` / `BRAIN_OWNER` in `.env`.
- Filament admin at `/admin` for browsing Conversations and reviewing proposed Lessons.
- `clients/cli` — a Go terminal client for `/api/chat`. First proof that Vesper's API
  is genuinely client-agnostic: any language can talk to it without touching the
  Laravel core. Phase 5's Godot HUD, desktop, and mobile clients follow the same
  pattern — thin, language-appropriate, all hitting the same API.

## Naming

The project, the open-source repo, and the assistant's spoken persona are all "Vesper"
— one brand, not a technical-name/product-name split. `app/Brain/` stays as the internal
namespace for the core logic (a generic architectural term, like "core" or "engine"),
which is normal even for a named product — it isn't user-facing.

Before any public launch (GitHub, pnscripts product listing), run an actual trademark
search — this was only screened against obviously conflicting major brands, not
professionally cleared.
