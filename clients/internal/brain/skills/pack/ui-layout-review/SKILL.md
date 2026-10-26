---
name: ui-layout-review
description: Review a screen, web page, app view or mock-up for layout quality - visual hierarchy, alignment, spacing, grouping, consistency and responsive behaviour - and give prioritised, concrete fixes. Use when the user shares a screenshot, Figma frame, URL or HTML/CSS and asks "does this look good", "why does it feel cluttered", "review my design", "make it look more professional", or before launching a landing page or app screen.
---

# UI layout review

Judge the layout against the screen's main task, not personal taste. Every remark must name the element, the problem and the fix.

## Steps
1. **Learn the goal.** Ask who uses the screen and the one action it must drive (sign up, buy, find an answer). Note the device: desktop, phone, both.
2. **Squint test.** Blur or zoom out the screenshot. The first three things that stand out should be, in order: what this is, the key content, the main action. If something else wins, hierarchy is wrong.
3. **Check hierarchy.**
   - One clear primary button per view; secondary actions look secondary.
   - Heading sizes step down consistently (for example 32 / 24 / 18 / 16 px).
   - Important information sits top-left (for left-to-right languages) or at the top on mobile.
4. **Check alignment and grid.** Elements line up on a few shared edges; one column grid (12 columns on desktop, 4 on phone is common). Flag anything off by a few pixels.
5. **Check spacing.** Use a spacing scale (4 or 8 px steps). Related items close together, groups separated by more space than items inside them. Enough padding inside cards and buttons.
6. **Check density and grouping.** Too many borders and boxes? Replace with whitespace. Long forms split into logical groups. Lines of body text about 50-80 characters.
7. **Check consistency.** Same component looks and behaves the same everywhere: buttons, inputs, icons, corner radius, shadows.
8. **Check responsiveness.** At 360-400 px wide: no horizontal scrolling, tap targets at least about 44x44 px, text at least 16 px for body, navigation reachable, nothing important hidden.
9. **Check states.** Empty, loading, error, long text, very long names, many items, no image.
10. **Prioritise.** Sort findings: blocks the task > hurts clarity > polish.

## Findings template
| # | Severity | Element | Problem | Fix |
|---|----------|---------|---------|-----|
| 1 | High | "Continue" button | same style as "Cancel", primary action unclear | filled accent colour for Continue, text button for Cancel |
| 2 | Medium | pricing cards | 12 px gap between cards, 24 px inside: groups blur together | 32 px gap between cards |
| 3 | Low | footer links | three different grey tones | one token, e.g. `--text-muted` |

## Pitfalls
- Redesigning everything when three fixes would do; respect the existing brand.
- Judging only the desktop screenshot; most traffic may be mobile.
- Centring long text blocks; centre only short headings.
- Using placeholder text as the only label in forms.
- Fixing visual issues while ignoring that the main action is below the fold.

## Output
Return a short verdict (what works, the biggest problem), the prioritised findings table with concrete values (sizes, spacing, colours), and, if helpful, CSS snippets or an annotated description of the improved layout.
