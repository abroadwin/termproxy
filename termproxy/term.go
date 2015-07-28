package termproxy

import (
	"fmt"
	"io"
	"sync"
	"time"

	term "github.com/erikh/termproxy/dockerterm"
)

var (
	windowStateMutex sync.Mutex
	windowState      *term.State
	winsize          Winch
)

func setWindowState(state *term.State, size Winch) {
	windowStateMutex.Lock()
	winsize = size
	windowState = state
	windowStateMutex.Unlock()
}

var (
	MakeRaw         func(uintptr)                    = makeraw
	GetWinsize      func(uintptr) (Winch, error)     = getwinsize
	SetWinsize      func(uintptr, Winch) error       = setwinsize
	WriteClear      func(io.Writer) error            = writeclear
	RestoreTerminal func(uintptr, *term.State) error = restoreterminal
	WriteTop        func(io.Writer, string) error    = writetop
)

func restoreterminal(fd uintptr, windowState *term.State) error {
	return term.RestoreTerminal(fd, windowState)
}

func writeclear(out io.Writer) error {
	_, err := out.Write([]byte{27, 'c'})
	return err
}

func makeraw(fd uintptr) {
	winsize, err := GetWinsize(fd)
	if err != nil {
		Exit(fmt.Sprintf("Could not retrieve terminal information: %v", err), ErrTerminal)
	}

	windowState, err := term.MakeRaw(fd)
	if err != nil {
		Exit(fmt.Sprintf("Could not create a raw terminal: %v", err), ErrTerminal)
	}

	setWindowState(windowState, winsize)
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

func writetop(w io.Writer, str string) error {
	var err error

	_, err = w.Write([]byte{27, '7'})
	if err != nil {
		return err
	}

	_, err = w.Write([]byte{27, '[', '7', 'm', 27, '[', '1', ';', '1', 'H', 27, '[', '2', 'K'})
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(str))
	if err != nil {
		return err
	}

	_, err = w.Write([]byte{27, '[', '0', 'm', 27, '8'})
	if err != nil {
		return err
	}

	go func() {
		time.Sleep(1 * time.Second)
		w.Write([]byte{27, '[', '7', 'm', 27, '[', '1', ';', '1', 'H', 27, '[', '2', 'K'})
		w.Write([]byte{27, '[', '0', 'm', 27, '8'})
	}()

	return nil
}
