---
name: photo-editing-workflow
description: Guide a non-destructive photo editing workflow - import and culling, RAW development, exposure, white balance, colour, local adjustments, retouching, noise and sharpening, and export for web, social media or print - in apps such as Lightroom, Darktable, RawTherapee, GIMP, Snapseed or a phone editor. Use when the user asks how to edit a photo, fix colours or a dull picture, batch-edit a shoot, or export the right size and format.
---

# Photo editing workflow

Edit in a fixed order so later steps do not undo earlier ones. Keep the original untouched and work non-destructively.

## Steps
1. **Back up and organise.** Copy the card to two places before editing; folders by date and event (`2026-05-14 Wedding`); keep RAW and JPEG pairs together.
2. **Cull fast.** One pass to reject (blurred, eyes closed, duplicates), one pass to pick the best; star or flag them. Edit only the picks.
3. **Global corrections, in this order:**
   1. Lens profile corrections and chromatic aberration removal.
   2. Crop and straighten.
   3. White balance: pick a neutral grey or white area with the eyedropper, then adjust by eye.
   4. Exposure: set mid-tones first, then highlights down, shadows up, whites and blacks to just short of clipping (hold Alt/Option while dragging where the app supports it).
   5. Contrast and tone curve: gentle S-curve for punch.
   6. Colour: vibrance before saturation; use HSL to calm one loud colour or fix skin (orange hue, lower saturation).
4. **Local adjustments.** Graduated filter to darken a bright sky, brush or AI mask for the subject (+exposure, +clarity) and background (−exposure), light vignette to hold the eye.
5. **Retouch.** Spot healing for dust and blemishes; keep skin texture (no strong smoothing); remove distractions only where it stays believable.
6. **Noise and sharpening.** Noise reduction first, then sharpening with a mask so only edges are sharpened; check at 100 % zoom.
7. **Sync and check.** Copy settings to similar shots, then fix each one; compare before/after (backslash key in many apps); look at the edit on a second screen or phone.
8. **Export for the destination.**

## Export presets
| Destination | Format | Size | Colour |
|---|---|---|---|
| Web and email | JPEG quality 80–85 | long edge 2048 px | sRGB |
| Social feed | JPEG quality 85–90 | 1080 px wide (4:5 → 1080×1350) | sRGB |
| Print lab | JPEG quality 95–100 or TIFF | print size × 300 ppi | sRGB or the lab's profile |
| Archive or further editing | TIFF 16-bit or original RAW | full | Adobe RGB or ProPhoto |

## Example
Backlit portrait, face too dark, sky white: white balance from the grey shirt, exposure +0.7, highlights −80, shadows +40, subject mask +0.3 exposure, sky mask −0.5, HSL orange saturation −10, noise reduction 20, sharpening with masking 60, export 1080×1350 sRGB JPEG.

## Pitfalls
- Editing on a screen at full brightness makes prints come out dark; lower the screen brightness or calibrate.
- Over-processing signs: halos around edges, grey "HDR" look, neon skies, plastic skin. Step every slider back by about a third at the end.
- Exporting in Adobe RGB or ProPhoto for the web shows dull colours in many viewers; use sRGB.
- Destructive edits in a pixel editor without layers cannot be undone after saving; duplicate the layer first.
- Removing or adding objects in news, documentary or competition photos may break the rules; check them.

## Output
Hand back the edit plan as ordered steps with suggested slider directions (and starting values where helpful), the export settings for the user's destination, and any warning about over-editing.
