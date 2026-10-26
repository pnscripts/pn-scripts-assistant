---
name: blender-materials-and-lighting
description: Set up physically based materials and scene lighting in Blender - Principled BSDF values, image textures and UVs, HDRI and three-point lighting, exposure and colour management. Use when the user asks how to make something look like metal, glass, wood or plastic, says "my render looks flat / too dark / plastic", has stretched textures, wants studio or outdoor lighting, or needs a material from photo textures.
---

# Blender materials and lighting

Make surfaces believable and light them with intent. Node and panel names change between Blender versions: tell the user to check the official manual for their version when a setting is not where you describe it.

## Steps
1. **Ask the context.** Render engine (Eevee or Cycles), what the object is made of, the mood (product studio, daylight, night, moody interior) and the Blender version.
2. **Start from Principled BSDF.** Set Base Color, Metallic, Roughness first; most realistic looks come from these three.
3. **Pick values that match reality.**
   - Metal: Metallic 1, Base Color is the tint of the metal (no pure black), Roughness 0.2-0.5.
   - Plastic or paint: Metallic 0, Roughness 0.3-0.6.
   - Glass or water: Transmission 1, Roughness near 0, IOR about 1.45-1.52 for glass, 1.33 for water.
   - Rubber, fabric, raw wood: Metallic 0, Roughness 0.6-0.9.
   - Never use pure white (1.0) or pure black (0.0) as Base Color; stay roughly within 0.03-0.9.
4. **Add textures.** UV unwrap first (mark seams, then Unwrap) and check with a checker texture. Connect Base Color image in sRGB; Roughness, Metallic and Normal maps as Non-Color. Normal map goes through a Normal Map node.
5. **Light the scene.**
   - Quick realism: an HDRI on the World (Environment Texture), adjust strength and rotation.
   - Studio: three-point lighting with area lights. Key light at about 45 degrees, fill on the opposite side at roughly a quarter to half the key's strength, rim light behind to separate the subject.
   - Bigger lights give softer shadows; distance changes intensity (inverse square).
6. **Set colour management.** Use the default view transform (AgX in recent versions, Filmic in older ones), not Standard, for realistic renders. Adjust Exposure rather than cranking every light.
7. **Check in rendered viewport.** Compare against a real reference photo of the same material.

## Example
User: "My gold ring looks like yellow plastic."
- Metallic to 1, Base Color a warm gold (about RGB 1.0, 0.77, 0.34), Roughness 0.25.
- Add an HDRI with clear bright spots so the metal has something to reflect; metals look dull in empty scenes.
- Optional: a subtle noise texture into Roughness for wear.

## Pitfalls
- Metallic values between 0 and 1 are rarely physically correct; use them only for transitions (dirt, worn paint).
- Roughness or normal maps left in sRGB look wrong and too strong.
- Stretched textures mean a bad or missing UV map, or unapplied object scale.
- Flat render: one light from the camera direction; move it to the side and add a rim light.
- Eevee needs extra settings for reflections, refraction and shadows that Cycles handles automatically; check the current manual for the version.

## Output
Return the material settings as a short node list with values, the lighting setup (light types, positions, strengths) and the colour management settings, plus one tip for checking the result against a reference.
