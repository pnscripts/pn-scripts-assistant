name: publish_game_jolt
version: 1
when: I ask to publish, add, upload or update a game on Game Jolt or gamejolt.com, a Game Jolt package, release, browser build, thumbnail, media, maturity rating or the partner system
tools: drive_browser, read_file, list_directory, search_files
---

PN Scripts publishes its free browser games on Game Jolt (https://gamejolt.com) under the profile named "PN Scripts". The partner system (Game Jolt's ad revenue share) must stay OFF: ad and revenue-share income counts as revenue and is on hold (see sales_hold). Use the same zip, texts and images as for itch.io: release/<slug>-itch-<version>.zip, docs/listing-itch.md and store-assets/ in the game folder.

## Before you start

1. The owner has said yes to publishing this game.
2. The zip has index.html at its root and the game plays locally (build passed).
3. Open https://gamejolt.com/dashboard with drive_browser. If a sign-in page appears, stop and ask the owner to sign in in that window.
4. Check that the game is not already on the profile, so that no duplicate is created.

## Steps

1. Dashboard: Add a game. Enter the title and choose the type "Game".
2. Development status: Complete. Development stage: Stable (released). Do not mark a finished game as a work in progress.
3. Details: choose the category matching the game's genre, set the engine to HTML and the language to English. In the monetisation or partner settings, make sure the partner system is OFF and no ads or revenue options are enabled.
4. Description: paste the description from the kit. Game Jolt uses hashtags for discovery, so end it with relevant #tags (for example #puzzle #arcade #browsergame #html5) taken from the kit's tag list.
5. Thumbnail: upload store-assets/cover-630x500.png (crop it to the required ratio in the editor if asked). Header image: optional, use the card image if one is needed.
6. Media: upload screenshot-1.png to screenshot-3.png.
7. Packages: create a package named after the game, with pricing Free. Add a release (version 1.0.0), and in it a browser build: upload the zip, choose type HTML, set the embed size to 1280 x 720, and allow fullscreen. Then press Publish Release.
8. Maturity: fill in the maturity questionnaire honestly (typically no violence, language, sexual content or gambling for the factory games; read the game before answering; horror games may need a fear or theme answer).
9. Review the game page preview. When the owner has confirmed, press Publish for the game.
10. For an update: add a new release with the new version number in the same package and publish it. Keep the old release; never delete it.
11. Record the Game Jolt URL and date in the game's profile in DEV/ai-brain and in business/pipeline.md.

## Verify

1. Open the public game page in a fresh tab: title, thumbnail, screenshots, description with tags and the Play button are visible.
2. Press Play, find the game's iframe with drive_browser eval and confirm that the canvas exists and the title screen draws; dispatch Enter and confirm that it starts.
3. Confirm on the game's settings that the partner system is still OFF.
4. Take a screenshot as evidence and report the public URL.

## Never

- Never turn on the partner system, ads or any paid package while the sales hold is active.
- Never type a password; the owner signs in himself.
- Never publish a game without the owner's yes, and never accept new terms on his behalf.
- Never delete old releases, builds or games; add a new release or hide instead.
- Never publish on Newgrounds as a side step: it bans AI-generated games.
- Never put secrets, tokens or personal data into descriptions or files.
