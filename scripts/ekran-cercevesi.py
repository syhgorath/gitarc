#!/usr/bin/env python3
"""README için: bir ekran görüntüsünü (PNG) tarayıcı penceresi çerçevesine alıp tek dosyalık SVG yapar.
GitHub, <img> ile gösterilen SVG içindeki gömülü (data:) görüntüyü çizer; dış dosya yüklemez, bu yüzden PNG gömülür.
Kullanım: scripts/ekran-cercevesi.py girdi.png cikti.svg [adres]   (varsayılan adres: 127.0.0.1:7432)
Yalnızca standart kütüphane."""
import base64
import struct
import sys


def png_size(data: bytes):
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise SystemExit("girdi bir PNG değil")
    return struct.unpack(">II", data[16:24])


def frame(png: bytes, address: str) -> str:
    w, h = png_size(png)
    pad, bar, r = 28, 40, 12
    W, H = w + 2 * pad, h + bar + 2 * pad
    b64 = base64.b64encode(png).decode()
    x, y = pad, pad
    return f'''<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="{W}" height="{H}" viewBox="0 0 {W} {H}">
  <defs>
    <filter id="g" x="-10%" y="-10%" width="120%" height="125%"><feDropShadow dx="0" dy="10" stdDeviation="14" flood-color="#000" flood-opacity="0.35"/></filter>
    <clipPath id="c"><rect x="{x}" y="{y}" width="{w}" height="{h + bar}" rx="{r}"/></clipPath>
  </defs>
  <rect x="{x}" y="{y}" width="{w}" height="{h + bar}" rx="{r}" fill="#0d1117" filter="url(#g)"/>
  <g clip-path="url(#c)">
    <rect x="{x}" y="{y}" width="{w}" height="{bar}" fill="#1f2630"/>
    <rect x="{x}" y="{y + bar - 1}" width="{w}" height="1" fill="#30363d"/>
    <circle cx="{x + 22}" cy="{y + bar / 2}" r="6" fill="#ff5f57"/>
    <circle cx="{x + 42}" cy="{y + bar / 2}" r="6" fill="#febc2e"/>
    <circle cx="{x + 62}" cy="{y + bar / 2}" r="6" fill="#28c840"/>
    <rect x="{x + w / 2 - 170}" y="{y + 8}" width="340" height="{bar - 16}" rx="{(bar - 16) / 2}" fill="#0d1117" stroke="#30363d"/>
    <text x="{x + w / 2}" y="{y + bar / 2 + 4.5}" text-anchor="middle" font-family="ui-monospace,Menlo,Consolas,monospace" font-size="13" fill="#8b949e">🔒 {address}</text>
    <image x="{x}" y="{y + bar}" width="{w}" height="{h}" xlink:href="data:image/png;base64,{b64}"/>
  </g>
  <rect x="{x}" y="{y}" width="{w}" height="{h + bar}" rx="{r}" fill="none" stroke="#30363d"/>
</svg>
'''


if __name__ == "__main__":
    if len(sys.argv) < 3:
        raise SystemExit(__doc__)
    src, dst = sys.argv[1], sys.argv[2]
    addr = sys.argv[3] if len(sys.argv) > 3 else "127.0.0.1:7432"
    with open(src, "rb") as f:
        data = f.read()
    with open(dst, "w", encoding="utf-8") as f:
        f.write(frame(data, addr))
