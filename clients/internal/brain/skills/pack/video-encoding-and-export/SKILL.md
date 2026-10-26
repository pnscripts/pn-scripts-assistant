---
name: video-encoding-and-export
description: Choose export and encoding settings for video - container, codec (H.264, H.265/HEVC, AV1, ProRes), resolution, frame rate, bitrate or CRF, audio format, colour and HDR - and write FFmpeg commands for converting, compressing, trimming or extracting audio. Use when the user asks for the best export settings for YouTube, Instagram, TikTok, a website or a client, says the file is too big, blurry, out of sync or will not play, or wants an FFmpeg command.
---

# Video encoding and export

Pick settings that match the destination, not the biggest numbers. Platform specs change: tell the user to check the platform's current upload recommendations before a final export.

## Steps
1. **Identify the destination.** Upload platform, website embed, client delivery, archive, or further editing. Ask about the source: resolution, frame rate, HDR or SDR, length.
2. **Keep source resolution and frame rate** unless the destination needs something else. Never convert 25 fps to 30 fps (or back) without a reason; it adds judder.
3. **Choose the codec.**
   - Upload and general delivery: H.264 in MP4 (works everywhere).
   - Smaller files at equal quality: H.265/HEVC or AV1, if the player or platform supports them.
   - Further editing or a master: an intermediate codec (ProRes, DNxHR) or a high-bitrate file.
4. **Choose quality control.** Constant quality (CRF) for files; target bitrate for platforms with limits or streaming. With x264, CRF 18-23 is a typical range (lower is better and bigger).
5. **Audio.** AAC 48 kHz, 192-320 kbps stereo for delivery; uncompressed PCM for masters. Normalise loudness before export.
6. **Colour.** SDR Rec.709 for normal delivery; keep HDR only if the whole chain supports it. Watch for washed-out or too-dark output caused by full/limited range mismatches.
7. **Check the result.** Play the file in a different player, check sync at the end, look at dark gradients for banding, and confirm file size and duration.

## FFmpeg examples
```
# Compress for sharing, good quality, web-friendly start
ffmpeg -i input.mov -c:v libx264 -crf 20 -preset slow -pix_fmt yuv420p -c:a aac -b:a 192k -movflags +faststart output.mp4

# Downscale to 1080p, keep aspect ratio
ffmpeg -i input.mp4 -vf "scale=-2:1080" -c:v libx264 -crf 21 -c:a copy output_1080p.mp4

# Trim without re-encoding (cuts land on keyframes)
ffmpeg -ss 00:01:30 -to 00:02:45 -i input.mp4 -c copy clip.mp4

# Extract audio as WAV
ffmpeg -i input.mp4 -vn -c:a pcm_s16le audio.wav

# Image sequence to video at 25 fps
ffmpeg -framerate 25 -i frame_%04d.png -c:v libx264 -crf 18 -pix_fmt yuv420p render.mp4
```

## Pitfalls
- Re-encoding an already compressed file again and again loses quality each time; export from the project or original.
- Missing `-pix_fmt yuv420p` can give files that some players and phones refuse to play.
- Stream copy (`-c copy`) cuts only on keyframes, so the start may be off by a second; re-encode for frame-accurate cuts.
- Variable frame rate phone footage can drift out of sync; convert to constant frame rate before editing.
- A higher bitrate than the platform re-encodes to brings no benefit, only a slower upload.

## Output
Return the recommended settings as a short table (container, codec, resolution, frame rate, quality or bitrate, audio) with a one-line reason, and the exact FFmpeg command or editor export preset when the user needs it.
