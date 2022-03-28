#!/bin/bash

set -eu -o pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR/../.."

HELM_CHARTS_PATH="$(pwd)/../helm-charts"
CHARTS_FILE="charts/telegraf-operator/Chart.yaml"

main() {
  local version="${1:?Version required}"
  local branch="${2:-telegraf-operator-v${version//./-}}"

  if [ ! -f "${HELM_CHARTS_PATH}/${CHARTS_FILE}" ] ; then
    echo >&2 "Unable to find ${HELM_CHARTS_PATH}/${CHARTS_FILE}; exiting..."
    exit 1
  fi

  cd "${HELM_CHARTS_PATH}"
  if [ "$(git rev-parse --abbrev-ref HEAD)" != "master" ] ; then
    echo >&2 "helm-charts repo at $(pwd) must be checked out as \"master\""
    exit 1
  fi

  if [ ! -z "$(git status --porcelain)" ]; then
    echo >&2 "helm-charts repo at $(pwd) has local changes"
    exit 1
  fi

  git pull
  git checkout -b "${branch}"

  # use a temporary file for updating version to
  # ensure consistency between Linux and macOS
  trap "rm -f \"${CHARTS_FILE}.tmp\"" INT TERM EXIT
  cat <"${CHARTS_FILE}" >"${CHARTS_FILE}.tmp"
  cat <"${CHARTS_FILE}.tmp" | \
    sed -E "s/^([[:space:]]*version:[[:space:]]*)[0-9].*\$/\1${version}/" | \
    sed -E "s/^([[:space:]]*appVersion:[[:space:]]*v)[0-9].*\$/\1${version}/" \
    >"${CHARTS_FILE}"

  git commit -m "Update telegraf-operator to ${version}" "${CHARTS_FILE}"
}

main "$@"
