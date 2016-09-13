#!/usr/bin/env bash
#
# Update vendored dedendencies. Requires glide >= 0.12
#
set -e

if ! [[ "$PWD" = "$GOPATH/src/github.com/sorintlab/stolon" ]]; then
  echo "must be run from \$GOPATH/src/github.com/sorintlab/stolon"
  exit 255
fi

if [ ! $(command -v glide) ]; then
        echo "glide: command not found"
        exit 255
fi

if [ ! $(command -v glide-vc) ]; then
        echo "glide-vc: command not found"
        exit 255
fi

glide update --strip-vendor
glide-vc --only-code --no-tests

