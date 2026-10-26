---
name: localisation-review
description: Review an existing translation or localised product (website, app UI, game, email templates, help pages) for accuracy, natural language, consistency, formats, cultural fit and broken placeholders or layout, and return a prioritised list of fixes. Use when the user asks to "check this translation", "review the Bulgarian version", "does this sound natural", shares source and target files (.po, .json, .xliff, .strings, spreadsheets) or screenshots of a translated interface.
---

# Localisation review

A good review finds what a native user would trip over, ranks it, and proposes the exact fix.

## Steps
1. **Collect the material.** Source and target text side by side, the glossary or style guide if any, screenshots for UI, target market (e.g. Bulgaria, UK, US) and audience.
2. **Check technical integrity first** (these break the product):
   - every placeholder present and unchanged: `{name}`, `%s`, `%1$d`, `{{count}}`, `<b>…</b>`
   - plural forms filled for every category the target language needs
   - no truncated strings, no untranslated keys, no source text left in the target
   - encoding correct (no `Ð°Ð±` mojibake), right-to-left not involved for EN/BG
3. **Check meaning.** Compare each segment with the source: omissions, additions, wrong sense, mistranslated negation, numbers and units changed.
4. **Check language.** Grammar, spelling, punctuation, agreement, articles, natural word order, one consistent register (ти or Вие; you formal or casual).
5. **Check consistency.** The same term translated the same way everywhere; button names in help text match the actual buttons.
6. **Check locale formats.** Dates, times, decimal separator, thousands separator, currency position, quotation marks, units (metric), phone and address formats.
7. **Check fit and culture.** Text length in buttons and menus, idioms and jokes that do not work, images or examples that make no sense locally, legal wording (consumer rights, cookie notices) adapted, not just translated.
8. **Rate and report.** Give each issue a severity and a concrete corrected text.

## Severity scale
- **Critical** - breaks function or changes meaning in a harmful way (lost placeholder, wrong price, wrong legal promise).
- **Major** - clearly wrong or confusing to a user (wrong term, grammar error in a visible place, untranslated text).
- **Minor** - unnatural or inconsistent but understandable (style, word choice, punctuation).
- **Preference** - optional improvement; do not count it as an error.

## Issue table template
| # | Location / key | Source | Current | Suggested | Type | Severity |
|---|---|---|---|---|---|---|
| 1 | checkout.button | Place order | Постави поръчка | Поръчай | Terminology | Major |
| 2 | cart.items | {count} items | {count} артикул | {count} артикула | Plural | Major |
| 3 | footer.date | 10/26/2026 | 10/26/2026 | 26.10.2026 | Locale format | Minor |

## Pitfalls
- Reviewing only the target text: meaning errors hide behind fluent language. Always compare with the source.
- Rewriting everything to personal taste; mark style changes as Preference.
- Missing context: a single word like "Open" can be a verb (Отвори) or an adjective (Отворено). Ask for the screen.
- Fixing a string in one file and forgetting its duplicates elsewhere.
- Ignoring length: a correct translation that overflows a button is still a bug.

## Output
Return a short verdict (ready / ready after fixes / needs retranslation), counts per severity, the issue table, and recurring patterns the translator should fix globally (for example "use Вие everywhere", "decimal comma").
