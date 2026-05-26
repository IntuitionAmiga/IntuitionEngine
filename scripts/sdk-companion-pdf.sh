#!/usr/bin/env bash
# sdk-companion-pdf.sh - print the five SDK companion manuals to PDF.

set -euo pipefail

usage() {
  cat <<'EOF'
usage: scripts/sdk-companion-pdf.sh [options]

Print the five SDK companion Markdown manuals to sdk/docs/*.pdf.

Options:
  --chrome PATH   Chrome/Chromium executable to use
  --mmdc PATH     Mermaid CLI executable to use for ```mermaid fences
  -h, --help      Show this help

Requirements are the same as scripts/refman-pdf.sh.
EOF
}

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chrome_args=()
mmdc_args=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --chrome)
      [[ $# -ge 2 ]] || { echo "sdk-companion-pdf: --chrome requires a path" >&2; exit 2; }
      chrome_args=(--chrome "$2")
      shift 2
      ;;
    --mmdc)
      [[ $# -ge 2 ]] || { echo "sdk-companion-pdf: --mmdc requires a path" >&2; exit 2; }
      mmdc_args=(--mmdc "$2")
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "sdk-companion-pdf: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

docs_dir="$root_dir/sdk/docs"
preface="$root_dir/sdk/docs/refman.publish/00-Preface.md"
manuals=(
  IE64_ISA
  IE32_ISA
  iemon
  iescript
  architecture
)

if [[ ! -s "$preface" ]]; then
  echo "sdk-companion-pdf: missing required PDF preface input: $preface" >&2
  echo "sdk-companion-pdf: run scripts/refman-publish.sh --strict first" >&2
  exit 2
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/intuition-sdk-companion-pdf.XXXXXX")"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

src_dir="$tmp_dir/src"
out_dir="$tmp_dir/pdf"
mkdir -p "$src_dir" "$out_dir"
cp -f "$preface" "$src_dir/00-Preface.md"

for manual in "${manuals[@]}"; do
  md_file="$docs_dir/$manual.md"
  if [[ ! -s "$md_file" ]]; then
    echo "sdk-companion-pdf: missing SDK companion manual: $md_file" >&2
    exit 1
  fi
  cp -f "$md_file" "$src_dir/$manual.md"
done

"$root_dir/scripts/refman-pdf.sh" --src "$src_dir" --out "$out_dir" "${chrome_args[@]}" "${mmdc_args[@]}"

for manual in "${manuals[@]}"; do
  pdf_file="$out_dir/$manual.pdf"
  if [[ ! -s "$pdf_file" ]]; then
    echo "sdk-companion-pdf: expected PDF was not produced: $pdf_file" >&2
    exit 1
  fi
  cp -f "$pdf_file" "$docs_dir/"
done

echo "sdk-companion-pdf: wrote ${#manuals[@]} SDK companion PDF(s) to $docs_dir"
