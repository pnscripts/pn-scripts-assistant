---
name: translate-en-to-bg
description: Translate English text into natural, correct Bulgarian - emails, web pages, UI strings, documents, marketing copy, instructions - keeping meaning, tone and formatting while applying Bulgarian conventions for quotes, numbers, dates, register and grammar. Use when the user says "translate to Bulgarian", "how do I say this in Bulgarian", pastes English text for a Bulgarian audience, or shares a file (.docx, .md, .po, .json strings) that needs a Bulgarian version.
---

# Translate English to Bulgarian

Translate meaning, not words. The result must read as if a careful Bulgarian writer wrote it first.

## Steps
1. **Clarify the brief.** Ask only if unclear: audience (customers, colleagues, officials), register (formal Вие or informal ти), purpose (legal, marketing, UI, casual), and any glossary or brand terms to keep.
2. **Read the whole text first.** Note terms, names, product names, repeated phrases and anything ambiguous before translating line by line.
3. **Translate by sentence and idea.** Restructure when English word order sounds foreign in Bulgarian. Prefer verbs to noun chains ("upon completion of the installation" → „след като инсталацията приключи“).
4. **Apply Bulgarian conventions** (checklist below).
5. **Keep what must stay.** Code, commands, variables (`{name}`, `%s`, `{{count}}`), URLs, file names, brand names and trademarks stay unchanged. Keep markup and placeholders in the right place for Bulgarian word order.
6. **Read the Bulgarian alone.** Without looking at the source: does it flow, is the register consistent, are agreements of gender and number correct?
7. **Mark doubts.** List terms where several translations are possible and say which one you chose and why.

## Bulgarian conventions checklist
- Quotes: „така“ (low-high), nested ‚така‘ or «така» used consistently.
- Decimal comma and space for thousands: 1 234,56 (not 1,234.56).
- Dates: 26.10.2026 or 26 октомври 2026 г.; months and weekdays in lower case.
- Time: 14:30 ч. (24-hour clock).
- Currency after the amount: 25 лв., 10 евро, 15 €; check which currency the audience uses now.
- Full and short definite article: full „-ът/-ят“ for the subject (Клиентът плати), short „-а/-я“ otherwise (на клиента).
- Register: Вие with capital В in letters to one person; вие in general texts; never mix ти and Вие in one text.
- Capitals: only the first word in titles and headings (Как да започнем, not Как Да Започнем).
- Dash with spaces – for a pause; hyphen without spaces in compounds (онлайн-магазин or онлайн магазин, follow current spelling rules and be consistent).
- Commas before „че“, „който“, „за да“, „но“, „ако“ clauses.

## False friends and traps
| English | Wrong | Better |
|---|---|---|
| actual | актуален | действителен, реален |
| eventually | евентуално | в крайна сметка, накрая |
| sympathetic | симпатичен | съчувстващ, разбиращ |
| ultimately | ултимативно | в крайна сметка |
| control (verify) | контролирам | проверявам |
| application (job) | апликация | кандидатура, заявление |
| please | моля (in every sentence) | once is enough; use a polite verb form |

## Example
Source: "Your order has been shipped and will arrive in 2-3 business days. Track it here."
Bad: „Вашата поръчка е била изпратена и ще пристигне в 2-3 бизнес дни. Проследете я тук.“
Good: „Поръчката Ви е изпратена и ще пристигне до 2–3 работни дни. Можете да я проследите тук.“

## Pitfalls
- Passive chains („беше извършено от“) where Bulgarian uses an active verb or „се“.
- Possessive overload („Вашият профил, Вашата парола“): Bulgarian often drops it or uses the short form „профила Ви“.
- Translating idioms literally ("piece of cake" is „лесна работа“, not „парче торта“).
- UI strings: Bulgarian is often 20-30 % longer; warn if a button or label has a hard length limit.
- Plurals with numbers: „2 файла“ (counting form for masculine nouns), „5 потребители“ for persons.

## Output
Return the Bulgarian text in the same format as the source, then a short note with terms you chose, placeholders kept, and any questions about ambiguous parts.
