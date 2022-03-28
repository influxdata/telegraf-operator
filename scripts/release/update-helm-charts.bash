#!/bin/bash

set -eu -o pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR/../.."

HELM_CHARTS_PATH="$(pwd)/../helm-charts"
CHARTS_FILE="charts/telegraf-operator/Chart.yaml"

main() {
  local version="${1:?Version required}"
  local branch="${2:-telegraf-operator-v$(echo "${version}" | sed 's#\.#-#g')}"

  if [ ! -f "${HELM_CHARTS_PATH}/${CHARTS_FILE}" ] ; then
    echo >&2 "Unable to find ${HELM_CHARTS_PATH}/${CHARTS_FILE}; exiting..."
    exit 1
  fi

  cd "${HELM_CHARTS_PATH}"
  if [ "$(git rev-parse --abbrev-ref HEAD)" != "master" ] ; then
    echo "helm-charts repo at $(pwd) must be checked out as \"master\""
    exit 1
  fi

  if [ ! -z "$(git status --porcelain)" ]; then
    echo "helm-charts repo at $(pwd) has local changes"
    exit 1
  fi

  git pull
  git checkout -b "${branch}"

  sed -E "s/^([[:space:]]*version:[[:space:]]*)[0-9].*\$/\1${version}/" \
    -i "${CHARTS_FILE}"

  sed -E "s/^([[:space:]]*version:[[:space:]]*)[0-9].*\$/\1${version}/" \
    -i "${CHARTS_FILE}"

  git commit -m "Update telegraf-operator to ${version}" "${CHARTS_FILE}"
}

main "$@"
