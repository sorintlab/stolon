#!/usr/bin/env bash

# Test script code thanks to coreos/etcd
#
# Run all tests
# ./test
# ./test -v
#
# Run also integration tests
# INTEGRATION=1 STOLON_TEST_STORE_BACKEND=etcdv3 ./test
#
set -e

# shellcheck disable=SC1091
source "$(dirname "$0")"/scripts/readlinkdashf.sh
BASEDIR=$(readlinkdashf "$(dirname "$0")")
BINDIR=${BASEDIR}/bin

if [ "$PWD" != "$BASEDIR" ]; then
	cd "$BASEDIR"
fi

ORG_PATH="github.com/pgvillage-tools"
REPO_PATH="${ORG_PATH}/stolon"

echo "Running tests..."

COVER=${COVER:-"-cover"}

function go_files_without_license_info() {
	find . -type f -iname '*.go' ! -path './vendor/*' ! -path './tests/*' ! -name '*_test.go' | while read -r file; do
		head -n3 "${file}" | grep -Eq "(Copyright|generated|GENERATED)" || echo -e "  ${file}"
	done
}

echo "Checking for license header..."
IFS=$'\n' read -r -d '' -a licRes < <(go_files_without_license_info) || true
if ((${#licRes[@]})); then
	echo -e "license header checking failed:\n${licRes[*]}"
	exit 255
fi

echo "Running go test"
# test all packages excluding integration tests
IGNORE_PKGS="(vendor/|tests/integration)"

function find_packages() {
	find "." -name \*_test.go | while read -r a; do dirname "$a"; done | sort | uniq | grep -vE "$IGNORE_PKGS" | sed "s|^\.|${REPO_PATH}|g"
}
IFS=$'\n' read -r -d '' -a PACKAGES < <(find_packages) || true

go test -timeout 3m "${COVER}" "$@" "${PACKAGES[@]}" "${RACE[@]}"

if [ -n "$INTEGRATION" ]; then
	echo "Running integration tests..."
	if [ -z "${STOLON_TEST_STORE_BACKEND}" ]; then
		echo "STOLON_TEST_STORE_BACKEND env var needs to be defined (etcd or consul)"
		exit 1
	fi
	export STKEEPER_BIN=${BINDIR}/stolon-keeper
	export STSENTINEL_BIN=${BINDIR}/stolon-sentinel
	export STPROXY_BIN=${BINDIR}/stolon-proxy
	export STCTL_BIN=${BINDIR}/stolonctl
	if [ "${STOLON_TEST_STORE_BACKEND}" == "etcd" ] || [ "${STOLON_TEST_STORE_BACKEND}" == "etcdv2" ] || [ "${STOLON_TEST_STORE_BACKEND}" == "etcdv3" ]; then
		if [ -z "${ETCD_BIN}" ]; then
			if [ -z "$(which etcd)" ]; then
				echo "cannot find etcd in PATH and ETCD_BIN environment variable not defined"
				exit 1
			fi
			ETCD_BIN=$(which etcd)
		fi
		echo "using etcd from $ETCD_BIN"
		export ETCD_BIN
	elif [ "${STOLON_TEST_STORE_BACKEND}" == "consul" ]; then
		if [ -z "${CONSUL_BIN}" ]; then
			if [ -z "$(which consul)" ]; then
				echo "cannot find consul in PATH and CONSUL_BIN environment variable not defined"
				exit 1
			fi
			CONSUL_BIN=$(which consul)
		fi
		echo "using consul from $CONSUL_BIN"
		export CONSUL_BIN
	else
		echo "Unknown store backend: \"${STOLON_TEST_STORE_BACKEND}\""
		exit 1
	fi

	[ -z "$PARALLEL" ] && PARALLEL=4
	go test -timeout 20m "$@" -v -count 1 -parallel "${PARALLEL}" "${REPO_PATH}"/tests/integration
fi

echo "Success"
