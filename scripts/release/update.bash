#!/bin/bash

set -eu -o pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR/../.."

HELM_CHARTS_PATH="${PWD}/helm-charts"

main() {
  local version="${1:?Version required}"

  ./scripts/release/update-helm-charts.bash "${version}"
  ./scripts/release/update-community-operators.bash "${version}"
}

main "$@"
