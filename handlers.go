package main

import (
	"net"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/server"
)

func closeHandler(s *server.SSHServer) func(*termproxy.Command) {
	return func(command *termproxy.Command) {
		s.Iterate(func(s *server.SSHServer, conn net.Conn, i int) error {
			conn.Close()
			return nil
		})
	}
}

func setPTYTerminal(s *server.SSHServer) func(*termproxy.Command) {
	return func(command *termproxy.Command) {
		ws, err := termproxy.GetWinsize(0)
		if err != nil {
			termproxy.ErrorOut("Could not retrieve the terminal dimensions", err, termproxy.ErrTerminal)
		}

		compareAndSetWinsize("localhost", ws, command, s)

		if err := termproxy.SetWinsize(command.PTY().Fd(), ws); err != nil {
			termproxy.ErrorOut("Could not set the terminal size of the PTY", err, termproxy.ErrTerminal)
		}
	}
}

func handleWinch(s *server.SSHServer) func(*termproxy.Command) {
	return func(command *termproxy.Command) {
		ws, err := termproxy.GetWinsize(0)
		if err != nil {
			termproxy.ErrorOut("Could not retrieve the terminal size: %v", err, termproxy.ErrTerminal)
		}

		compareAndSetWinsize("localhost", ws, command, s)
	}
}
