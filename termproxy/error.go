package termproxy

import (
	"fmt"
	"os"
)

const (
	ErrUsage    uint8 = 1
	ErrTerminal       = 1 << iota
	ErrCommand        = 1 << iota
	ErrTLS            = 1 << iota
	ErrNetwork        = 1 << iota
)

var ErrorOut func(string, error, int) = errorout

func errorout(msg string, err error, exitcode int) {
	if err != nil {
		msg = fmt.Sprintf(msg+": %v", err)
	}

	if windowState != nil {
		windowStateMutex.Lock()

		if err := RestoreTerminal(0, windowState); err != nil {
			Exit(fmt.Sprintf("Could not restore terminal during termination: %v", err), ErrTerminal)
		}

		if err := SetWinsize(0, winsize); err != nil {
			Exit(fmt.Sprintf("Could not restore terminal dimensions during termination: %v", err), ErrTerminal)
		}

		windowStateMutex.Unlock()
	}

	Exit(msg, exitcode)
}

func Exit(message string, exitcode int) {
	WriteClear(os.Stdout)
	fmt.Println(message)
	os.Exit(exitcode)
}
