# termproxy: share a program with others (for pairing!)

**termproxy is currently alpha quality**

termproxy is a shared program tool. It allows you to start the program of your
choice (a shell, vim/emacs, etc) and allows others to connect and interact with
it via SSH. The intended use case is pairing.

## Features

* Share a terminal with your friends or collagues over SSH.
  * start any program -- when it exits, it will terminate the SSH server too.
  * Terminals are resized to fit everyone's terminal on a new connection.
* Notifications on connection (set `-n=false` to disable).
* Read-only mode for connectors: `-r`
  * present a terminal to others instead of sharing it with them.

## Try quickly with Docker

```bash
$ docker run -p 1234:1234 -it erikh/termproxy
```

(in another window)

```bash
$ ssh -p 1234 scott@localhost
```

Note that the standard SSH connection termination sequence is `~.`. You can
enter this on the SSH side to disconnect from termproxy without stopping the
shared program.

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

Will work for termproxy as long as it is in the directory termproxy is launched
in.

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
