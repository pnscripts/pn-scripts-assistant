---
name: audio-recording-cleanup
description: Record cleaner voice and instrument audio and repair existing recordings - room and microphone setup, gain staging, noise reduction, hum and click removal, de-essing, EQ, compression and loudness - in tools such as Audacity, Reaper, Audition or a DAW. Use when the user says the recording has background noise, hiss, hum, echo, clipping, plosives or sibilance, asks how to make their voice sound better, or wants settings for recording at home.
---

# Audio recording and cleanup

Prevention beats repair: fix the room and the gain first, then clean up gently. Effect names vary between programs: describe the effect type and tell the user to check their software's current manual.

## Steps
1. **Diagnose.** Ask for a short sample or a description: what is heard (hiss, hum, room echo, distortion, pops, harsh "s"), the microphone, the room and the software.
2. **Fix the recording setup (if they can re-record).**
   - Soft room: rugs, curtains, sofa, closet full of clothes; avoid bare walls and corners.
   - Mic 10-20 cm from the mouth, slightly off-axis, with a pop filter.
   - Turn off fans, fridge, notifications; record 5 seconds of room silence for noise profiles.
   - Gain: speech peaks around -12 dBFS, never touching 0 dBFS. Record at 48 kHz, 24-bit.
3. **Repair in a safe order** (on a copy, never the original):
   1. Cut or replace clipped parts; a declipper helps only mild clipping.
   2. Hum removal: notch at 50 Hz (Europe) or 60 Hz (Americas) and harmonics.
   3. Noise reduction using the room-tone profile, moderate amount (e.g. 6-12 dB). Listen for "underwater" artefacts.
   4. Click and plosive removal; high-pass filter around 70-100 Hz for voice.
   5. De-esser on harsh sibilance (roughly 5-8 kHz).
   6. EQ: small cuts first (muddiness around 200-400 Hz), small boosts for clarity (2-5 kHz) only if needed.
   7. Compression: ratio 2:1 to 4:1, a few dB of gain reduction, to even out levels.
   8. Loudness normalise to the target (see below), with a true-peak limiter.
4. **Compare A/B** at equal loudness, on headphones and on small speakers.

## Loudness targets (orientation, check the platform's current spec)
- Podcasts and spoken audio: often around -16 LUFS integrated (stereo), -1 dBTP true peak.
- Streaming video platforms: often normalise around -14 LUFS.
- Broadcast in Europe: EBU R128, -23 LUFS.

## Example
User: "My voice-over has a constant hiss and a fridge hum."
- Copy the file. Notch filter at 50 Hz plus 100 and 150 Hz.
- Select 2 seconds of silence, take a noise profile, reduce by about 10 dB.
- High-pass at 80 Hz, light compression 3:1, normalise to -16 LUFS, limiter at -1 dBTP.
- Check the pauses: if they sound unnatural, lower the reduction.

## Pitfalls
- Heavy noise reduction makes speech watery and metallic; less is better.
- Echo (reverb) from a bare room is hard to remove later; fix the room.
- Normalising peaks is not the same as loudness; use LUFS for perceived level.
- Every effect applied to the only copy is permanent; keep the raw file.
- Processing on laptop speakers hides low hum and harsh highs; use headphones.

## Output
Return the likely causes of the problem, a numbered processing chain with starting settings, recording tips if a re-record is possible, and the loudness target to use.
