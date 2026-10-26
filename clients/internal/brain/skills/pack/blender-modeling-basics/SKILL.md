---
name: blender-modeling-basics
description: Guide a beginner or intermediate user through modeling an object in Blender - blocking out shapes, edit-mode tools, modifiers, clean topology, scale and naming - and diagnose common mesh problems. Use when the user asks how to model something in Blender, says "my mesh looks wrong", has shading artefacts, flipped normals, overlapping vertices, a bevel or subdivision that misbehaves, or wants to plan a model from reference images.
---

# Blender modeling basics

Help the user build clean, correctly scaled models step by step. Menu names and shortcuts shift between Blender versions: when unsure, tell the user to check the official Blender manual for their version (Help > Manual).

## Steps
1. **Clarify the goal.** Ask: what object, for what use (still render, animation, game, 3D print), target detail level and Blender version. A game asset needs a low polygon budget; a print needs a closed, manifold mesh.
2. **Set up the scene.** Unit system in Scene Properties (metric, metres). Load reference images (front and side) as reference or background images and align them to real dimensions.
3. **Block out.** Start from primitives (cube, cylinder, UV sphere) at real-world size. Get proportions right with large shapes before any detail.
4. **Refine in Edit Mode.** Core tools: extrude (E), inset (I), loop cut (Ctrl+R), bevel (Ctrl+B), knife (K), merge (M). Work in vertex, edge or face select mode as needed.
5. **Use modifiers non-destructively.** Mirror for symmetric objects (enable clipping), Subdivision Surface for smooth forms, Bevel for edge highlights, Solidify for thickness, Array for repeats. Keep the stack in that logical order and apply only when needed.
6. **Clean up.** Apply scale (Ctrl+A > Scale) before bevels, modifiers and export. Merge by distance to remove doubles. Recalculate normals outside (Shift+N). Turn on the Face Orientation overlay: blue is outside, red is inside.
7. **Shade.** Use Shade Smooth plus auto smooth or the "Smooth by Angle" modifier (the name depends on version), or add support loops near hard edges for subdivision.
8. **Name and organise.** Give objects and materials clear names, group parts in collections, set the origin where the object will pivot.

## Topology checklist
- Mostly quads where the mesh will deform or subdivide; triangles are fine on flat, static areas.
- No n-gons on curved surfaces under subdivision.
- Even edge flow following the shape; poles (5+ edges) placed on flat areas.
- No interior faces, zero-area faces or loose vertices (Select > Select All by Trait > Loose Geometry / Non Manifold).
- Polygon count fits the purpose; check Statistics in the viewport overlays.

## Example
User: "Model a simple mug."
1. Cylinder, 16-24 sides, scale to about 8 cm tall and 8 cm wide, apply scale.
2. Delete the top face, inset the rim, extrude down for the inner wall, Solidify or manual thickness.
3. Handle: select two faces on the side, extrude along a curve, or bridge edge loops.
4. Subdivision level 2, add support loops at the rim and base, Shade Smooth.

## Pitfalls
- Unapplied scale makes bevel, solidify and physics behave unevenly.
- Modelling at the wrong scale breaks lighting, depth of field and physics later.
- Subdivision on a mesh with n-gons and triangles gives pinching.
- Black patches or odd shading usually mean flipped normals or duplicate vertices.
- Applying the Mirror modifier too early doubles every later edit.

## Output
Give a numbered build plan for the object, the modifiers to use with key settings, and a short cleanup checklist. For a problem, give the likely cause and the exact fix.
