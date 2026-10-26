---
name: game-ready-3d-export
description: Prepare and export 3D models from Blender or similar tools for game engines such as Godot, Unity and Unreal - scale and axes, polygon budgets, UVs, texture baking, LODs, collision, rigs and animations, glTF or FBX settings. Use when the user asks how to export a model to a game engine, the model imports too big, rotated, black, missing textures or animations, or they need a game-ready asset checklist.
---

# 3D export for games

Turn a model into a light, correct, engine-ready asset. Exporter options and engine importers change between versions: tell the user to check the current Blender manual and the engine's import documentation for their versions.

## Steps
1. **Ask the target.** Engine and version, platform (mobile, desktop, web), role of the asset (hero, prop, background) and whether it is static, skinned or animated.
2. **Set a budget.** Rough triangle counts as orientation, not rules: small mobile props a few hundred to a few thousand; desktop hero characters tens of thousands. Texture size to match screen size (512-2048 px typical).
3. **Clean the mesh.** Apply all transforms (Ctrl+A > All Transforms), apply or keep modifiers deliberately, merge by distance, recalculate normals, delete hidden and interior faces.
4. **Scale and axes.** Model in metres at real size (1 unit = 1 m). glTF uses Y-up and the exporter converts; for FBX set Forward and Up axes per engine and apply unit scale. Origin at the base or pivot point.
5. **UVs and textures.** One non-overlapping UV map for baking (UV1), optional second map for lightmaps. Bake high-poly detail to normal maps; pack textures as Base Color, Normal and an ORM or roughness/metal map as the engine expects.
6. **Materials.** Keep to Principled BSDF with image textures; procedural nodes do not export and must be baked.
7. **LOD and collision.** Make LOD meshes (e.g. 100 %, 50 %, 25 %) with Decimate or by hand; build simple collision shapes (boxes, convex hulls) and name them as the engine's importer expects.
8. **Rigs and animation.** Single armature, apply scale before skinning, limit bone influences to 4 per vertex, name actions clearly, use NLA or action export options to include each clip.
9. **Export.** Prefer glTF 2.0 (.glb) for Godot and web; FBX is common for Unity and Unreal. Export selected only, include custom normals, tangents if needed, and animations.
10. **Verify in the engine.** Import, check scale against a 1 m cube or default capsule, lighting, textures, animation playback and collision.

## Game-ready checklist
- [ ] Transforms applied, origin correct, real-world scale
- [ ] Triangle count and texture size within budget
- [ ] Clean, non-overlapping UVs; textures baked
- [ ] Normal map in Non-Color and the right convention (OpenGL vs DirectX green channel)
- [ ] Clear names for meshes, materials, bones and animations
- [ ] LODs and collision prepared
- [ ] Imported and tested in the engine

## Pitfalls
- Model imports 100x too big or small: unit scale mismatch; fix in exporter or apply scale.
- Model lies on its side: axis settings for FBX.
- Black or missing textures: procedural materials, unpacked image paths, or wrong colour space.
- Inverted lighting on bumps: normal map green channel convention is flipped for the engine.
- Animations missing: actions not assigned or not included in the export options.

## Output
Return the export settings for the user's engine, a filled-in game-ready checklist for the asset, and the fix for any import problem described.
