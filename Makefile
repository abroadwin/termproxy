test:
	go test -v ./...

dist:
	sh dist.sh

distclean:
	rm termproxy-*.tar.gz

all: test dist
