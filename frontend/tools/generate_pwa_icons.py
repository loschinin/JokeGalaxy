from math import cos, sin, sqrt
from pathlib import Path
import struct
import zlib


def png(path: Path, size: int) -> None:
    cx = cy = size / 2
    scale = size / 512
    rows = []
    angle = -0.48
    ca, sa = cos(angle), sin(angle)
    for y in range(size):
        row = bytearray([0])
        for x in range(size):
            dx, dy = x - cx, y - cy
            glow = max(0.0, 1.0 - sqrt(((x - .31 * size) / (.7 * size)) ** 2 + ((y - .23 * size) / (.7 * size)) ** 2))
            r = int(8 + 16 * glow)
            g = int(10 + 16 * glow)
            b = int(22 + 39 * glow)

            # Fine outer orbit and a brighter tilted orbit around the central star.
            rx, ry = ca * dx + sa * dy, -sa * dx + ca * dy
            norm = sqrt((rx / (.63 * size)) ** 2 + (ry / (.29 * size)) ** 2)
            orbit = abs(norm - 1) * (.29 * size)
            if orbit < max(1.2, 2.5 * scale):
                r, g, b = max(r, 129), max(g, 115), max(b, 187)
            norm2 = sqrt(((dx + 25 * scale) / (.45 * size)) ** 2 + ((dy - 12 * scale) / (.45 * size)) ** 2)
            orbit2 = abs(norm2 - 1) * (.45 * size)
            if orbit2 < max(1, 1.3 * scale):
                r, g, b = max(r, 82), max(g, 85), max(b, 128)

            radius = sqrt((dx / scale) ** 2 + (dy / scale) ** 2)
            if radius < 57:
                mix = max(0.0, min(1.0, 1 - radius / 57))
                r = int(255 - 42 * mix)
                g = int(211 - 48 * mix)
                b = int(112 + 92 * mix)
            if radius < 13:
                r, g, b = 255, 246, 223

            # Orbiting stars.
            for sx, sy, sr, color in [(377, 187, 16, (69, 229, 255)), (142, 336, 10, (245, 246, 255))]:
                star_dist = sqrt(((x - sx * scale) / scale) ** 2 + ((y - sy * scale) / scale) ** 2)
                if star_dist < sr:
                    r, g, b = color
            row.extend((r, g, b, 255))
        rows.append(bytes(row))
    raw = b"".join(rows)

    def chunk(kind: bytes, payload: bytes) -> bytes:
        return struct.pack(">I", len(payload)) + kind + payload + struct.pack(">I", zlib.crc32(kind + payload) & 0xffffffff)

    data = b"\x89PNG\r\n\x1a\n"
    data += chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0))
    data += chunk(b"IDAT", zlib.compress(raw, 9))
    data += chunk(b"IEND", b"")
    path.write_bytes(data)


if __name__ == "__main__":
    out = Path(__file__).resolve().parents[1] / "public"
    out.mkdir(parents=True, exist_ok=True)
    for name, size in [("icon-192.png", 192), ("icon-512.png", 512), ("apple-touch-icon.png", 180)]:
        png(out / name, size)
