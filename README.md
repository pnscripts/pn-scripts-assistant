# Pnexus

A personal, self-learning AI assistant. Laravel core, hybrid local/API LLM routing, and
a Lesson-quarantine learning pipeline modeled on the `.ai/` knowledge-promotion system
from the `pnscripts` Laravel project. Full architecture and phase roadmap: see
[docs/ROADMAP.md](docs/ROADMAP.md).

## Portability

Pnexus's *data* (conversations, Lessons, promoted knowledge, Postgres/Redis files)
lives outside this repo, in a portable `PNEXUS-DATA/` folder on an external drive,
marked with `.brain-root.json`. This repo (the code) can be cloned onto any machine;
`scripts/start-brain.sh` finds the data root wherever it currently is (or creates a new
one) and points Docker at it — nothing is hardcoded to one computer or one drive.

## Running it

Requires Docker. First time, or after plugging the drive into a different computer:

```bash
scripts/start-brain.sh
```

Then, once containers are up:

```bash
docker compose exec laravel.test php artisan migrate
docker compose exec laravel.test php artisan make:filament-user
```

Visit `http://localhost:8090` for the temporary chat smoke-test page, or
`http://localhost:8090/admin` for the Filament admin (Conversations, Lessons).

To stop: `scripts/start-brain.sh --down`

## Configuration

Copy `.env.example` to `.env` (done automatically by `start-brain.sh` on first run) and
set `ANTHROPIC_API_KEY` if you want the `anthropic` provider available. Local Ollama
models (already installed on the host) are used via `http://host.docker.internal:11434`
— no key needed, but slower since inference is CPU-only.

`BRAIN_NAME` / `BRAIN_OWNER` control the assistant's spoken identity (see
`app/Brain/Persona.php`) — change them freely, nothing else depends on the value.

## API

`POST /api/chat` — body: `{ "message": "...", "conversation_id": null, "provider": null }`.
`provider` is optional (`"ollama"` or `"anthropic"`); omit it to use the configured
default (`LLM_DEFAULT_PROVIDER` in `.env`).

`GET /api/brain` — `{ "name": "Pnexus", "owner": "Petar" }`. Lets every client introduce
the assistant the same way without hardcoding its name.

## Clients

Pnexus itself (routing, learning pipeline, database) is single-sourced in Laravel on
purpose — one source of truth for the logic. Everything that just *talks* to it over
the API is free to be whatever language fits its platform:

| Client | Language | Status |
|---|---|---|
| [clients/cli](clients/cli) | Go | done — terminal chat client |
| Jarvis-style HUD | Godot | planned (Phase 5) |
| Desktop app | C#/.NET | planned (Phase 5) |
| Mobile | Flutter/PWA | planned (Phase 5) |

See [docs/ROADMAP.md](docs/ROADMAP.md) for the full sequencing.

## License

MIT — see [LICENSE](LICENSE).
