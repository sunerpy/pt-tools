#!/usr/bin/env python3
"""校验 pt-tools 工具链版本来源的一致性。

设计依据：CI/Release 迁移设计 v7 §4.5。共 12 类来源：

  10 类「精确 pin」——要求解析并规范化后的 SemVer 相等；
   2 类「兼容区间」——要求 pin 落在区间内（用区间求值，不做字符串比较）。

镜像的 repository 与 flavor 单独校验，不被版本规范化吞掉：错误的
repository（例如 golang -> golang-alpine 之外的仓库）或错误的 flavor
（例如 node:25.2.0-slim）必须失败，而不是被当作等价版本放过。

规范来源（canonical）：
  Go   -> go.mod
  Node -> .node-version
  pnpm -> Dockerfile 的 `npm install -g pnpm@X`

其余来源与规范来源比较。任一不一致即以非零退出并逐条打印诊断。
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

FAILURES: list[str] = []
CHECKED = 0


def fail(source: str, detail: str) -> None:
    FAILURES.append(f"{source}: {detail}")


def read(rel: str) -> str:
    path = ROOT / rel
    if not path.is_file():
        fail(rel, "文件不存在")
        return ""
    return path.read_text(encoding="utf-8")


def find1(text: str, pattern: str, source: str) -> str | None:
    """提取唯一捕获组；缺失或多于一处都视为失败（避免漏改或歧义）。"""
    hits = re.findall(pattern, text, re.MULTILINE)
    if not hits:
        fail(source, f"未匹配到版本来源，正则 {pattern!r}")
        return None
    if len(set(hits)) > 1:
        fail(source, f"同一来源出现多个不同版本 {sorted(set(hits))}")
        return None
    return hits[0]


def semver(raw: str, source: str) -> tuple[int, int, int] | None:
    """规范化为 (major, minor, patch)。拒绝非三段或含非数字的输入。"""
    m = re.fullmatch(r"v?(\d+)\.(\d+)\.(\d+)", raw.strip())
    if not m:
        fail(source, f"不是规范的三段 SemVer: {raw!r}")
        return None
    return (int(m.group(1)), int(m.group(2)), int(m.group(3)))


def parse_image(raw: str, source: str, want_repo: str, want_flavor: str | None):
    """拆解 repo:version[-flavor]，分别校验 repository、flavor 与版本。"""
    if ":" not in raw:
        fail(source, f"镜像缺少 tag: {raw!r}")
        return None
    repo, tag = raw.rsplit(":", 1)
    if repo != want_repo:
        fail(source, f"镜像 repository 应为 {want_repo!r}，实际 {repo!r}")
        return None
    if want_flavor is None:
        if "-" in tag:
            fail(source, f"镜像 tag 不应带 flavor 后缀，实际 {tag!r}")
            return None
        version = tag
    else:
        suffix = f"-{want_flavor}"
        if not tag.endswith(suffix):
            fail(source, f"镜像 flavor 应为 {want_flavor!r}，实际 tag {tag!r}")
            return None
        version = tag[: -len(suffix)]
    return semver(version, source)


def expect_equal(source: str, got, canonical, canon_name: str) -> None:
    global CHECKED
    CHECKED += 1
    if got is None or canonical is None:
        return
    if got != canonical:
        got_s = ".".join(map(str, got))
        want_s = ".".join(map(str, canonical))
        fail(source, f"版本 {got_s} 与规范来源 {canon_name} 的 {want_s} 不一致")


def satisfies_min(pin, spec: str, source: str) -> None:
    """兼容区间求值：当前只需支持 '>=X[.Y[.Z]]' 形式。"""
    global CHECKED
    CHECKED += 1
    if pin is None:
        return
    m = re.fullmatch(r">=\s*v?(\d+)(?:\.(\d+))?(?:\.(\d+))?", spec.strip())
    if not m:
        fail(source, f"无法解析兼容区间 {spec!r}（当前仅支持 '>=X[.Y[.Z]]'）")
        return
    lower = (
        int(m.group(1)),
        int(m.group(2) or 0),
        int(m.group(3) or 0),
    )
    if pin < lower:
        pin_s = ".".join(map(str, pin))
        low_s = ".".join(map(str, lower))
        fail(source, f"pin {pin_s} 不满足区间 {spec.strip()}（下界 {low_s}）")


def main() -> int:
    go_mod = read("go.mod")
    dockerfile = read("Dockerfile")
    makefile = read("Makefile")
    node_version_file = read(".node-version")

    # ---- 规范来源 ----
    go_canon = semver(find1(go_mod, r"^go (\d+\.\d+\.\d+)$", "go.mod") or "", "go.mod")
    node_canon = semver(node_version_file, ".node-version")
    pnpm_canon = semver(
        find1(dockerfile, r"pnpm@(\d+\.\d+\.\d+)", "Dockerfile pnpm") or "",
        "Dockerfile pnpm",
    )

    # ---- 1..3 Go 版本三处 ----
    expect_equal("go.mod", go_canon, go_canon, "go.mod")  # 自身，占位计数
    expect_equal(
        "Dockerfile ARG BUILD_IMAGE",
        parse_image(
            find1(dockerfile, r"^ARG BUILD_IMAGE=(\S+)$", "Dockerfile BUILD_IMAGE") or "",
            "Dockerfile ARG BUILD_IMAGE",
            want_repo="golang",
            want_flavor=None,
        ),
        go_canon,
        "go.mod",
    )
    expect_equal(
        "Makefile BUILD_IMAGE",
        parse_image(
            find1(makefile, r"^BUILD_IMAGE \?= (\S+)$", "Makefile BUILD_IMAGE") or "",
            "Makefile BUILD_IMAGE",
            want_repo="golang",
            want_flavor=None,
        ),
        go_canon,
        "go.mod",
    )

    # ---- 4..6 Node 版本三处 ----
    expect_equal(".node-version", node_canon, node_canon, ".node-version")
    expect_equal(
        "Dockerfile ARG NODE_IMAGE",
        parse_image(
            find1(dockerfile, r"^ARG NODE_IMAGE=(\S+)$", "Dockerfile NODE_IMAGE") or "",
            "Dockerfile ARG NODE_IMAGE",
            want_repo="node",
            want_flavor="alpine",
        ),
        node_canon,
        ".node-version",
    )
    expect_equal(
        "Makefile NODE_IMAGE",
        parse_image(
            find1(makefile, r"^NODE_IMAGE \?= (\S+)$", "Makefile NODE_IMAGE") or "",
            "Makefile NODE_IMAGE",
            want_repo="node",
            want_flavor="alpine",
        ),
        node_canon,
        ".node-version",
    )

    # ---- 7..9 pnpm 三处（Dockerfile 为规范来源，另两处 packageManager） ----
    expect_equal("Dockerfile pnpm", pnpm_canon, pnpm_canon, "Dockerfile pnpm")
    for rel in ("web/frontend/package.json", "tools/browser-extension/package.json"):
        raw = read(rel)
        if not raw:
            continue
        try:
            pkg = json.loads(raw)
        except json.JSONDecodeError as exc:
            fail(rel, f"JSON 解析失败: {exc}")
            continue
        pm = pkg.get("packageManager")
        if not isinstance(pm, str) or not pm.startswith("pnpm@"):
            fail(rel, f"packageManager 应为 'pnpm@<version>'，实际 {pm!r}")
            continue
        expect_equal(
            f"{rel} packageManager",
            semver(pm[len("pnpm@") :], f"{rel} packageManager"),
            pnpm_canon,
            "Dockerfile pnpm",
        )

    # ---- 10 各 workflow 的 pnpm/action-setup version ----
    wf_dir = ROOT / ".github" / "workflows"
    seen_workflow_pin = False
    for wf in sorted(wf_dir.glob("*.yml")):
        text = wf.read_text(encoding="utf-8")
        # 仅在紧随 pnpm/action-setup 的 with 块内取 version
        for m in re.finditer(
            r"uses:\s*pnpm/action-setup@[^\n]*\n(?:\s+[^\n]*\n){0,4}?\s+version:\s*(\S+)",
            text,
        ):
            seen_workflow_pin = True
            source = f".github/workflows/{wf.name} pnpm/action-setup"
            expect_equal(
                source,
                semver(m.group(1).strip('"\''), source),
                pnpm_canon,
                "Dockerfile pnpm",
            )
    if not seen_workflow_pin:
        fail(".github/workflows", "未找到任何 pnpm/action-setup 的 version 固定值")

    # ---- 11..12 兼容区间 ----
    fe_raw = read("web/frontend/package.json")
    if fe_raw:
        try:
            fe = json.loads(fe_raw)
        except json.JSONDecodeError as exc:
            fail("web/frontend/package.json", f"JSON 解析失败: {exc}")
        else:
            engines = fe.get("engines") or {}
            if "pnpm" in engines:
                satisfies_min(
                    pnpm_canon, engines["pnpm"], "web/frontend engines.pnpm"
                )
            else:
                fail("web/frontend/package.json", "缺少 engines.pnpm")
            if "node" in engines:
                satisfies_min(
                    node_canon, engines["node"], "web/frontend engines.node"
                )
            else:
                fail("web/frontend/package.json", "缺少 engines.node")

    # ---- 结果 ----
    if FAILURES:
        print(f"toolchain-check 失败（{len(FAILURES)} 项，共校验 {CHECKED} 处）：")
        for line in FAILURES:
            print(f"  - {line}")
        return 1
    print(f"toolchain-check 通过（共校验 {CHECKED} 处来源）")
    print(
        "  Go   = {}\n  Node = {}\n  pnpm = {}".format(
            ".".join(map(str, go_canon)) if go_canon else "?",
            ".".join(map(str, node_canon)) if node_canon else "?",
            ".".join(map(str, pnpm_canon)) if pnpm_canon else "?",
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
