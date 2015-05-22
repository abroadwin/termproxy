FROM golang:1-onbuild

RUN apt-get update && apt-get install vim-nox tmux -y

WORKDIR /go/bin
RUN ssh-keygen -t rsa -b 4096 -f host_key_rsa -N ''

EXPOSE 1234

ENTRYPOINT ["./app", "0.0.0.0:1234"]
CMD ["/bin/bash"]
