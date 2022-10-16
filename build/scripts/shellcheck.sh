#!/bin/sh

[ -n "${DEBUG_SCRIPTS}" ] && set -x

set -euo

cd "${SRC_DIR}"

shellcheck --version
# shellcheck disable=SC2046
shellcheck $(find "${SHELLCHECK_SOURCEPATH}" -type f -name '*.sh')
