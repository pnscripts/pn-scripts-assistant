name: sales_hold
version: 1
when: I ask whether something can be sold, priced, listed as paid, put on sale, monetised with ads or revenue share, published on Gumroad, itch.io paid pages, CrazyGames, Game Jolt partner or Newgrounds, or about the sales hold, "Coming soon", VAT or art. 97a
tools: read_file, search_files
---

The owner placed a sales hold on 2026-10-08. It is recorded in DEV/ai-brain/business/accounts.md (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV); read that file first, because only the owner can lift the hold and the file will say so when he does.

The rule: everything paid or revenue-earning stays "Coming soon" until the company's VAT registration under art. 97a of the Bulgarian VAT Act and an accountant are in place. Creating products and games continues; free games and free items may be published.

What counts as revenue:
- Paid products, paid pages, bundles, sales and pay-what-you-want with a minimum, on any store (Gumroad, itch.io, BuiltByBit and others).
- Ad and revenue-share portals: CrazyGames ads, the Game Jolt partner system and anything similar that pays per play or per view.
- Donations or tips enabled on a page, sponsorships and affiliate links.

Platform rules that apply regardless of the hold:
- Newgrounds bans AI-generated games: do not publish there at all.
- Superhive and Envato were not pursued because of their AI policies.

## Before you start

1. Read the sales hold row and the withdrawal rule in DEV/ai-brain/business/accounts.md. Check the date of the latest entry.
2. If the owner asks to sell something, remind him of the hold in one sentence and ask whether he has lifted it. Only his explicit statement lifts it.

## Steps

1. For a new paid product: build it fully, but list it as Coming soon. On pnscripts.com that means no buy button, no card price, no JSON-LD Offer, and the hold texts in EN and BG. In listing copy: never a store name, a price, a date or "buy now". The planned price lives only in planned_price_usd.
2. For a free game: publish it free (see publish_itch_io and publish_game_jolt), with donations off, the Game Jolt partner system off, and no ads.
3. For a paid source template that goes with a free game: keep it Coming soon and do not upload it as a paid page.
4. If you find a paid page that is public: do not change it on your own; report it to the owner with the URL and ask whether to set it to Draft.
5. When the owner lifts the hold: follow his instructions. The site has a reversible migration for this (described in DEV/ai-brain/projects/pnscripts/pnscripts.com.md); lifting it is a new migration that calls its down(), shipped with ship_pnscripts_com.

## Verify

- Visitor check (a fresh tab, not signed in): paid pages answer 404 or show Coming soon; no price or buy button is visible.
- Game Jolt game settings show the partner system off; itch.io free games have payments off.

## Never

- Never publish, re-enable or sell anything paid, and never turn on ads or revenue share, until the owner lifts the hold.
- Never request a payout or withdrawal from any platform; payouts are paused by the owner's withdrawal rule.
- Never fill in tax forms, VAT fields, bank details or payout methods; the owner does that himself.
- Never give tax or legal advice as fact; say that the accountant must confirm.
- Never publish on Newgrounds.
- Never type passwords, and never write VAT numbers, bank details or other company or personal data into files or messages.
