name: build_products
version: 1
when: I ask to build, make or generate new products for sale, a product factory wave, Blender add-ons, plugins, templates, spreadsheets, sound or texture packs, fonts, tools, their previews, listing.json or the EN/BG product texts
tools: run_command, read_file, write_file, edit_file, list_directory, search_files, web_search, do_in_background
---

The workspace root is DEV, the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV. Products live in DEV/Projects/pnscripts/marketplace/<folder>, one local git repository each. The durable copy of the factory specification is DEV/ai-brain/projects/marketplace/product-factory-wave-1.md; read it, and DEV/ai-brain/business/pipeline.md, before proposing anything.

## Before you start

1. Check the machine load and the number of running builders (see machine_throttling).
2. Make sure the idea is not a near-duplicate of the catalogue (Blender add-ons, retro texture packs, sound packs, business spreadsheets, Pterodactyl extensions, WooCommerce plugins, FAQ accordions, HTML5 game templates and so on). Search the pipeline and profiles.
3. Pick a short original brand name, one word if possible, with no trademark in it. Check the web for the name. The visible title is "<Name> by PN Scripts" (see brand_naming).
4. Agree the list with the owner: name, category, what it does, planned price.

## Steps

1. git init -b main in the new folder. Work only inside it.
2. Build the real product, working and complete: no stubs, no TODO features, no mock-ups of features that do not exist. Add a build step (Makefile, build.sh or build.py) that writes the distributable to dist/, which is gitignored. Keep git small (under about 5 MB; no zips or binaries in git).
3. Write automated tests (make test or a documented command) and make them pass. Say in the README which versions were actually tested and nothing more.
4. README.md: what it is, features, requirements, install, usage, build, test, licence, "Made by PN Scripts (https://pnscripts.com)", and the AI disclosure: "Created with AI assistance; <what was checked> by PN Scripts with <how>."
5. LICENSE: GPL-3.0-or-later for Blender add-ons and WordPress or WooCommerce plugins; the PN Scripts Commercial Licence for everything else. CHANGELOG.md with the 1.0.0 entry.
6. Preview images in listing/gallery/: four to six JPEGs at 1600 x 900 or 1600 x 1000, quality about 85, each under 350 KB, kebab-case names. Generate them from the real thing (renders, screenshots, specimen sheets). The first is the featured image. For headless screenshots use google-chrome --headless=new with its own --user-data-dir inside the product's build directory.
7. listing/listing.json with every required key: slug, name, title, planned_price_usd, pricing_model, category (one of the allowed category slugs), type_slugs, platform_slugs, license_term, product type and deployment model in EN and BG, tech_stack, compatibility, and the en and bg blocks (lead, three description paragraphs, meta_description ending "Coming soon.", problem, solution, positioning, value, five to eight key features, three use cases, two audiences, exactly four chapters, exactly the four FAQ questions) plus the gallery entries with alt texts and captions in both languages. The Bulgarian must be natural, not word for word, with product and software names in Latin script.
8. The product is paid and Coming soon: never name a store, never write a price, a date or "buy now" in the copy. The price goes only in planned_price_usd.
9. Validate: python3 -c "import json;json.load(open('listing/listing.json'))". Run the tests and the build once more.
10. Commit by explicit paths (git add <paths>), never git add -A.
11. A GitHub repository, if the owner wants one, is created private under the pnscripts organisation with no Actions workflows (see github_cost_control). Ask before creating it.
12. Record the product in DEV/ai-brain (the family profile and business/pipeline.md) and run the brain check.

## Verify

- The test command passes, and you quote its result line.
- dist/ holds the distributable; listing/gallery/ holds the images, and you have looked at the featured one.
- listing.json parses, and its meta descriptions end with "Coming soon." and "Очаквайте скоро."
- git log shows the commit; git status is clean.

## Never

- Never publish, list or sell anything paid: the sales hold applies (see sales_hold).
- Never add .github/workflows to a private repository, and never push without the owner's yes.
- Never copy icons, sounds, code or texts from elsewhere; never use trademarks in names; never invent reviews, statistics or tested versions.
- Never run more than two or three builders, or more than one heavy job, at once.
- Never type passwords, and never put secrets or personal data into files, listings or commits.
- Never delete an existing product folder or its history.
