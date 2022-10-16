#!/bin/bash

[ -n "${DEBUG_SCRIPTS}" ] && set -x

set -euo pipefail

cd "${SRC_DIR}"

go version

go mod tidy -compat=1.17
