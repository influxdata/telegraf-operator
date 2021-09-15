#!/bin/sh

export CGO_ENABLED=0
export GO111MODULE=on
export GOOS=linux

# default to building amd64 binary, same as prior to multi-arch support
TARGETPLATFORM="${1:-linux/amd64}"

# default to building same platform as the target one
BUILDPLATFORM="${2:-}"

if [ "${TARGETPLATFORM}" = "linux/amd64" ] ; then
  export GOARCH=amd64
elif [ "${TARGETPLATFORM}" = "linux/arm64" ] ; then
  export GOARCH=arm64
elif [ "${TARGETPLATFORM}" = "linux/arm/v7" ] ; then
  export GOARCH=arm
else
  echo >&2 "Unknown TARGETPLATFORM: \"${TARGETPLATFORM}\""
  exit 1
fi

echo "Compiling telegraf-operator binary for $GOOS / $GOARCH (from $TARGETPLATFORM)"
if [ "$BUILDPLATFORM" != "" ] ; then
  echo "  (using $BUILDPLATFORM for compilation)"
fi

go build -a -o manager *.go
