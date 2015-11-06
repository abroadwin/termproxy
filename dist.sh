#!/bin/sh

set -e

VERSION=$(cat VERSION)
mypwd=${PWD}

for os in linux darwin freebsd
do
  echo "Building ${os}"

  export GOOS=$os 
  export GOARCH=amd64 
  export GOBIN=$GOPATH/bin
  bin="termproxy-${os}-${VERSION}"

	godep go build -v -o "${bin}" .
  bzip2 "${bin}"
done
