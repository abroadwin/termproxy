test:
	go test -v ./...

cert:
	sh generate_cert.sh

dist:
	sh dist.sh

distclean:
	rm termproxy-*.tar.gz

all: test dist
