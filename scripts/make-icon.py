"""Draws the application icon: the mark the core wears.

Generated rather than drawn by hand so the icon and the interface cannot drift
apart. Every number below is read off core3d.js — the same inverted triangle,
the same five iris rings at the same radii, the same pupil, and the colour the
core actually rests at. An icon that is merely "in the style of" the thing it
launches drifts the first time the thing changes; this one cannot, because
changing the core and not changing this is a visible mismatch in the same file.
"""
import math
import struct
import zlib
import os

SIZE = 512

# The dark the core sits in.
BG = (7, 11, 20)

# What the core rests at, from appearance.go: Idle "#3d8ce8".
#
# Brighter than the hex here, because everything in the core is additive and
# bloomed — the rings read as this colour only after several passes have piled
# up. Drawing the flat hex gives a dull navy nothing like what is on screen.
BLUE = (56, 140, 246)
HOT = (150, 200, 255)

# The core is built in units where the triangle reaches 1.9. Everything is
# expressed in those units and scaled once, so these numbers can be compared
# with core3d.js by eye.
REACH = 1.9
RING_RADII = [0.74 - i * 0.115 for i in range(5)]
RING_TUBES = [0.028 + i * 0.006 for i in range(5)]
PUPIL = 0.235

# How much of the icon the triangle spans. Larger than the core uses on screen:
# a launcher shows this at 48 pixels, where the margin a full window can afford
# is most of the picture.
SCALE = (SIZE * 0.47) / REACH


def canvas():
    return [[list(BG) for _ in range(SIZE)] for _ in range(SIZE)]


def blend(pixels, x, y, colour, alpha):
    if not (0 <= x < SIZE and 0 <= y < SIZE) or alpha <= 0:
        return
    alpha = min(1.0, alpha)
    p = pixels[y][x]
    for i in range(3):
        # Additive, like the light in the core itself, then clamped.
        p[i] = min(255, int(p[i] + colour[i] * alpha))


def ring(pixels, cx, cy, radius, tube, strength):
    """One iris band, with the falloff that makes it read as light."""
    steps = int(radius * 20) + 120
    reach = max(2, int(tube * 2.5))

    for i in range(steps):
        a = (i / steps) * math.tau
        ux, uy = math.cos(a), math.sin(a)

        for w in range(-reach, reach + 1):
            r = radius + w
            # A band is bright at its middle and falls away, which is what a
            # tube of glowing material looks like from the front.
            fade = math.exp(-(w * w) / (2 * tube * tube))
            colour = HOT if abs(w) <= tube * 0.5 else BLUE
            blend(pixels, int(cx + ux * r), int(cy + uy * r), colour,
                  strength * fade)


def disc(pixels, cx, cy, radius, colour):
    """Opaque, because additive light cannot make anything darker.

    Without something that actually occludes, the middle of the iris fills in
    and the eye closes — the same reason the core draws its pupil in front.
    """
    for y in range(int(cy - radius) - 2, int(cy + radius) + 3):
        for x in range(int(cx - radius) - 2, int(cx + radius) + 3):
            if not (0 <= x < SIZE and 0 <= y < SIZE):
                continue
            d = math.hypot(x - cx, y - cy)
            if d <= radius + 1:
                edge = max(0.0, min(1.0, radius - d))
                p = pixels[y][x]
                for i in range(3):
                    p[i] = int(p[i] * (1 - edge) + colour[i] * edge)


def line(pixels, x0, y0, x1, y1, width, strength):
    steps = int(max(abs(x1 - x0), abs(y1 - y0)) * 3) + 2
    for i in range(steps + 1):
        t = i / steps
        x, y = x0 + (x1 - x0) * t, y0 + (y1 - y0) * t
        for w in range(-width * 3, width * 3 + 1):
            for v in range(-width * 3, width * 3 + 1):
                d = math.hypot(w, v)
                fade = math.exp(-(d * d) / (2 * width * width))
                blend(pixels, int(x) + w, int(y) + v, BLUE, strength * fade)


