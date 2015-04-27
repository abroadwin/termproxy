package main

import "github.com/erikh/termproxy"

func closeHandler(command *termproxy.Command) {
	connMutex.Lock()
	// FIXME sloppy as heck but works for now.
	for _, conn := range connections {
		conn.Close()
	}
	connMutex.Unlock()

}

func setPTYTerminal(command *termproxy.Command) {
	ws, err := termproxy.GetWinsize(0)
	if err != nil {
		termproxy.ErrorOut("Could not retrieve the terminal dimensions", err, termproxy.ErrTerminal)
	}

	compareAndSetWinsize("localhost", ws, command)

	if err := termproxy.SetWinsize(command.PTY().Fd(), ws); err != nil {
		termproxy.ErrorOut("Could not set the terminal size of the PTY", err, termproxy.ErrTerminal)
	}
}

func handleWinch(command *termproxy.Command) {
	ws, err := termproxy.GetWinsize(0)
	if err != nil {
		termproxy.ErrorOut("Could not retrieve the terminal size: %v", err, termproxy.ErrTerminal)
	}

	compareAndSetWinsize("localhost", ws, command)
}
