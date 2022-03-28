#!/bin/bash

set -eu -o pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR/../.."

COMMUNITY_OPERATORS_PATH="$(pwd)/../community-operators"
OPERATOR_PATH="operators/telegraf-operator"

main() {
  local version="${1:?Version required}"
  local branch="${2:-telegraf-operator-v$(echo "${version}" | sed 's#\.#-#g')}"
  local dir="${OPERATOR_PATH}/${version}"
  local hygendir="$(pwd)"

  if [ ! -d "${COMMUNITY_OPERATORS_PATH}/${OPERATOR_PATH}" ] ; then
    echo >&2 "Unable to find ${COMMUNITY_OPERATORS_PATH}/${OPERATOR_PATH}; exiting..."
    exit 1
  fi

  cd "${COMMUNITY_OPERATORS_PATH}"
  if [ "$(git rev-parse --abbrev-ref HEAD)" != "main" ] ; then
    echo "community-operators repo at $(pwd) must be checked out as \"main\""
    exit 1
  fi

  if [ ! -z "$(git status --porcelain)" ]; then
    echo "community-operators repo at $(pwd) has local changes"
    exit 1
  fi

  git pull
  git checkout -b "${branch}"

  cd "${hygendir}"
  hygen community-operator release --output "${COMMUNITY_OPERATORS_PATH}/${OPERATOR_PATH}" --version "${version}" --createdAt "$(date +%Y-%m-%d)"

  cd "${COMMUNITY_OPERATORS_PATH}"
  git add "${OPERATOR_PATH}/${version}"
  git commit -m "operators telegraf-operator (${version})"
}

main "$@"
