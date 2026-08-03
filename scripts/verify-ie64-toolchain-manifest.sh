#!/usr/bin/env bash
set -euo pipefail

root=$1
manifest="${root}/share/ie64/build-manifest.txt"
[[ -f "${manifest}" ]] || { echo 'missing build manifest' >&2; exit 1; }

declare -A declared
while IFS= read -r line; do
    [[ "${line}" == file=* ]] || continue
    record=${line#file=}
    digest=${record%%  *}
    path=${record#*  }
    [[ -n "${digest}" && -n "${path}" && "${path}" != "${record}" ]] || {
        echo "invalid manifest file record: ${line}" >&2
        exit 1
    }
    [[ -z "${declared[${path}]:-}" ]] || { echo "duplicate path: ${path}" >&2; exit 1; }
    declared["${path}"]=${digest}
done <"${manifest}"

while IFS= read -r file; do
    path=${file#"${root}/"}
    [[ "${path}" == share/ie64/build-manifest.txt ]] && continue
    expected=${declared[${path}]:-}
    [[ -n "${expected}" ]] || { echo "undeclared payload: ${path}" >&2; exit 1; }
    actual=$(sha256sum "${file}" | cut -d' ' -f1)
    [[ "${actual}" == "${expected}" ]] || { echo "digest mismatch: ${path}" >&2; exit 1; }
    unset 'declared[$path]'
done < <(find "${root}" -type f -print | LC_ALL=C sort)

[[ ${#declared[@]} -eq 0 ]] || {
    echo "manifest names missing payload files" >&2
    exit 1
}
