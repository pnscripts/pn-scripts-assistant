---
name: blender-rendering
description: Choose a render engine and settings in Blender and produce clean stills or animations - Cycles vs Eevee, samples and denoising, GPU setup, resolution, output formats, render passes and speed fixes. Use when the user asks how to render, says "the render is noisy / slow / black / takes hours", wants to render an animation, needs a transparent background, or asks which output format to choose.
---

# Blender rendering

Get the best image in the time available. Settings move between Blender versions: tell the user to check the official manual for their version if a field is not where you expect.

## Steps
1. **Ask the essentials.** Still or animation, final size and use (web, print, video), deadline, hardware (GPU model, RAM) and Blender version.
2. **Pick the engine.**
   - Cycles: path tracing, accurate light, glass and shadows; slower. Best for product shots and realism.
   - Eevee: real-time, fast; good for stylised work, previews and long animations. Needs more manual setup for reflections and shadows.
   - Workbench: viewport-style previews and clay renders.
3. **Enable the GPU.** Preferences > System: choose CUDA or OptiX (NVIDIA), HIP (AMD), Metal (Apple), oneAPI (Intel); then in Render Properties set Device to GPU Compute.
4. **Set samples and denoising (Cycles).** Use Noise Threshold (adaptive sampling) around 0.01-0.05 with a max sample cap, and turn on the denoiser (OpenImageDenoise or OptiX). Raise samples only if denoising smears detail.
5. **Set output.** Resolution and percentage; frame range and frame rate for animation. For animation render to an image sequence (PNG or OpenEXR), never straight to a video file, then encode afterwards.
6. **Transparency.** Film > Transparent, and save as PNG (RGBA) or EXR; JPEG has no alpha.
7. **Passes for compositing.** Enable passes such as Denoising Data, Mist, AO, Cryptomatte when the user will adjust in the compositor or another tool.
8. **Test small.** Render a crop region (Ctrl+B in the camera view) or 25 % resolution before the full render.

## Speed checklist
- Light paths: lower max bounces (e.g. 4-6 total) when there is little glass.
- Caustics off unless needed; clamp indirect light (about 10) to kill fireflies.
- Simplify panel: cap subdivision levels and texture size for previews.
- Persistent Data for animations in Cycles, so the scene is not rebuilt each frame.
- Hide unseen objects from render; replace distant detail with simpler meshes.
- Check memory: GPU running out of VRAM fails or falls back; reduce textures or render on CPU.

## Example
User: "Product shot, 3000x2000 for print, render is grainy after 20 minutes."
- Cycles on GPU, Noise Threshold 0.02, max samples 1024, OpenImageDenoise on.
- Clamp indirect 10, bounces 6, caustics off.
- Output 16-bit PNG or TIFF; test a crop of the label first.

## Pitfalls
- Black render: camera missing or inside an object, light strength too low, or World strength 0.
- Rendering animation directly to MP4: a crash loses everything and quality suffers; render frames, then encode.
- Fireflies (white dots) come from small bright lights and glossy bounces; clamp and enlarge the lights.
- Denoising flickers in animation unless denoising passes and temporal approaches are used; test a short range.

## Output
Return the engine choice with a one-line reason, a list of exact settings (device, samples, threshold, denoiser, bounces, resolution, format) and a test-render plan. For a problem, name the cause and the fix.
