# termproxy: share a program with others (for pairing!)

**termproxy is currently alpha quality**

termproxy is a shared program tool. It allows you to start the program of your
choice (a shell, vim/emacs, etc) and allows others to connect and interact with
it with SSH. The intended use case is pairing.

## Installation

```bash
$ go get github.com/erikh/termproxy
```

You will have to supply a host private key to the program so that SSH clients
can connect to it. Do this by supplying the `-k` option. Generating this is
just like generating any ssh key:

```bash
$ ssh-keygen -t rsa -b 4096 -f host_key_rsa -N ''
```

## Usage

Server (presumes default settings):
```
termproxy <host:port> <program>
```

Client:
```
ssh -p <port> scott@host
# password is 'tiger'
```

There are also options to change the default username `-u` and password `-p`,
the default of which is `scott/tiger`.

## Author

Erik Hollensbe <erik@hollensbe.org>

This code also uses a vendored copy of `docker/docker`'s `pkg/term` to provide
the termios support. It is located in the `dockerterm` directory. This was done
to ensure stability of the package.
