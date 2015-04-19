package termproxy

import (
	"fmt"
	"io"
	"sync"

	"github.com/docker/docker/pkg/term"
	"github.com/erikh/termproxy/framing"
)

var (
	windowStateMutex sync.Mutex
	windowState      *term.State
)

func setWindowState(state *term.State) {
	windowStateMutex.Lock()
	windowState = state
	windowStateMutex.Unlock()
}

var (
	MakeRaw         func(uintptr)                         = makeraw
	GetWinsize      func(uintptr) (*framing.Winch, error) = getwinsize
	SetWinsize      func(uintptr, *framing.Winch) error   = setwinsize
	WriteClear      func(io.Writer) error                 = writeclear
	RestoreTerminal func(uintptr, *term.State) error      = restoreterminal
)

func restoreterminal(fd uintptr, windowState *term.State) error {
	return term.RestoreTerminal(fd, windowState)
}

func writeclear(out io.Writer) error {
	// Using all those BBSes in high school really mattered.
	_, err := out.Write([]byte{27, '[', '2', 'J'})
	return err
}

func makeraw(fd uintptr) {
	windowState, err := term.MakeRaw(fd)
	if err != nil {
		Exit(fmt.Sprintf("Could not create a raw terminal: %v", err), ErrTerminal)
	}

	setWindowState(windowState)
}

func getwinsize(fd uintptr) (*framing.Winch, error) {
	ws, err := term.GetWinsize(fd)
	if err != nil {
		return nil, err
	}

	return &framing.Winch{Height: ws.Height, Width: ws.Width}, nil
}

func setwinsize(fd uintptr, winch *framing.Winch) error {
	ws := &term.Winsize{Height: winch.Height, Width: winch.Width}

	return term.SetWinsize(fd, ws)
}
