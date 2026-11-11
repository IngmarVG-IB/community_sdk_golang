#!/usr/bin/env bash
# Format Go code with gofumpt (pinned as a go.mod tool dependency).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
go tool gofumpt -l -w .
