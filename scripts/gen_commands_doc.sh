#!/usr/bin/env bash

set -e
source readlinkdashf.sh

BASEDIR=$(readlinkdashf $(dirname $0)/..)

go run $BASEDIR/scripts/gen_commands_doc.go $BASEDIR/doc/commands/
