#!/usr/bin/env python3
# Phase 0.5 dead-branch sweep for sdk/ab3d64/src.
#
# - Deletes IFND IS_IE / IFEQ IS_IE,0 ... ENDC regions.
# - Flattens IFD IS_IE / IFNE IS_IE,0 ... ENDC regions (drops the gates,
#   keeps the body). ELSE branches in IS_IE conditionals are inverted.
# - Deletes `include` directives that target Amiga NDK headers
#   (exec/, dos/, graphics/, intuition/, devices/, hardware/,
#   libraries/, resources/).
#
# Walks sdk/ab3d64/src/ in place. Idempotent: a second run is a no-op.
# Operates only on .s / .i / .asm files.
#
# Strip is conservative — only IS_IE-keyed conditionals are resolved.
# Other conditionals (e.g. IFD IE_OVERDRIVE) are preserved verbatim so
# the transpiler's preprocessor handles them via -D defines.

from __future__ import annotations
import argparse
import re
import sys
from pathlib import Path

AMIGA_INCLUDE_PREFIXES = (
    "exec/",
    "dos/",
    "graphics/",
    "intuition/",
    "devices/",
    "hardware/",
    "libraries/",
    "resources/",
)

IF_RE = re.compile(r"^\s*(if|ifd|ifnd|ifeq|ifne)\b\s*(.*?)\s*(?:;.*)?$", re.IGNORECASE)
ELSEIF_RE = re.compile(r"^\s*(elseif|elseifd|elseifnd|elseifeq|elseifne)\b", re.IGNORECASE)
ELSE_RE = re.compile(r"^\s*else\s*(?:;.*)?$", re.IGNORECASE)
ENDC_RE = re.compile(r"^\s*(endc|endif)\s*(?:;.*)?$", re.IGNORECASE)
INCLUDE_RE = re.compile(r"^(\s*)include\s+[\"']?([^\"'\s]+)[\"']?\s*(?:;.*)?$", re.IGNORECASE)


def is_is_ie_cond(directive: str, expr: str) -> tuple[bool, bool] | None:
    """Return (is_is_ie, body_active_when_taken) if directive gates IS_IE.

    body_active_when_taken=True  -> body kept when condition true,
                                    dropped when false.
    Returns None for non-IS_IE conditionals.
    """
    d = directive.lower()
    e = expr.strip().lower()
    if d == "ifd" and e == "is_ie":
        return (True, True)
    if d == "ifnd" and e == "is_ie":
        return (True, False)
    if d == "ifeq":
        # IFEQ IS_IE,0  -> body taken if IS_IE==0  -> drop body
        if e.replace(" ", "") in ("is_ie,0", "is_ie"):
            # IFEQ IS_IE with no rhs is malformed; treat as IS_IE==0.
            return (True, False)
    if d == "ifne":
        if e.replace(" ", "") in ("is_ie,0", "is_ie"):
            return (True, True)
    return None


def find_matching_endc(lines: list[str], start: int) -> int:
    """Return index of the ENDC that closes the IF at `start`."""
    depth = 1
    i = start + 1
    while i < len(lines):
        line = lines[i]
        if IF_RE.match(line) and not ELSEIF_RE.match(line):
            depth += 1
        elif ENDC_RE.match(line):
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise SyntaxError(f"unterminated IF at line {start + 1}")


def find_top_level_else(lines: list[str], start: int, end: int) -> int | None:
    """Return index of the ELSE at depth 1 between start and end, or None."""
    depth = 1
    i = start + 1
    while i < end:
        line = lines[i]
        if IF_RE.match(line) and not ELSEIF_RE.match(line):
            depth += 1
        elif ENDC_RE.match(line):
            depth -= 1
        elif depth == 1 and ELSE_RE.match(line):
            return i
        i += 1
    return None


def strip_file(text: str) -> tuple[str, dict[str, int]]:
    lines = text.splitlines(keepends=False)
    out: list[str] = []
    stats = {"include_dropped": 0, "is_ie_regions_resolved": 0}
    i = 0
    while i < len(lines):
        line = lines[i]
        # Amiga include drop.
        m = INCLUDE_RE.match(line)
        if m and m.group(2).startswith(AMIGA_INCLUDE_PREFIXES):
            stats["include_dropped"] += 1
            i += 1
            continue
        # IS_IE conditional resolution.
        mif = IF_RE.match(line)
        if mif:
            directive, expr = mif.group(1), mif.group(2)
            verdict = is_is_ie_cond(directive, expr)
            if verdict is not None:
                _, active = verdict
                end = find_matching_endc(lines, i)
                else_idx = find_top_level_else(lines, i, end)
                if active:
                    body_start, body_end = i + 1, (else_idx if else_idx is not None else end)
                else:
                    if else_idx is not None:
                        body_start, body_end = else_idx + 1, end
                    else:
                        body_start, body_end = end, end  # empty
                out.extend(lines[body_start:body_end])
                stats["is_ie_regions_resolved"] += 1
                i = end + 1
                continue
        out.append(line)
        i += 1
    # Preserve trailing newline if input had one.
    trailing = "\n" if text.endswith("\n") else ""
    return "\n".join(out) + trailing, stats


def main() -> int:
    ap = argparse.ArgumentParser(description="Strip Amiga surface from AB3D2 snapshot.")
    ap.add_argument("root", type=Path, help="sdk/ab3d64/src root")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    totals = {"files_changed": 0, "include_dropped": 0, "is_ie_regions_resolved": 0}
    targets = []
    for p in sorted(args.root.rglob("*")):
        if not p.is_file():
            continue
        if p.suffix.lower() not in (".s", ".i", ".asm"):
            continue
        targets.append(p)

    for p in targets:
        try:
            text = p.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            text = p.read_bytes().decode("latin-1")
        try:
            new_text, stats = strip_file(text)
        except SyntaxError as e:
            print(f"{p}: {e}", file=sys.stderr)
            continue
        if new_text != text:
            totals["files_changed"] += 1
            totals["include_dropped"] += stats["include_dropped"]
            totals["is_ie_regions_resolved"] += stats["is_ie_regions_resolved"]
            if not args.dry_run:
                p.write_text(new_text, encoding="utf-8")

    print(
        "amiga_strip: files_changed={files_changed} "
        "include_dropped={include_dropped} "
        "is_ie_regions_resolved={is_ie_regions_resolved}".format(**totals)
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
