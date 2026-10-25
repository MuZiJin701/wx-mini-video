#!/usr/bin/env python3
"""把 build/uipreview 生成的 ANSI 文本渲染为 PNG 文档预览图。

用法：
    python poster/ansi_to_png.py build/preview/home.ansi docs/images/首页.png

这是文档配图生成脚本，不参与程序运行。渲染使用 Cascadia Mono、微软雅黑和
Segoe UI Symbol；不同终端的字体与像素格可能略有差异。
"""

from __future__ import annotations

import argparse
import re
import unicodedata
from dataclasses import dataclass
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

CELL_W = 9
CELL_H = 19
PADDING = 12

DEFAULT_FG = (222, 224, 227)
DEFAULT_BG = (40, 62, 44)
BASE_BG = (30, 36, 32)

SGR_RE = re.compile(r"\x1b\[([0-9;]*)m")
CSI_RE = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]")
OSC_RE = re.compile(r"\x1b\].*?(?:\x07|\x1b\\)")

FONT_DIR = Path(r"C:\Windows\Fonts")
MONO = FONT_DIR / "CascadiaMono.ttf"
MONO_BOLD = FONT_DIR / "CascadiaMono.ttf"
CJK = FONT_DIR / "msyh.ttc"
CJK_BOLD = FONT_DIR / "msyhbd.ttc"
SYMBOL = FONT_DIR / "seguisym.ttf"

PALETTE_256 = {
    0: (0, 0, 0),
    1: (128, 0, 0),
    2: (0, 128, 0),
    3: (128, 128, 0),
    4: (0, 0, 128),
    5: (128, 0, 128),
    6: (0, 128, 128),
    7: (192, 192, 192),
    8: (128, 128, 128),
    9: (255, 0, 0),
    10: (0, 255, 0),
    11: (255, 255, 0),
    12: (0, 0, 255),
    13: (255, 0, 255),
    14: (0, 255, 255),
    15: (255, 255, 255),
}


@dataclass
class Style:
    fg: tuple[int, int, int] = DEFAULT_FG
    bg: tuple[int, int, int] = DEFAULT_BG
    bold: bool = False
    underline: bool = False


def color_256(index: int) -> tuple[int, int, int]:
    if index in PALETTE_256:
        return PALETTE_256[index]
    if 16 <= index <= 231:
        index -= 16
        levels = [0, 95, 135, 175, 215, 255]
        r = levels[index // 36]
        g = levels[(index % 36) // 6]
        b = levels[index % 6]
        return (r, g, b)
    gray = 8 + (index - 232) * 10
    return (gray, gray, gray)


def apply_sgr(style: Style, params: str) -> Style:
    codes = [int(p) for p in params.split(";") if p != ""] or [0]
    i = 0
    while i < len(codes):
        code = codes[i]
        if code == 0:
            style = Style()
        elif code == 1:
            style.bold = True
        elif code == 4:
            style.underline = True
        elif code == 22:
            style.bold = False
        elif code == 24:
            style.underline = False
        elif 30 <= code <= 37:
            style.fg = color_256(code - 30)
        elif code == 39:
            style.fg = DEFAULT_FG
        elif 40 <= code <= 47:
            style.bg = color_256(code - 40)
        elif code == 49:
            style.bg = DEFAULT_BG
        elif 90 <= code <= 97:
            style.fg = color_256(code - 90 + 8)
        elif 100 <= code <= 107:
            style.bg = color_256(code - 100 + 8)
        elif code == 38 and i + 1 < len(codes):
            if codes[i + 1] == 2 and i + 4 < len(codes):
                style.fg = (codes[i + 2], codes[i + 3], codes[i + 4])
                i += 4
            elif codes[i + 1] == 5 and i + 2 < len(codes):
                style.fg = color_256(codes[i + 2])
                i += 2
        elif code == 48 and i + 1 < len(codes):
            if codes[i + 1] == 2 and i + 4 < len(codes):
                style.bg = (codes[i + 2], codes[i + 3], codes[i + 4])
                i += 4
            elif codes[i + 1] == 5 and i + 2 < len(codes):
                style.bg = color_256(codes[i + 2])
                i += 2
        i += 1
    return style


def char_width(ch: str) -> int:
    return 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1


def tokenize(line: str) -> list[tuple[str, Style, int]]:
    cells: list[tuple[str, Style, int]] = []
    style = Style()
    i = 0
    while i < len(line):
        if line[i] == "\x1b":
            match = SGR_RE.match(line, i)
            if match:
                style = apply_sgr(style, match.group(1))
                i = match.end()
                continue
            for pattern in (CSI_RE, OSC_RE):
                match = pattern.match(line, i)
                if match:
                    i = match.end()
                    break
            else:
                i += 1
            continue
        ch = line[i]
        i += 1
        if ch == "\r":
            continue
        cells.append((ch, style, char_width(ch)))
    return cells


def load_font(path: Path, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(str(path), size)


def render(input_path: Path, output_path: Path) -> None:
    text = input_path.read_text(encoding="utf-8")
    lines = text.split("\n")
    while lines and lines[-1] == "":
        lines.pop()

    width_cells = max((sum(w for _, _, w in tokenize(line)) for line in lines), default=0)
    width = PADDING * 2 + width_cells * CELL_W
    height = PADDING * 2 + len(lines) * CELL_H
    image = Image.new("RGB", (width, height), BASE_BG)
    draw = ImageDraw.Draw(image)

    mono = load_font(MONO, 14)
    mono_bold = load_font(MONO_BOLD, 14)
    cjk = load_font(CJK, 14)
    cjk_bold = load_font(CJK_BOLD, 14)
    symbol = load_font(SYMBOL, 14)

    for row, line in enumerate(lines):
        col = 0
        y0 = PADDING + row * CELL_H
        for ch, style, cell_w in tokenize(line):
            x0 = PADDING + col * CELL_W
            draw.rectangle([x0, y0, x0 + cell_w * CELL_W - 1, y0 + CELL_H - 1], fill=style.bg)
            if ch.strip():
                if cell_w == 2:
                    font = cjk_bold if style.bold else cjk
                elif ord(ch) < 128:
                    font = mono_bold if style.bold else mono
                else:
                    font = symbol
                bbox = font.getbbox(ch)
                glyph_h = bbox[3] - bbox[1]
                ty = y0 + (CELL_H - glyph_h) // 2 - bbox[1]
                draw.text((x0, ty), ch, font=font, fill=style.fg)
                if style.underline:
                    draw.line([x0, y0 + CELL_H - 3, x0 + cell_w * CELL_W - 2, y0 + CELL_H - 3], fill=style.fg)
            col += cell_w

    output_path.parent.mkdir(parents=True, exist_ok=True)
    image.save(output_path)
    print(f"{input_path.name} -> {output_path} ({width}x{height})")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    render(args.input, args.output)


if __name__ == "__main__":
    main()
