test:
	go test -v ./...

dist:
	mkdir -p /tmp/termproxy-`cat ${PWD}/VERSION` && \
	cd /tmp/termproxy-`cat ${PWD}/VERSION` && \
	go build github.com/erikh/termproxy/tp && \
	go build github.com/erikh/termproxy/tpc && \
	cd .. && \
	tar cvzf termproxy-`cat ${PWD}/VERSION`.tar.gz termproxy-`cat ${PWD}/VERSION` && \
  cp termproxy-`cat ${PWD}/VERSION`.tar.gz ${PWD}	

all: test dist
