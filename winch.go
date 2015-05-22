package main

import (
	"net"
	"os"
	"sync"

	"github.com/erikh/termproxy/server"
	"github.com/erikh/termproxy/termproxy"
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

	// send the clear only in the height case, it will resolve itself with width.
	myws, _ := termproxy.GetWinsize(0)

	if myws.Height != height {
		s.Iterate(func(s *server.SSHServer, c net.Conn, index int) error {
			termproxy.WriteClear(c)
			return nil
		})

		termproxy.WriteClear(os.Stdout)
	}

	termproxy.SetWinsize(command.PTY().Fd(), termproxy.Winch{Height: height, Width: width})
	termproxy.SetWinsize(0, termproxy.Winch{Height: height, Width: width})

	s.Iterate(func(s *server.SSHServer, c net.Conn, index int) error {
		payload := []byte{
			0, 0, byte(ws.Width >> 8 & 0xFF), byte(ws.Width & 0xFF),
			0, 0, byte(ws.Height >> 8 & 0xFF), byte(ws.Height & 0xFF),
			0, 0, 0, 0,
			0, 0, 0, 0,
		}

		c.(*server.Conn).SendRequest("window-change", false, payload)
		return nil
	})

	winsizeMutex.Unlock()
}
