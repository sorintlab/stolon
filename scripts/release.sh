#!/usr/bin/env bash
#
# Build all release binaries and sources into "./release" directory.
#

set -e

VERSION=$1
if [ -z "${VERSION}" ]; then
	echo "Usage: ${0} VERSION" >> /dev/stderr
	exit 255
fi

BASEDIR=$(readlink -f $(dirname $0))/..
BINDIR=${BASEDIR}/bin

if [ $PWD != $BASEDIR ]; then
	cd $BASEDIR
fi


echo Building stolon binary...
./scripts/build-binary ${VERSION}
