package termproxy

import (
	"net"

	term "github.com/erikh/termproxy/dockerterm"
)

type Winch struct {
	Conn   net.Conn
	Width  uint
	Height uint
}

func (w Winch) ToWinsize() *term.Winsize {
	// the precision loss shouldn't matter
	return &term.Winsize{Height: uint16(w.Height), Width: uint16(w.Width)}
}
