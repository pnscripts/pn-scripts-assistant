name: update_social_profiles
version: 1
when: I ask to update, fill in, refresh or rename the social profiles, page or channel on Facebook, LinkedIn, YouTube, itch.io, Game Jolt or WordPress.org, the bio, display name, cover, banner, avatar, category or links from the social kit
tools: drive_browser, read_file, list_directory, search_files
---

The social kit is DEV/Projects/pnscripts/marketing/social-kit (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV). It holds profiles.md (display names, bios in EN and BG with character counts, categories, links, pinned posts per platform), handles.md (which handles are free or taken), growth-plan.md (posting rhythm and community rules), avatars/, banners/ and previews/ (how each crop looks). The current status of every profile is in DEV/ai-brain/business/accounts.md.

Every public profile is named "PN Scripts". Paid items are Coming soon: no prices and no buy links in any bio or pinned post.

## Before you start

1. Read the platform's section in profiles.md and its row in handles.md. Use the texts exactly as written; the counts were checked against each platform's limit by source/profiles.py.
2. Choose the right image: avatars/pnscripts-avatar-square-1080.png for Facebook, LinkedIn, Game Jolt and itch.io; avatars/pnscripts-avatar-circle-safe-1080.png for round-crop platforms such as YouTube. Banners: facebook-cover-1640x624.png, linkedin-cover-1128x191.png, youtube-banner-2560x1440.png.
3. Open the platform with drive_browser. If a sign-in page appears, stop and ask the owner to sign in in that window. Creating new accounts is the owner's job.
4. Tell the owner which profile you will change and get his yes, because profile edits are public.

## Steps

1. Facebook page: display name "PN Scripts"; category Video Game Developer, then Software Company; intro from profiles.md; links to pnscripts.com, pnscripts.itch.io and games.pnscripts.com; cover image; profile picture. Do not try to take a username that belongs to someone else; use the fallbacks listed in handles.md.
2. LinkedIn company page: name, tagline (EN, or BG for a Bulgarian audience), industry Software Development, website, logo and cover, and the company-page fields listed in profiles.md.
3. YouTube: channel name "PN Scripts", handle as in handles.md, description EN and BG, links, banner (check youtube-safe-area.png), category Gaming.
4. itch.io (https://itch.io/user/settings and the profile page): display name, avatar, profile text and links. The profile text follows the Facebook intro and the YouTube description.
5. Game Jolt: display name "PN Scripts", avatar, bio and links. A username change to pnscripts is free; ask the owner before changing it, because it changes profile URLs.
6. WordPress.org profile (profiles.wordpress.org): display name "PN Scripts", website, short bio. Plugin names follow brand_naming.
7. After each platform, open the public page in a fresh tab and compare it with the kit.
8. Update the status table in DEV/ai-brain/business/accounts.md (display name, status, date), without any login details.

## Verify

- For each changed platform: a screenshot of the public view, and the visible name, bio and links quoted back and compared with profiles.md.
- Images are not cropped badly (compare with the previews/ folder).
- No bio or pinned post contains a price, a store link or "buy".

## Never

- Never type passwords or 2FA codes, and never create accounts; the owner does both.
- Never post, publish or change a public profile without the owner's yes.
- Never claim a handle that belongs to someone else, and never use another company's name or logo.
- Never put prices, buy links or paid offers into profiles while the sales hold is active.
- Never delete old posts, pages or channels; edit or hide instead.
- Never write phone numbers, private email addresses, personal addresses or other personal data into a public profile unless the owner gives the exact text himself.
