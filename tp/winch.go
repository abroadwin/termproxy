package main

import (
	"net"
	"os"
	"sync"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/framing"
	"github.com/erikh/termproxy/server"
)

var (
	connectionWinsizeMap = map[string]*framing.Winch{}
	winsizeMutex         = new(sync.Mutex)
)

func compareAndSetWinsize(host string, ws *framing.Winch, command *termproxy.Command, t *server.TLSServer) {
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
		termproxy.WriteClear(os.Stdout)
		termproxy.SetWinsize(command.PTY().Fd(), winsize)

		t.Iterate(func(t *server.TLSServer, conn net.Conn, index int) error {
			return winsize.WriteTo(conn)
		})
	}

	winsizeMutex.Unlock()
}