def triangle(cx, cy, reach):
    """Pointing down, as the core's does."""
    out = []
    for i in range(3):
        a = -math.pi / 2 + (i / 3) * math.tau
        out.append((cx + math.cos(a) * reach, cy - math.sin(a) * reach))

    return out


def draw():
    pixels = canvas()
    cx = cy = SIZE / 2

    # The halo the core has behind it.
    #
    # Hollow in the middle, as core3d.js builds it: a glow brightest at its own
    # centre sits on top of the iris and closes the eye. Peaked out where the
    # rings are, it lights them from behind instead.
    for y in range(SIZE):
        for x in range(SIZE):
            d = math.hypot(x - cx, y - cy) / (SIZE / 2)
            lit = math.exp(-((d - 0.34) ** 2) / (2 * 0.20 ** 2))
            blend(pixels, x, y, BLUE, lit * 0.20)

    # Both triangles: the core draws one at its reach and one just inside it.
    for reach, strength, width in ((REACH, 0.75, 2), (REACH * 0.9, 0.4, 1)):
        corners = triangle(cx, cy, reach * SCALE)

        for i in range(3):
            x0, y0 = corners[i]
            x1, y1 = corners[(i + 1) % 3]
            line(pixels, x0, y0, x1, y1, width, strength)

    # The iris: five bands, the inner ones fatter and brighter, as on screen.
    # Distinct bands with dark between them: the first attempt made the tubes
    # nearly three times their real width, and five rings whose glow overlaps
    # its neighbours is not an iris, it is a white disc.
    for i, (radius, tube) in enumerate(zip(RING_RADII, RING_TUBES)):
        ring(pixels, cx, cy, radius * SCALE, tube * SCALE, 0.62 + i * 0.05)

    # The pupil and its hot rim.
    disc(pixels, cx, cy, PUPIL * SCALE, (8, 6, 10))
    ring(pixels, cx, cy, PUPIL * SCALE, 2.4, 0.9)

    return pixels


def shrink(pixels, size):
    """Box-average down to a smaller square.

    Written out rather than reached for from a library because this script has
    no dependencies on purpose: it runs on a fresh machine with nothing but
    python, which is the same promise the rest of the program makes.
    """
    step = SIZE / size
    out = []

    for y in range(size):
        row = []
        y0, y1 = int(y * step), max(int(y * step) + 1, int((y + 1) * step))

        for x in range(size):
            x0, x1 = int(x * step), max(int(x * step) + 1, int((x + 1) * step))
            totals = [0, 0, 0]
            n = 0

            for sy in range(y0, min(y1, SIZE)):
                for sx in range(x0, min(x1, SIZE)):
                    p = pixels[sy][sx]
                    for i in range(3):
                        totals[i] += p[i]
                    n += 1

            row.append([t // max(1, n) for t in totals])

        out.append(row)

    return out


def write_png(path, pixels, size):
    raw = b"".join(
        b"\x00" + bytes(v for px in row for v in px) for row in pixels
    )

    def chunk(tag, data):
        body = tag + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body))

    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(raw, 9))
    png += chunk(b"IEND", b"")

    os.makedirs(os.path.dirname(path), exist_ok=True)

    with open(path, "wb") as f:
        f.write(png)


full = draw()

# Written into the package that embeds them, so a single downloaded file can
# put itself in the applications menu with nothing else on disk beside it.
# Every size the icon theme asks for, drawn at full size and averaged down,
# because a launcher that has to scale 512 pixels into 48 makes a smear of
# five concentric rings.
ICONS = "clients/internal/brain/desktop/icons"

write_png(f"{ICONS}/pn-scripts-assistant-512.png", full, SIZE)

for size in (256, 128, 64, 48):
    write_png(f"{ICONS}/pn-scripts-assistant-{size}.png", shrink(full, size), size)

# One copy for the AppImage, which wants it beside the binary rather than
# inside it.
write_png("assets/pn-scripts-assistant.png", full, SIZE)

print(f"  written {SIZE}, 256, 128, 64, 48")
