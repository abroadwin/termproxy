package main

import (
	"os"
	"sync"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/framing"
)

var (
	connectionWinsizeMap = map[string]*framing.Winch{}
	winsizeMutex         = new(sync.Mutex)
)

func compareAndSetWinsize(host string, ws *framing.Winch, command *termproxy.Command) {
	winsizeMutex.Lock()
	connectionWinsizeMap[host] = ws

	var height, width uint16

	for _, wm := range connectionWinsizeMap {
		if height == 0 || width == 0 {
			height = wm.Height
			width = wm.Width
		}
		if wm.Height <= height {
			height = wm.Height
		}

		if wm.Width <= width {
			width = wm.Width
		}
	}

	winsize := &framing.Winch{Height: height, Width: width}

	myws, _ := termproxy.GetWinsize(command.PTY().Fd())

	if winsize.Height != myws.Height || winsize.Width != myws.Width {
		// send the clear only in the height case, it will resolve itself with width.

		if winsize.Height != myws.Height {
			termproxy.WriteClear(os.Stdout)
		}

		termproxy.SetWinsize(command.PTY().Fd(), winsize)

		connMutex.Lock()
		for i, c := range connections {
			var prune bool

			if err := winsize.WriteTo(c); err != nil {
				prune = true
			}

			if prune {
				var err error
				connections, err = pruneConnection(connections, i, err)
				if err != nil {
					termproxy.ErrorOut("Error closing connection", err, termproxy.ErrNetwork)
				}
			}
		}
		connMutex.Unlock()
	}

	winsizeMutex.Unlock()
}
