package termproxy

import (
	"fmt"
	"io"
	"sync"

	term "github.com/erikh/termproxy/dockerterm"
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
	MakeRaw         func(uintptr)                    = makeraw
	GetWinsize      func(uintptr) (Winch, error)     = getwinsize
	SetWinsize      func(uintptr, Winch) error       = setwinsize
	WriteClear      func(io.Writer) error            = writeclear
	RestoreTerminal func(uintptr, *term.State) error = restoreterminal
)

func restoreterminal(fd uintptr, windowState *term.State) error {
	return term.RestoreTerminal(fd, windowState)
}

func writeclear(out io.Writer) error {
	_, err := out.Write([]byte{27, 'c'})
	return err
}

func makeraw(fd uintptr) {
	windowState, err := term.MakeRaw(fd)
	if err != nil {
		Exit(fmt.Sprintf("Could not create a raw terminal: %v", err), ErrTerminal)
	}

	setWindowState(windowState)
}

func getwinsize(fd uintptr) (Winch, error) {
	ws, err := term.GetWinsize(fd)
	if err != nil {
		return Winch{}, err
	}

	return Winch{Height: uint(ws.Height), Width: uint(ws.Width)}, nil
}

func setwinsize(fd uintptr, winch Winch) error {
	return term.SetWinsize(fd, winch.ToWinsize())
}
