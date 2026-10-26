---
name: accessibility-check
description: Check a web page, app screen, document or design for accessibility against WCAG 2.2 level AA - contrast, keyboard use, screen reader labels, headings, forms, images, motion and zoom - and give concrete fixes. Use when the user asks "is this accessible", "check WCAG", "does it work with a screen reader", needs to meet an accessibility law or public-sector requirement, shares HTML, a URL or a screenshot for review, or gets an accessibility complaint or audit.
---

# Accessibility check

Check what real users with low vision, colour blindness, motor limits or screen readers will meet. Automated tools find only part of the problems; combine them with manual checks.

## Steps
1. **Scope.** Which pages or screens, which platform, and which standard (WCAG 2.2 AA is the usual target; national laws based on the European Accessibility Act refer to it). Check the current official text when the user needs legal compliance.
2. **Automated scan.** Run a browser checker (for example Lighthouse or axe in developer tools) and note every issue with its element.
3. **Keyboard.** Unplug the mouse mentally: Tab through the page.
   - Every link, button and field is reachable in a logical order.
   - The focus outline is always visible.
   - Menus, dialogs and carousels open, work and close with Enter, Space, Esc and arrows; no keyboard traps.
   - A "Skip to content" link exists on pages with long navigation.
4. **Contrast and colour.** Body text at least 4.5:1, large text (about 24 px, or 19 px bold) and icons/borders of controls at least 3:1. Information is never given by colour alone (errors also have text or an icon).
5. **Structure for screen readers.**
   - One `h1`, headings in order without skipped levels; landmarks (`header`, `nav`, `main`, `footer`).
   - Images: meaningful `alt` text; decorative images `alt=""`.
   - Buttons and links say what they do ("Download invoice PDF", not "Click here"); icon-only buttons have `aria-label`.
   - Page `lang` attribute set; page title describes the page.
6. **Forms.** Every field has a visible `<label>`; required fields marked in text; errors explained next to the field and announced; no time limits without a way to extend.
7. **Zoom and reflow.** At 200 % zoom and at 320 px width, content reflows without horizontal scrolling or overlapping. Text spacing can be increased without breaking.
8. **Motion and media.** Respect `prefers-reduced-motion`; nothing flashes more than 3 times per second; videos have captions, audio has a transcript, autoplay can be paused.
9. **Test with a screen reader** for the main task (NVDA on Windows, VoiceOver on macOS/iOS, TalkBack on Android).

## Common fixes
```html
<button aria-label="Close dialog"><svg aria-hidden="true">...</svg></button>
<label for="email">Email</label>
<input id="email" type="email" autocomplete="email" aria-describedby="email-err">
<p id="email-err" role="alert">Enter an address like name@example.com</p>
```
```css
:focus-visible { outline: 3px solid #1a5fb4; outline-offset: 2px; }
@media (prefers-reduced-motion: reduce) { * { animation: none !important; transition: none !important; } }
```

## Pitfalls
- A perfect automated score does not mean accessible; keyboard and screen reader checks are required.
- ARIA added to the wrong element makes things worse; prefer native HTML elements (`button`, `a`, `label`).
- `outline: none` without a replacement focus style.
- Placeholder text instead of a label; it disappears and often has low contrast.
- Accessibility overlay widgets do not fix the underlying code.

## Output
Return a findings list grouped by severity (blocks a task / makes it hard / minor), each with the WCAG criterion number, the element, the user impact and the exact fix, plus what you could not test and should be checked manually.
