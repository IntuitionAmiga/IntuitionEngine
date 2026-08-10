#!/usr/bin/env python3
"""Create the five-file AB3D2 x64 release archive."""

from __future__ import annotations

import argparse
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("archive", type=Path)
    parser.add_argument("files", nargs="+", help="file names relative to root")
    args = parser.parse_args()

    if len(args.files) != 5:
        parser.error("the AB3D2 archive must contain exactly five files")
    paths = [args.root / name for name in args.files]
    missing = [str(path) for path in paths if not path.is_file()]
    if missing:
        raise SystemExit("missing AB3D2 archive input: " + ", ".join(missing))

    args.archive.parent.mkdir(parents=True, exist_ok=True)
    with ZipFile(
        args.archive,
        "w",
        compression=ZIP_DEFLATED,
        compresslevel=9,
        allowZip64=True,
    ) as archive:
        for path in paths:
            archive.write(path, arcname=path.name)


if __name__ == "__main__":
    main()
