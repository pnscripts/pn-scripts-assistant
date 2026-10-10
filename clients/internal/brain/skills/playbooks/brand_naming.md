name: brand_naming
version: 1
when: I ask how to name or rename a product, plugin, game, add-on, template or profile, the brand, a title, "PN Scripts X", "by PN Scripts", or whether a name is allowed
tools: read_file, search_files, web_search, fetch_url, edit_file
---

The owner approved this branding rule on 2026-10-10. It is recorded in DEV/ai-brain/business/accounts.md (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV).

The rule:
- A product that was named "PN X" becomes "PN Scripts X". Example: PN Omnibus became "PN Scripts Price History for Discounts".
- Every other product is "<Name> by PN Scripts", where <Name> is a short original word. Example: "Cleanlink by PN Scripts".
- Every public profile (itch.io, Game Jolt, Facebook, LinkedIn, YouTube, WordPress.org and the rest) is named "PN Scripts".
- Package metadata: Author "PN Scripts", Author URI https://pnscripts.com.
- Free games keep their own one-word title on game pages (for example on itch.io), and the studio name "PN Scripts" appears as the author.

## Before you start

1. Find every place the old name appears: search_files across the product's repository (headers, readme, package metadata, listing.json, screenshots' captions, docs), its profile in DEV/ai-brain/projects/, business/pipeline.md and the pnscripts.com repository (listings, migrations, menus, blog posts).
2. Read DEV/ai-brain/.ai/memory/lessons/20261010-wporg-rejects-trademark-or-generic-names.md if the product is a WordPress plugin.

## Steps

1. Pick the name. For a new word, search the web for it together with the product's field. Reject it if another product or service in that field already uses it, if it contains a trademark (WordPress, WooCommerce, Blender, Unity, Godot and so on as the leading word), or if it is a generic phrase that describes the feature only.
2. Check for a local clash: no folder of that name under DEV/Projects/pnscripts/marketplace and no listing on pnscripts.com.
3. Propose the name to the owner with the evidence of the search, and wait for his yes.
4. Apply it everywhere at once: the product's header, readme, package metadata, licence header if it names the product, listing.json (name and title), the screenshots and preview images that show the name (regenerate them), and the site page.
5. Slugs: keep existing slugs where a platform forbids a change (WordPress.org after approval). Where a slug changes on pnscripts.com, add a 301 redirect from the old URL, and ship it through a migration (see ship_pnscripts_com).
6. A GitHub repository rename needs the owner's yes; GitHub redirects the old URL, but update any Composer or Packagist references.
7. Record the rename in the product's profile in DEV/ai-brain with the date.

## Verify

- search_files for the old name returns only history (changelogs, profiles' logs, redirects), not live texts.
- The public pages show the new name, in both English and Bulgarian where the page has both.
- Screenshots no longer show the old name.

## Never

- Never use a trademark or another company's product name as part of a name.
- Never rename a published product, slug or repository without the owner's yes.
- Never break live URLs: redirect instead of deleting old pages.
- Never delete history, old releases or old tags because of a rename.
- Never type passwords, and never put secrets or personal data into names, files or commits.
