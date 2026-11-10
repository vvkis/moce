#!/usr/bin/env python3
"""Build 墨册 icons from assets/icon/source.jpg.

Large sizes keep the painting. Small sizes thicken the book, flatten the
ink wash, and enlarge the seal so the taskbar can still read it.
"""
import io
import struct
from pathlib import Path

from PIL import Image, ImageFilter

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "assets" / "icon" / "source.jpg"
OUT = ROOT / "assets" / "icon"


def content_box(im: Image.Image):
    px = im.convert("RGB")
    w, h = px.size
    def paper(p):
        r, g, b = p
        return not (r > 248 and g > 248 and b > 248)
    top = next(y for y in range(h) if any(paper(px.getpixel((x, y))) for x in range(0, w, 3)))
    bot = next(y for y in range(h - 1, -1, -1) if any(paper(px.getpixel((x, y))) for x in range(0, w, 3)))
    left = next(x for x in range(w) if any(paper(px.getpixel((x, y))) for y in range(0, h, 3)))
    right = next(x for x in range(w - 1, -1, -1) if any(paper(px.getpixel((x, y))) for y in range(0, h, 3)))
    return left, top, right + 1, bot + 1


def rounded_mask(size, radius):
    mask = Image.new("L", (size, size), 0)
    rect = Image.new("L", (size, size), 255)
    # draw rounded rect by pasting circles and bars
    from PIL import ImageDraw
    d = ImageDraw.Draw(mask)
    d.rounded_rectangle((0, 0, size - 1, size - 1), radius=radius, fill=255)
    return mask


def seal_box(im: Image.Image):
    px = im.load()
    w, h = im.size
    pts = []
    for y in range(h):
        for x in range(w):
            r, g, b, a = px[x, y]
            if a > 200 and r > 140 and r > g + 35 and r > b + 35:
                pts.append((x, y))
    if not pts:
        return None
    xs = [p[0] for p in pts]
    ys = [p[1] for p in pts]
    return min(xs), min(ys), max(xs) + 1, max(ys) + 1


def lift_seal(im: Image.Image):
    """Return the seal crop and cover its old spot with the page above it."""
    box = seal_box(im)
    if not box:
        return im, None
    x0, y0, x1, y1 = box
    pad = 3
    x0, y0 = max(0, x0 - pad), max(0, y0 - pad)
    x1, y1 = min(im.width, x1 + pad), min(im.height, y1 + pad)
    seal = im.crop((x0, y0, x1, y1)).copy()
    height = y1 - y0
    src_y = max(0, y0 - height - 6)
    patch = im.crop((x0, src_y, x1, src_y + height))
    im.paste(patch, (x0, y0))
    return im, seal


def place_seal(im: Image.Image, seal: Image.Image, side_ratio: float):
    side = max(12, int(im.width * side_ratio))
    stamp = seal.resize((side, side), Image.Resampling.LANCZOS)
    margin = max(1, int(im.width * 0.06))
    im.alpha_composite(stamp, (im.width - side - margin, im.height - side - margin))
    return im


def simplify(master: Image.Image, seal: Image.Image, size: int):
    """Downscale the painting, darken the strokes, and put a larger seal back."""
    base = master.resize((size, size), Image.Resampling.LANCZOS)
    px = base.load()
    for y in range(size):
        for x in range(size):
            r, g, b, a = px[x, y]
            if a < 20 or (r > g + 28 and r > b + 28 and r > 120):
                continue
            lum = (r + g + b) / 3
            if lum < 145:
                k = 0.55
                px[x, y] = (int(r * k), int(g * k), int(b * k), a)
    if seal is not None:
        place_seal(base, seal, 0.42 if size <= 24 else 0.34)
    return base


def write_png_ico(path: Path, images):
    blobs = []
    for im in images:
        buf = io.BytesIO()
        im.save(buf, format="PNG")
        blobs.append((im.width, buf.getvalue()))
    count = len(blobs)
    offset = 6 + 16 * count
    out = struct.pack("<HHH", 0, 1, count)
    payload = b""
    for size, blob in blobs:
        w = 0 if size >= 256 else size
        h = 0 if size >= 256 else size
        out += struct.pack("<BBBBHHII", w, h, 0, 0, 1, 32, len(blob), offset)
        offset += len(blob)
        payload += blob
    path.write_bytes(out + payload)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    raw = Image.open(SRC).convert("RGBA")
    box = content_box(raw)
    cropped = raw.crop(box)
    side = max(cropped.size)
    canvas = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    canvas.paste(cropped, ((side - cropped.width) // 2, (side - cropped.height) // 2))
    big = canvas.resize((1024, 1024), Image.Resampling.LANCZOS)
    mask = rounded_mask(1024, 180)
    # knock out pixels outside the rounded square
    out = Image.new("RGBA", (1024, 1024), (0, 0, 0, 0))
    out.paste(big, (0, 0))
    bands = out.split()
    bands[3].paste(Image.composite(bands[3], Image.new("L", (1024, 1024), 0), mask))
    # rebuild alpha properly
    rgb = out.convert("RGB")
    rgba = rgb.convert("RGBA")
    rgba.putalpha(mask)
    plain, seal = lift_seal(rgba)
    master = plain.copy()
    if seal is not None:
        place_seal(master, seal, 0.16)
    master.save(OUT / "icon-1024.png")
    for size in (512, 256):
        master.resize((size, size), Image.Resampling.LANCZOS).save(OUT / f"icon-{size}.png")
    small = {}
    for size in (64, 48, 32, 24, 16):
        img = simplify(plain, seal, size) if size <= 48 else master.resize((size, size), Image.Resampling.LANCZOS)
        if size <= 48:
            img.save(OUT / f"icon-{size}.png")
        small[size] = img
    for size in (128, 256):
        small[size] = master.resize((size, size), Image.Resampling.LANCZOS)
    ico_sizes = [16, 24, 32, 48, 64, 128, 256]
    icons = []
    for size in ico_sizes:
        if size <= 48:
            icons.append(small.get(size) or simplify(plain, seal, size))
        else:
            icons.append(master.resize((size, size), Image.Resampling.LANCZOS))
    ico_path = ROOT / "build" / "windows" / "icon.ico"
    write_png_ico(ico_path, icons)
    master.save(ROOT / "build" / "appicon.png")
    logo = master.resize((128, 128), Image.Resampling.LANCZOS)
    logo.save(ROOT / "frontend" / "src" / "assets" / "logo.png")
    logo.save(ROOT / "frontend" / "public" / "favicon.png")
    (ROOT / "docs").mkdir(exist_ok=True)
    master.resize((256, 256), Image.Resampling.LANCZOS).save(ROOT / "docs" / "icon.png")
    print("wrote", ico_path, "and png sizes")


if __name__ == "__main__":
    main()
