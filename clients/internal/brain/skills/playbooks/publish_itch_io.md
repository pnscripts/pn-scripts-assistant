name: publish_itch_io
version: 1
when: I ask to publish, upload, release or update a free game on itch.io, an itch page, an HTML build or zip, the embed size, the cover, screenshots, tags or the AI disclosure on pnscripts.itch.io
tools: drive_browser, run_command, read_file, list_directory, search_files
---

The PN Scripts itch.io account is https://pnscripts.itch.io. Free browser games may be published; anything paid stays on hold (see sales_hold). Each game built by the factory carries its own listing kit in docs/listing-itch.md and its images in store-assets/. butler is installed at ~/.local/share/butler/butler for command-line uploads; the owner signs butler in himself.

## Before you start

1. The owner has said yes to publishing this particular game. Ask if he has not.
2. The game passed its build: release/<slug>-itch-<version>.zip exists and index.html sits at the zip root (check with unzip -l).
3. Read docs/listing-itch.md in the game folder: title, short description, genre, tags, description, AI disclosure, images.
4. Open https://itch.io/dashboard with drive_browser. If a sign-in page appears, stop and ask the owner to sign in in that window. Do not type anything into a sign-in form.

## Steps

1. Create a new project: https://itch.io/game/new. The form's selectize widgets and the cover button appear a moment after load; wait until select.selectize exists before filling anything.
2. Fill in the title, the project URL (the slug), the short description and the classification "Games".
3. Kind of project: HTML.
4. Release status: Released. Pricing: "No payments" for a free game (or $0 with donations off).
5. Uploads: upload the itch zip. Tick "This file will be played in the browser".
6. Embed options: viewport 1280 x 720, fullscreen button on, mobile friendly on, automatically start on page load off.
7. Description: paste the description from the kit. Add the genre and up to ten tags from the kit.
8. AI generation disclosure: answer Yes, and tick Code and Text only. The games draw every shape and synthesise every sound in code, so Graphics and Sound stay unticked.
9. Cover image: upload store-assets/cover-630x500.png (click .game_edit_cover_uploader_widget .file_tools button). Screenshots: upload screenshot-1.png to screenshot-3.png. When uploading more than six screenshots at once, one can be dropped; upload six, then the rest.
10. Visibility: save as Draft first. Open the draft page and check it plays (see Verify). Only then switch visibility to Public and save again.
11. For an update of an existing game: upload the new zip as a new file, tick "played in the browser" on it, and hide the old upload instead of deleting it. Or push with butler to the same channel: ~/.local/share/butler/butler push <zip> pnscripts/<slug>:html5, then butler status pnscripts/<slug>.
12. Record the page URL, the upload id and the date in the game's docs/listing-itch.md and in DEV/ai-brain (the game's profile and business/pipeline.md).

## Verify

1. The page loads for a visitor (open it in a fresh tab, not the edit page) and shows the cover, three screenshots, the tags and the AI disclosure.
2. Find the embed frame: with drive_browser eval, read the src of the iframe on the game page. It points to https://html-classic.itch.zone/html/<upload id>/index.html (add ?v=<anything> to avoid a cached copy). Open that address directly and confirm with eval that the game's canvas exists and that the title screen draws, and read the console for errors.
3. Press Enter (dispatch a keydown) and confirm that the game starts.
4. Take a screenshot with drive_browser screenshot as evidence, and report the public URL.

## Never

- Never type a password or an API key. The owner signs in to itch.io and to butler himself.
- Never publish (switch to Public) without the owner's yes for this game, and never publish a paid page, a price, a bundle or a sale while the sales hold is active.
- Never delete old builds or uploads. Hide them, so that the download history and links stay intact.
- Never accept new terms, change payout settings or answer tax forms on the owner's behalf.
- Never tick Graphics or Sound in the AI disclosure for factory games, and never claim human review that did not happen.
- Never put secrets, tokens or personal data into descriptions, files or commits.
