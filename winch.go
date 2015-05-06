package main

import (
	"net"
	"os"
	"sync"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/server"
)

var (
	connectionWinsizeMap = map[string]termproxy.Winch{}
	winsizeMutex         = new(sync.Mutex)
)

func compareAndSetWinsize(host string, ws termproxy.Winch, command *termproxy.Command, s *server.SSHServer) {
	winsizeMutex.Lock()
	connectionWinsizeMap[host] = ws

	var height, width uint

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

	myws, _ := termproxy.GetWinsize(command.PTY().Fd())

	if ws.Height != myws.Height || ws.Width != myws.Width {
		// send the clear only in the height case, it will resolve itself with width.
		termproxy.WriteClear(os.Stdout)
		termproxy.SetWinsize(command.PTY().Fd(), ws)

		s.Iterate(func(s *server.SSHServer, c net.Conn, index int) error {
			payload := []byte{
				0, 0, byte(ws.Width >> 8 & 0xFF), byte(ws.Width & 0xFF),
				0, 0, byte(ws.Height >> 8 & 0xFF), byte(ws.Height & 0xFF),
				0, 0, 0, 0,
				0, 0, 0, 0,
			}

			c.(*server.Conn).SendRequest("window-change", false, payload)
			if ws.Height != myws.Height {
				termproxy.WriteClear(c)
			}

			return nil
		})
	}

	winsizeMutex.Unlock()
}
