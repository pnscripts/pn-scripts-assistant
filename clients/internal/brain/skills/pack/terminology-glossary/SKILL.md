---
name: terminology-glossary
description: Build or update a bilingual English-Bulgarian terminology glossary for a product, company, project or subject area - approved terms, forbidden variants, definitions, part of speech, context and do-not-translate items - so that every translation and text stays consistent. Use when the user says "make a glossary", "which Bulgarian term should we use for", "keep terms consistent", starts a translation project, or shares documents or UI strings whose key terms need fixing.
---

# Terminology glossary

One concept, one approved term per language. A glossary saves more time than any later review.

## Steps
1. **Define scope.** Product or subject, languages (EN-BG), audience, and who approves terms. Ask for existing style guides, previous translations and competitor or standard vocabularies the field already uses.
2. **Extract candidates.** From the source texts take: product and feature names, UI labels, domain terms, abbreviations, recurring phrases, and words that are often mistranslated. Skip ordinary words.
3. **Research each term.**
   - How do Bulgarian users and official sources in that field say it? Prefer established usage over a new coinage.
   - For legal, tax or medical terms use the wording of the official Bulgarian source (law, regulator, standard), and note it.
   - Check whether the English term is better left untranslated (brand names, widely used terms like Wi-Fi, Bluetooth, SQL).
4. **Decide and record** one approved term, forbidden variants, and a short definition.
5. **Add usage notes.** Gender and plural for Bulgarian nouns, the verb to pair with ("натиснете бутона", not "кликнете бутона" if the style guide says so), capitalisation, and an example sentence.
6. **Review with a subject expert** or the user for disputed entries; mark status (approved / proposed / rejected).
7. **Maintain it.** Date every change; never silently change an approved term - update existing texts or note the transition.

## Glossary template
| EN term | BG term | Part of speech | Definition / context | Forbidden | Notes | Status |
|---|---|---|---|---|---|---|
| account | профил | noun, m. | user account in the app | акаунт | plural: профили | approved |
| sign in | вход / влезте | noun / verb | button and action | логване, логин | button: „Вход“ | approved |
| invoice | фактура | noun, f. | tax document | инвойс | | approved |
| dashboard | табло | noun, n. | main overview screen | дашборд | | proposed |
| Wi-Fi | Wi-Fi | - | | уайфай | do not translate | approved |

## Checklist
- Each concept has exactly one approved term per language.
- Synonyms used in the past are listed as forbidden, so search-and-replace can find them.
- Do-not-translate list exists (brand, product, feature names, code terms).
- Bulgarian entries show gender and plural where useful.
- Each disputed entry has a reason and an owner.
- The file is easy to import (CSV or spreadsheet with the columns above).

## Pitfalls
- Collecting every word; a glossary of 2 000 common words is ignored. Keep it to terms that matter.
- Choosing a technically correct term nobody uses; real users must recognise it.
- Different translators adding their own variants without approval.
- Translating the same English word the same way in all contexts ("record" can be запис, рекорд or записвам).
- Leaving anglicisms by habit (апдейт, сетинги) when a good Bulgarian word exists (обновяване, настройки).

## Output
Return the glossary as a table (and as CSV if asked), a list of open questions for approval, and a short rule set: register, do-not-translate items, preferred verbs for UI actions.
