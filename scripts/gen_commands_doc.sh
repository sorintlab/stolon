#!/usr/bin/env bash

set -e
# shellcheck disable=SC1091
source "$(dirname "$0")"/readlinkdashf.sh

BASEDIR=$(readlinkdashf "$(dirname "$0")"/..)

go run "$BASEDIR"/scripts/gen_commands_doc.go "$BASEDIR"/doc/commands/
