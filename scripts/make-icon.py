"""Draws the application icon: the mark the core wears.

Generated rather than drawn by hand so the icon and the interface cannot drift
apart — the geometry here is the same inverted triangle and iris the core is
built from, at the sizes a desktop asks for.
"""
import math
import struct
import zlib

SIZE = 512
BG = (7, 11, 20)
GLOW = (255, 62, 22)


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


def ring(pixels, cx, cy, radius, width, colour, strength):
    steps = int(radius * 14) + 60
    for i in range(steps):
        a = (i / steps) * math.tau
        for w in range(-width, width + 1):
            r = radius + w * 0.5
            x = cx + math.cos(a) * r
            y = cy + math.sin(a) * r
            fade = 1 - abs(w) / (width + 1)
            blend(pixels, int(x), int(y), colour, strength * fade)


def line(pixels, x0, y0, x1, y1, colour, strength, width=2):
    steps = int(max(abs(x1 - x0), abs(y1 - y0)) * 2) + 2
    for i in range(steps + 1):
        t = i / steps
        x = x0 + (x1 - x0) * t
        y = y0 + (y1 - y0) * t
        for w in range(-width, width + 1):
            for v in range(-width, width + 1):
                fade = 1 - (abs(w) + abs(v)) / (2 * width + 2)
                blend(pixels, int(x) + w, int(y) + v, colour, strength * fade)


def disc(pixels, cx, cy, radius, colour):
    for y in range(int(cy - radius) - 2, int(cy + radius) + 3):
        for x in range(int(cx - radius) - 2, int(cx + radius) + 3):
            d = math.hypot(x - cx, y - cy)
            if d <= radius and 0 <= x < SIZE and 0 <= y < SIZE:
                edge = min(1.0, radius - d)
                p = pixels[y][x]
                for i in range(3):
                    p[i] = int(p[i] * (1 - edge) + colour[i] * edge)


def draw():
    pixels = canvas()
    cx = cy = SIZE / 2

    # The wash the light sits in.
    for y in range(SIZE):
        for x in range(SIZE):
            d = math.hypot(x - cx, y - cy) / (SIZE / 2)
            glow = max(0.0, 1 - d) ** 3 * 0.5
            blend(pixels, x, y, GLOW, glow * 0.75)

    # The iris, sized to sit inside the triangle rather than burst out of it.
    for i in range(5):
        ring(pixels, cx, cy, 112 - i * 18, 3, GLOW, 0.5 + i * 0.06)

    # The pupil, which has to occlude rather than add.
    disc(pixels, cx, cy, 34, (10, 8, 14))
    ring(pixels, cx, cy, 34, 2, GLOW, 0.85)

    # The triangle, pointing down.
    reach = 232
    corners = []
    for i in range(3):
        a = -math.pi / 2 + (i / 3) * math.tau
        corners.append((cx + math.cos(a) * reach, cy - math.sin(a) * reach))

    for i in range(3):
        x0, y0 = corners[i]
        x1, y1 = corners[(i + 1) % 3]
        line(pixels, x0, y0, x1, y1, GLOW, 0.7, 1)

    return pixels


def write_png(path, pixels):
    raw = b"".join(
        b"\x00" + bytes(v for px in row for v in px) for row in pixels
    )

    def chunk(tag, data):
        body = tag + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body))

    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", SIZE, SIZE, 8, 2, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(raw, 9))
    png += chunk(b"IEND", b"")

    with open(path, "wb") as f:
        f.write(png)


write_png("assets/pn-brain.png", draw())
print("  written")
