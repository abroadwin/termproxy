#!/bin/sh

VERSION=$(cat VERSION)
mypwd=${PWD}

for arch in linux darwin freebsd
do
	mkdir -p /tmp/termproxy-${arch}-${VERSION} && \
	cd /tmp/termproxy-${arch}-${VERSION} && \
	GOOS=$i go build github.com/erikh/termproxy && \
	cd .. && \
	tar cvzf termproxy-${arch}-${VERSION}.tar.gz termproxy-${arch}-${VERSION} && \
  cp termproxy-${arch}-${VERSION}.tar.gz ${mypwd}	
done
