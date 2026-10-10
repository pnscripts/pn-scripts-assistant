name: build_html5_games
version: 1
when: I ask to build, make or generate a batch of new HTML5 browser games, a game factory batch, small free games for itch.io or Game Jolt, a game zip, a cover, screenshots or the EN/BG game texts
tools: run_command, read_file, write_file, edit_file, list_directory, search_files, do_in_background
---

The workspace root is DEV, the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV. Games live in DEV/Projects/pnscripts/marketplace/<slug>, one folder and one local git repository per game. The durable description of the factory contract is DEV/ai-brain/projects/marketplace/games-batch-10-11.md; read it first. The kit (FACTORY.md, shell.js, build.py, smoke.mjs, namecheck.sh) was kept in a coordinator session's scratchpad and may no longer exist. Look for it with search_files; if it is gone, rebuild the same contract from that profile rather than inventing a new one.

## Before you start

1. Check the machine load (see the machine_throttling playbook). At most two builders at once, and only one headless Chrome or node test at a time.
2. Read the family profile and DEV/ai-brain/business/pipeline.md so that you do not repeat a game that already exists. Distinctness matters more than count: itch.io, Game Jolt and search engines punish near-duplicates.
3. For every proposed name, check that no local folder and no itch.io game already uses it (the kit's namecheck.sh does both). Names are one original word, with no trademark in them.
4. Agree the list of concepts with the owner: genre, core mechanic and the game's own twist, one line each.

## Steps

1. For each game, create DEV/Projects/pnscripts/marketplace/<slug>/ with exactly three hand-written files: index.html (the fixed template from FACTORY.md: charset, viewport, title, description, then shell.js and game.js), game.js and meta.json.
2. game.js is plain modern JavaScript in one strict IIFE, typically 500 to 1,200 lines, calling PN.boot({...}) with start, update, draw, key, pointer, state and bot. No modules, no build step, no asset files: every shape is drawn on the canvas and every sound is synthesised. No external requests and no third-party code.
3. Meet the quality bar: a win or a lose state, real progression (at least eight levels for level-based games, a competent AI for board and card games), full play with the keyboard alone and with touch alone (touch targets of at least 44 logical pixels), a palette of its own, sound for every action, best score saved through api.save. The shell reserves P and Esc for pause and M for mute; do not bind them.
4. The bot must play through the same functions the player's input calls. It must never set the score or call end itself.
5. Write meta.json with slug, title, version 1.0.0, batch, the site genre and the itch genre from the allowed lists, kind and kindBg, short (at most 120 characters), siteDescription (350 to 700 characters, facts only), up to ten lowercase tags, controls, the itch description lines, differs (the closest known games and our twist), the test block (keys, taps, what must change in state(), endBy, endMaxSeconds, screenshot ticks, coverTicks) and the template texts in English and natural Bulgarian (polite plural imperative, game names stay in Latin script).
6. Build one game at a time: run_command with argv ["nice","-n","19","python3","<kit>/build.py","<game folder>"]. It validates meta.json, copies shell.js, runs the headless Chrome smoke test (title screen, no console errors, keys and taps change state, a run reaches the result screen, a phone viewport can start it), captures store-assets/ (cover-630x500.png, screenshot-1.png to screenshot-3.png, card-960x540.jpg), builds release/<slug>-itch-1.0.0.zip with index.html at the zip root, builds the paid template zip, writes docs/listing-itch.md and commits locally. Repeat until it prints "ok": true.
7. Look at the cover and screenshot-2 with read_file. If they look empty, cramped or like a prototype, improve the drawing or the shot ticks and rebuild.
8. Keep the AI disclosure exactly as build.py writes it: code and text written with AI assistance, checked by PN Scripts with automated tests, every shape drawn and every sound synthesised by code, no AI-generated image or audio files. On itch.io this maps to AI disclosure: Code and Text only.
9. The paid source template of each game stays Coming soon (see sales_hold). Never put a price or a store link in its copy.
10. Record the batch in the family profile in DEV/ai-brain (folder, kind, genre, git head) and run the brain check (see update_ai_brain).

## Verify

- build.py printed "ok": true for every game, with the zip path and byte size.
- unzip -l on the itch zip lists index.html at the root, not inside a folder.
- store-assets/ holds the cover at 630 by 500 pixels and three screenshots, and you have looked at them.
- git log in each game folder shows the build commit.
- Report per game: slug, one-line concept, git head, zip path and bytes, and the bot's result.

## Never

- Never publish anything from this playbook. Publishing is publish_itch_io or publish_game_jolt, and only with the owner's yes.
- Never create GitHub repositories, never push, and never add .github/workflows.
- Never run more than two builders, never run builders beside the pnscripts.com test suite, and never write busy-wait loops.
- Never use pkill -f; stop processes by their recorded process id.
- Never type passwords. Never put secrets, tokens or personal data into game files, meta.json or commits.
- Never delete an existing game folder, build or release zip; bump the version instead.
- Never claim features in the texts that the game does not have, and never use a trademark in a game name.
