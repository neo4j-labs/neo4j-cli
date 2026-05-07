#!/bin/bash
#
# Normalise a Git tag-shaped version string into its PEP 440 equivalent.
#
# Usage: version-to-pep440.sh <version>
#
# Accepted input shapes (with or without a leading `v`):
#   X.Y.Z              -> X.Y.Z
#   X.Y.Z-alpha.N      -> X.Y.ZaN
#   X.Y.Z-beta.N       -> X.Y.ZbN
#   X.Y.Z-rc.N         -> X.Y.ZrcN
#
# Anything else (e.g. `v0.1.0-pre.6`, `v0.1.0.alpha.6`, empty, `dev`) exits
# non-zero with an error to stderr that names the offending input.
#
# Zero non-bash dependencies (no python, no jq) so it runs on any
# GitHub-hosted ubuntu runner without extra setup.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "ERROR: version-to-pep440.sh requires exactly one argument, got $#" >&2
  echo "Usage: version-to-pep440.sh <version>" >&2
  exit 1
fi

raw="$1"
v="${raw#v}"

if [[ "$v" =~ ^([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "${BASH_REMATCH[1]}"
elif [[ "$v" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-(alpha|beta|rc)\.([0-9]+)$ ]]; then
  base="${BASH_REMATCH[1]}"
  pre="${BASH_REMATCH[2]}"
  n="${BASH_REMATCH[3]}"
  case "$pre" in
    alpha) echo "${base}a${n}" ;;
    beta)  echo "${base}b${n}" ;;
    rc)    echo "${base}rc${n}" ;;
  esac
else
  echo "ERROR: version '$raw' does not match PEP 440 contract (expected vX.Y.Z or vX.Y.Z-(alpha|beta|rc).N)" >&2
  exit 1
fi
