#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags novulkan .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags "headless novulkan" .
