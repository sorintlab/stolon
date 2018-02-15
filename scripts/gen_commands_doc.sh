#!/usr/bin/env bash

set -e

BASEDIR=$(readlink -f $(dirname $0)/..)

go run $BASEDIR/scripts/gen_commands_doc.go $BASEDIR/doc/commands/
