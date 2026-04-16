#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

git -C "$ROOT_DIR" config core.hooksPath .githooks
printf 'Git hooks installed: %s\n' "$ROOT_DIR/.githooks"