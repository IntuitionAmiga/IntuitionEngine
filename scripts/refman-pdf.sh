#!/usr/bin/env bash
# refman-pdf.sh - print each published PRG Markdown file to PDF.
#
# Default input:  sdk/docs/refman.publish/
# Default output: sdk/docs/refman.publish/pdf/

set -euo pipefail

usage() {
  cat <<'EOF'
usage: scripts/refman-pdf.sh [options]

Print every .md file in sdk/docs/refman.publish/ to one PDF per file.

Options:
  --src DIR       Markdown source directory (default: sdk/docs/refman.publish)
  --out DIR       PDF output directory (default: SRC/pdf)
  --chrome PATH   Chrome/Chromium executable to use
  --keep-html     Keep temporary rendered HTML files
  -h, --help      Show this help

Requirements:
  python3 with the "markdown" module
  google-chrome, chromium, or chromium-browser
EOF
}

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src_dir="$root_dir/sdk/docs/refman.publish"
out_dir=""
chrome_path="${CHROME:-}"
keep_html=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --src)
      [[ $# -ge 2 ]] || { echo "refman-pdf: --src requires a directory" >&2; exit 2; }
      src_dir="$2"
      shift 2
      ;;
    --out)
      [[ $# -ge 2 ]] || { echo "refman-pdf: --out requires a directory" >&2; exit 2; }
      out_dir="$2"
      shift 2
      ;;
    --chrome)
      [[ $# -ge 2 ]] || { echo "refman-pdf: --chrome requires a path" >&2; exit 2; }
      chrome_path="$2"
      shift 2
      ;;
    --keep-html)
      keep_html=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "refman-pdf: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$src_dir" ]]; then
  echo "refman-pdf: source directory not found: $src_dir" >&2
  exit 2
fi

if [[ -z "$out_dir" ]]; then
  out_dir="$src_dir/pdf"
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "refman-pdf: python3 not found" >&2
  exit 2
fi

if ! python3 - <<'PY' >/dev/null 2>&1
import markdown
PY
then
  echo 'refman-pdf: Python module "markdown" not found' >&2
  echo 'refman-pdf: install python3-markdown or run: python3 -m pip install markdown' >&2
  exit 2
fi

if [[ -z "$chrome_path" ]]; then
  for candidate in google-chrome chromium chromium-browser; do
    if command -v "$candidate" >/dev/null 2>&1; then
      chrome_path="$(command -v "$candidate")"
      break
    fi
  done
fi

if [[ -z "$chrome_path" || ! -x "$chrome_path" ]]; then
  echo "refman-pdf: Chrome/Chromium executable not found" >&2
  echo "refman-pdf: set CHROME=/path/to/chrome or pass --chrome /path/to/chrome" >&2
  exit 2
fi

mapfile -d '' md_files < <(find "$src_dir" -maxdepth 1 -type f -name '*.md' -print0 | sort -z)
if [[ ${#md_files[@]} -eq 0 ]]; then
  echo "refman-pdf: no .md files found in $src_dir" >&2
  exit 1
fi

if [[ -e "$src_dir/README.md" ]]; then
  echo "refman-pdf: unexpected legacy input found: $src_dir/README.md" >&2
  echo "refman-pdf: run scripts/refman-publish.sh --strict; the preface is now 00-Preface.md" >&2
  exit 2
fi

if [[ ! -e "$src_dir/00-Preface.md" ]]; then
  echo "refman-pdf: missing required preface input: $src_dir/00-Preface.md" >&2
  echo "refman-pdf: run scripts/refman-publish.sh --strict before printing PDFs" >&2
  exit 2
fi

mkdir -p "$out_dir"

declare -A expected_pdfs=()
for md_file in "${md_files[@]}"; do
  base="$(basename "$md_file" .md)"
  expected_pdfs["$out_dir/$base.pdf"]=1
done

while IFS= read -r -d '' pdf_file; do
  if [[ -z "${expected_pdfs[$pdf_file]+x}" ]]; then
    rm -f "$pdf_file"
    printf 'removed stale %s\n' "$pdf_file"
  fi
done < <(find "$out_dir" -maxdepth 1 -type f -name '*.pdf' -print0)

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/intuition-refman-pdf.XXXXXX")"
cleanup() {
  if [[ "$keep_html" -eq 1 ]]; then
    echo "refman-pdf: kept temporary HTML in $tmp_dir"
  else
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

python3 - "$src_dir" "$tmp_dir/html" <<'PY'
from pathlib import Path
import html
import sys

import markdown

src_dir = Path(sys.argv[1])
html_dir = Path(sys.argv[2])
html_dir.mkdir(parents=True, exist_ok=True)

css = r'''
@page { size: Letter; margin: 0.65in; }
:root { color-scheme: light; }
body {
  max-width: 8.5in;
  margin: 0 auto;
  color: #161616;
  background: #fff;
  font-family: "Noto Serif", Georgia, "Times New Roman", serif;
  font-size: 10.5pt;
  line-height: 1.38;
}
h1, h2, h3, h4, h5, h6 {
  font-family: "Noto Sans", Arial, Helvetica, sans-serif;
  line-height: 1.18;
  break-after: avoid;
  page-break-after: avoid;
  margin: 1.25em 0 0.45em;
}
h1 {
  font-size: 22pt;
  border-bottom: 1px solid #777;
  padding-bottom: 0.18in;
  margin-top: 0;
}
h2 {
  font-size: 15pt;
  border-bottom: 1px solid #ddd;
  padding-bottom: 0.06in;
}
h3 { font-size: 12.5pt; }
h4, h5, h6 { font-size: 11pt; }
p, ul, ol, table, pre, blockquote { margin: 0.58em 0; }
a { color: #0645ad; text-decoration: none; }
code, pre {
  font-family: "DejaVu Sans Mono", "Liberation Mono", Consolas, monospace;
  font-size: 8.7pt;
}
pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  border: 1px solid #d0d0d0;
  background: #f7f7f7;
  padding: 0.09in;
  break-inside: avoid;
  page-break-inside: avoid;
}
blockquote {
  border-left: 3px solid #aaa;
  margin-left: 0;
  padding-left: 0.14in;
  color: #444;
}
table {
  border-collapse: collapse;
  width: 100%;
  font-size: 8.9pt;
  break-inside: auto;
}
th, td {
  border: 1px solid #c9c9c9;
  padding: 0.045in 0.065in;
  vertical-align: top;
}
th {
  background: #eee;
  font-family: "Noto Sans", Arial, Helvetica, sans-serif;
}
tr {
  break-inside: avoid;
  page-break-inside: avoid;
}
hr {
  border: 0;
  border-top: 1px solid #aaa;
  margin: 1em 0;
}
img { max-width: 100%; }
'''

extensions = ["extra", "toc", "tables", "fenced_code", "sane_lists"]
for md_file in sorted(src_dir.glob("*.md")):
    text = md_file.read_text(encoding="utf-8")
    body = markdown.markdown(text, extensions=extensions, output_format="html5")
    title = next((line.lstrip("#").strip() for line in text.splitlines() if line.startswith("#")), md_file.stem)
    rendered = f'''<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>{html.escape(title)}</title>
<style>{css}</style>
</head>
<body>
{body}
</body>
</html>
'''
    (html_dir / f"{md_file.stem}.html").write_text(rendered, encoding="utf-8")
PY

chrome_flags=(
  --headless
  --disable-gpu
  --no-sandbox
  --disable-dev-shm-usage
  --disable-background-networking
  --disable-extensions
  --disable-sync
  --disable-crash-reporter
  --disable-crashpad
  "--user-data-dir=$tmp_dir/chrome-profile"
  --no-pdf-header-footer
)

file_uri() {
  python3 - "$1" <<'PY'
from pathlib import Path
import sys

print(Path(sys.argv[1]).resolve().as_uri())
PY
}

failures=()
generated=0
for md_file in "${md_files[@]}"; do
  base="$(basename "$md_file" .md)"
  html_file="$tmp_dir/html/$base.html"
  pdf_file="$out_dir/$base.pdf"
  uri="$(file_uri "$html_file")"

  if "$chrome_path" "${chrome_flags[@]}" "--print-to-pdf=$pdf_file" "$uri" >/dev/null 2>"$tmp_dir/$base.chrome.err"; then
    if [[ -s "$pdf_file" ]]; then
      generated=$((generated + 1))
      printf 'wrote %s\n' "$pdf_file"
    else
      failures+=("$md_file: Chrome exited successfully but PDF is empty")
    fi
  else
    failures+=("$md_file: $(tail -n 1 "$tmp_dir/$base.chrome.err")")
  fi
done

if [[ ${#failures[@]} -gt 0 ]]; then
  echo "refman-pdf: FAIL - ${#failures[@]} file(s) failed" >&2
  printf '  %s\n' "${failures[@]}" >&2
  exit 1
fi

echo "refman-pdf: wrote $generated PDF(s) to $out_dir"
