#!/usr/bin/env python3
"""把品牌矢量渲染成仓库里用到的全部位图（网页、浏览器扩展、手机 App、NAS 应用商店）。

矢量原件由 firlab 仓库的 brand/pt-tools/build.py 生成，逐字节复制到本仓库（见 docs/brand.md）：
web/frontend/public/logo.svg 与 logo-dark.svg，以及只用来出图的 scripts/brand/*.svg。
这里只做栅格化，不改图形；每个尺寸都从矢量直接渲染，不拿大图缩小。

    uv run --with cairosvg==2.7.1 --with pillow==12.0.0 python3 scripts/brand/render.py

只能放一张图的地方一律用墨黑（ink）版，规则：
- 32 px 及以下用 favicon.svg（图形不缩，小尺寸更清楚）；
- 48 px 及以上用 logo.svg（超椭圆底板、四角透明）；
- 要求满铺、不透明的图标（iOS、apple-touch、PWA maskable）用 app-icon.svg，输出不带透明通道的 RGB；
- 安卓自适应图标用三层：前景、背景、单色（108dp 画布，可见区是中间的 72dp）。
"""

from __future__ import annotations

import io
import json
import sys
from pathlib import Path

import cairosvg
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
BRAND = ROOT / "scripts" / "brand"
SOURCES = {
    "logo": ROOT / "web/frontend/public/logo.svg",
    "favicon": BRAND / "favicon.svg",
    "app": BRAND / "app-icon.svg",
    "fg": BRAND / "adaptive-foreground.svg",
    "bg": BRAND / "adaptive-background.svg",
    "mono": BRAND / "adaptive-monochrome.svg",
}

RES = "apps/mobile/android/app/src/main/res"
DENSITIES = {"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4}
IOS = "apps/mobile/ios/Runner/Assets.xcassets/AppIcon.appiconset"


def jobs() -> list[tuple[str, int, str, bool]]:
    """(来源, 边长 px, 输出路径, 是否压成不透明 RGB)"""
    out: list[tuple[str, int, str, bool]] = [
        ("favicon", 16, "web/frontend/public/favicon-16x16.png", False),
        ("favicon", 32, "web/frontend/public/favicon-32x32.png", False),
        ("app", 180, "web/frontend/public/apple-touch-icon.png", True),
        ("logo", 512, "web/frontend/public/logo.png", False),
        ("logo", 192, "web/frontend/public/android-chrome-192x192.png", False),
        ("logo", 512, "web/frontend/public/android-chrome-512x512.png", False),
        ("favicon", 16, "tools/browser-extension/public/icons/icon16.png", False),
        ("favicon", 32, "tools/browser-extension/public/icons/icon32.png", False),
        ("logo", 48, "tools/browser-extension/public/icons/icon48.png", False),
        ("logo", 128, "tools/browser-extension/public/icons/icon128.png", False),
        ("logo", 64, "deploy/fnos/apps/pt-tools/fnos/ICON.PNG", False),
        ("logo", 256, "deploy/fnos/apps/pt-tools/fnos/ICON_256.PNG", False),
        ("logo", 64, "deploy/fnos/apps/pt-tools/fnos/ui/images/64.png", False),
        ("logo", 256, "deploy/fnos/apps/pt-tools/fnos/ui/images/256.png", False),
        ("logo", 512, "deploy/1panel/pt-tools/logo.png", False),
        ("favicon", 32, "apps/mobile/web/favicon.png", False),
        ("logo", 192, "apps/mobile/web/icons/Icon-192.png", False),
        ("logo", 512, "apps/mobile/web/icons/Icon-512.png", False),
        ("app", 192, "apps/mobile/web/icons/Icon-maskable-192.png", True),
        ("app", 512, "apps/mobile/web/icons/Icon-maskable-512.png", True),
    ]
    for name, scale in DENSITIES.items():
        out.append(("logo", round(48 * scale), f"{RES}/mipmap-{name}/ic_launcher.png", False))
        size = round(108 * scale)
        out.append(("fg", size, f"{RES}/mipmap-{name}/ic_launcher_foreground.png", False))
        out.append(("bg", size, f"{RES}/mipmap-{name}/ic_launcher_background.png", True))
        out.append(("mono", size, f"{RES}/mipmap-{name}/ic_launcher_monochrome.png", False))
    contents = json.loads((ROOT / IOS / "Contents.json").read_text(encoding="utf-8"))
    seen: set[str] = set()
    for image in contents["images"]:
        name = image["filename"]
        if name in seen:
            continue
        seen.add(name)
        points = float(image["size"].split("x")[0])
        scale = int(image["scale"].rstrip("x"))
        out.append(("app", round(points * scale), f"{IOS}/{name}", True))
    return out


def render(source: str, size: int) -> Image.Image:
    png = cairosvg.svg2png(url=str(SOURCES[source]), output_width=size, output_height=size)
    return Image.open(io.BytesIO(png)).convert("RGBA")


def save(image: Image.Image, path: Path, opaque: bool) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if opaque:
        if image.getextrema()[3][0] < 255:
            sys.exit(f"{path}: 要求不透明，但来源有透明像素")
        image = image.convert("RGB")
    image.save(path, format="PNG", optimize=True)


def main() -> None:
    for source, path in SOURCES.items():
        if not path.is_file():
            sys.exit(f"缺少矢量原件 {path.relative_to(ROOT)}（{source}）：先从 firlab/brand/pt-tools 复制")
    for source, size, rel, opaque in jobs():
        save(render(source, size), ROOT / rel, opaque)
        print(f"{rel}  {size}px  ← {SOURCES[source].relative_to(ROOT)}")
    # favicon.ico：16、32 用 favicon.svg，48 起用 logo.svg，每一帧都是按尺寸直接渲染的
    frames = [render("favicon" if s <= 32 else "logo", s) for s in (16, 32, 48, 64, 128, 256)]
    ico = ROOT / "web/frontend/public/favicon.ico"
    frames[-1].save(ico, format="ICO", sizes=[f.size for f in frames], append_images=frames[:-1])
    print(f"{ico.relative_to(ROOT)}  16–256px")


if __name__ == "__main__":
    main()
