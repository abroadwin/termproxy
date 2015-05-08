#!/bin/sh

VERSION=$(cat VERSION)
mypwd=${PWD}

for os in linux darwin freebsd
do
	mkdir -p /tmp/termproxy-${os}-${VERSION} && \
	cd /tmp/termproxy-${os}-${VERSION} && \
	GOOS=$os GOARCH=amd64 go build github.com/erikh/termproxy && \
	cd .. && \
	tar cvzf termproxy-${os}-${VERSION}.tar.gz termproxy-${os}-${VERSION} && \
  cp termproxy-${os}-${VERSION}.tar.gz ${mypwd}	
done
