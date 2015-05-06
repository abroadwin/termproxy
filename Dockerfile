FROM golang:1-onbuild

RUN apt-get update && apt-get install vim-nox tmux -y

RUN ssh-keygen -t rsa -b 4096 -f /go/src/app/host_key_rsa -N ''

ENTRYPOINT ["/go/bin/app", "0.0.0.0:1234"]
CMD ["/bin/bash"]
