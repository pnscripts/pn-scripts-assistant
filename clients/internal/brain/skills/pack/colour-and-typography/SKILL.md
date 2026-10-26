---
name: colour-and-typography
description: Choose or fix a colour palette and type system for a website, app, presentation or brand - primary and accent colours, neutrals, contrast, dark mode, font pairing, type scale, line length and spacing - and deliver them as ready-to-use values or CSS variables. Use when the user asks "which colours go together", "pick fonts for my site", "my design looks amateur", "make a dark mode", needs brand colours, or text looks hard to read.
---

# Colour and typography

Build a small, consistent system instead of picking colours and fonts one element at a time. Readability comes before style.

## Steps
1. **Collect constraints.** Existing logo or brand colours, audience, tone (calm, playful, premium, technical), where it is used (screen, print, both), languages and scripts needed (Cyrillic, Greek, accents).
2. **Build the palette.**
   - One primary (brand, main buttons), one accent at most, and a neutral grey ramp of about 8-10 steps for text, borders and backgrounds.
   - Semantic colours: success, warning, error, info - each with a light background tint and a strong text tone.
   - Rough share of use: about 60 % neutrals/background, 30 % secondary surfaces, 10 % primary and accent.
3. **Check contrast.** Text at least 4.5:1 on its background, large text and UI borders at least 3:1. Test every text/background pair you actually use, including white text on the primary button.
4. **Dark mode.** Do not just invert. Use dark grey (not pure black) backgrounds, slightly lighter surfaces for raised elements, desaturated and lighter versions of the primary, and recheck all contrasts.
5. **Choose fonts.**
   - One family is often enough; two at most (for example a serif for headings and a sans-serif for text).
   - Check the font covers every needed script and weight, has a licence that allows the use (web, app embedding, print), and loads fast (2-3 weights).
   - Good system fallback stack: `system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`.
6. **Set the type scale.** Body 16-18 px on screens; a ratio of about 1.2-1.25 between levels; line height 1.4-1.6 for body and 1.1-1.3 for headings; line length 50-80 characters.
7. **Deliver as tokens.** Name by role, not by colour, so the theme can change later.

## Example tokens
```css
:root {
  --color-primary: #1a5fb4;   --color-on-primary: #ffffff;
  --color-text: #1f2328;      --color-text-muted: #59636e;
  --color-bg: #ffffff;        --color-surface: #f6f8fa;   --color-border: #d0d7de;
  --color-error: #b42318;     --color-success: #1a7f37;
  --font-body: "Inter", system-ui, sans-serif;
  --text-sm: 0.875rem; --text-base: 1rem; --text-lg: 1.25rem; --text-xl: 1.563rem; --text-2xl: 1.953rem;
  --leading-body: 1.5;
}
@media (prefers-color-scheme: dark) {
  :root { --color-bg: #16181d; --color-surface: #1f232a; --color-text: #e6e8eb;
          --color-text-muted: #a3acb6; --color-primary: #78aeed; --color-on-primary: #0b1a2e; }
}
```
Verify every pair with a contrast checker before shipping; the values above are a starting point.

## Pitfalls
- Grey text on grey backgrounds and light placeholder text that fails contrast.
- Too many brand colours used as decoration, so nothing signals "clickable".
- Red/green as the only difference between states.
- Fonts without Cyrillic or other needed letters fall back to a different font mid-word.
- All caps and very thin weights for body text; justified text on narrow screens.

## Output
Return the palette with hex values and roles, contrast results for the main pairs, font choice with fallback and licence note, the type scale, and the whole system as CSS variables (or a table for non-web tools).
