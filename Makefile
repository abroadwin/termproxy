test:
	godep go test -v ./...

dist: distclean
	sh dist.sh

distclean:
	rm -f *.bz2

all: test dist
