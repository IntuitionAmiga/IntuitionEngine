#!/usr/bin/env bash
set -euo pipefail

fail(){ echo "check-intuitionengine-release-secrets: $*" >&2; exit 1; }
private_key_marker="BEGIN PGP PRIVATE KEY"
private_key_marker+=" BLOCK"
root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repository_root="$root_dir"
output="$root_dir/intuitionengine.com"

while (($#)); do
    case "$1" in
        --root|--output)
            (($# >= 2)) || fail "missing value for $1"
            if [[ "$1" == --root ]]; then repository_root=$2; else output=$2; fi
            shift 2
            ;;
        *) fail "unknown argument: $1" ;;
    esac
done

[[ -d "$repository_root/.git" ]] || fail "repository root is not a Git checkout: $repository_root"

is_private_name() {
    local path="$1" name
    name="${path##*/}"
    [[ "$name" == private* || "$name" == secret* || "$name" == *.key || "$name" == *.pem || "$name" == secring.gpg ]]
}

check_path_name() {
    local path="$1"
    if is_private_name "$path"; then
        fail "private-key-shaped path is tracked or staged: $path"
    fi
}

while IFS= read -r -d '' path; do
    check_path_name "$path"
done < <(git -C "$repository_root" ls-files -z)

while IFS= read -r -d '' path; do
    check_path_name "$path"
done < <(git -C "$repository_root" diff --cached --name-only -z --diff-filter=ACMR)

if git -C "$repository_root" grep --cached -a -l "$private_key_marker" -- . >/dev/null 2>&1; then
    fail "ASCII-armoured private key material is staged"
fi
if git -C "$repository_root" grep -a -l "$private_key_marker" -- . >/dev/null 2>&1; then
    fail "ASCII-armoured private key material is present in the checkout"
fi

if [[ -d "$output" ]]; then
    while IFS= read -r -d '' path; do
        check_path_name "$path"
    done < <(find "$output" -type f -print0)
    if rg -a -l --hidden --glob '!.git/**' "$private_key_marker" "$output" >/dev/null 2>&1; then
        fail "ASCII-armoured private key material is below release output"
    fi
fi

echo "IntuitionEngine release secret preflight passed"
