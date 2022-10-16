#!/bin/bash

[ -n "${DEBUG_SCRIPTS}" ] && set -x

set -euo pipefail

cd "${SRC_DIR}"
mkdir -p "${BUILD_TARGET_DIR}"

PACKAGE_NAME=$(go mod edit -json | grep 'Path' | head -1 | sed -re 's/.*: "([^"]+)"/\1/')
APP_NAME=$(echo "${PACKAGE_NAME}" | grep -oE '[^/]+$')
LD_FLAGS="-X "${PACKAGE_NAME}/internal/buildinfo".Version=${BUILD_VERSION} \
    -X "${PACKAGE_NAME}/internal/buildinfo".BuildTime=${BUILD_TIME} \
    -X "${PACKAGE_NAME}/internal/buildinfo".AppName=${APP_NAME}"

go version
go mod verify
go build \
    -v \
    -o "${BUILD_TARGET_DIR}/${APP_NAME}" \
    -ldflags="${LD_FLAGS}" \
    "${GO_BUILD_FLAGS}" \
    .
