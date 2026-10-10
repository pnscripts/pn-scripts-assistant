name: wordpress_org_plugin_review
version: 1
when: I ask to submit a plugin to WordPress.org, rename a plugin, answer the plugin review team, check a plugin name for trademark or generic-name risk, run Plugin Check or prepare readme.txt, the slug, SVN or screenshots
tools: run_command, read_file, edit_file, search_files, web_search, fetch_url, drive_browser, read_email, read_mailbox, send_email
---

PN Scripts publishes WordPress and WooCommerce plugins on WordPress.org under the pnscripts account. Sources of truth: DEV/ai-brain/.skills/platforms/wordpress/directory-submission.md, DEV/ai-brain/checklists/marketplace-release.md and the lesson DEV/ai-brain/.ai/memory/lessons/20261010-wporg-rejects-trademark-or-generic-names.md (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV).

The review team rejected a coined name that an unrelated service in the same field already used, even with "PN Scripts" in front. Generic phrases such as "Product Sidebars" are also refused. Plugins are named under the brand rule: "PN Scripts <original word>" (see brand_naming).

## Before you start

1. Read the plugin's profile in DEV/ai-brain/projects/marketplace/ and its repository's README and readme.txt.
2. Plugin Check and two-factor authentication on the author account are mandatory for new submissions. The owner manages the account and its 2FA.
3. The slug is generated from the Plugin Name header at submission, can be changed once before review and never after approval. Decide it first: slug = folder = text domain = main file name, with no "wordpress" in it, and no trademark as the first word.

## Steps

1. Name check. Search the web (not only the plugin directory) for the proposed word together with the plugin's field. Reject it if a product or service in the same field uses it, if it contains a platform mark first (WordPress, WooCommerce), or if it is a generic phrase.
2. Check all four places where the name appears and make them agree: the plugin header and readme.txt name; the Plugin URI; the screenshots (retake them if they show an old name); and the site page on pnscripts.com (listing, slug, 301 redirects from old URLs, shipped through ship_pnscripts_com).
3. readme.txt: Stable tag equals the Version header and the release tag; Tested up to is a version that was really tested; licence GPL-2.0-or-later or GPL-3.0-or-later; short description under 150 characters.
4. Run Plugin Check on a local WordPress: wp plugin install plugin-check --activate, then wp plugin check <slug>. Zero errors; fix warnings where reasonable. Keep the output as evidence.
5. Build the zip from the repository's build step and test install it on a clean local site.
6. Submission: the owner submits at https://wordpress.org/plugins/developers/add/ while signed in, or you fill in the form with drive_browser after he has signed in and has confirmed. Uploading is a submission: ask first.
7. Reviewer emails: find the thread with read_mailbox or read_email (search for the plugin name or "plugins@wordpress.org"). Draft a reply that answers every point in order, states what was changed and in which version, and attaches or links the new zip if asked. Reply in the same email thread (keep the subject with "Re:" and reply to the review address), never as a new message, because the review team tracks threads. Show the draft to the owner and send only after his yes.
8. After approval, release through SVN as directory-submission.md describes (trunk, tags/<version>, assets/), only with the owner's yes.
9. Record the outcome in the plugin's profile and add a lesson if the review taught something new (see update_ai_brain).

## Verify

- Plugin Check reports zero errors, and you quote the summary.
- The four name places agree (quote each).
- The reply appears in the same thread in the sent folder, or the submission confirmation page or email exists.
- After approval, https://wordpress.org/plugins/<slug>/ shows the right name, version and screenshots.

## Never

- Never type the owner's WordPress.org password or 2FA codes.
- Never submit, reply to reviewers or commit to SVN without the owner's yes for that step.
- Never start a new email thread for a review reply, and never argue with the review team; fix and explain.
- Never change a slug after approval, and never delete old SVN tags.
- Never put secrets, keys or personal data into plugin code, readme.txt, screenshots or commits.
