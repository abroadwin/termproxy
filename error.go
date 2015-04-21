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
	windowStateMutex.Lock()

	if err != nil {
		fmt.Println(fmt.Sprintf(msg+": %v", err))
	}

	if err := RestoreTerminal(0, windowState); err != nil {
		Exit(fmt.Sprintf("Could not restore terminal during termination: %v", err), ErrTerminal)
	}
	windowStateMutex.Unlock()

	Exit(msg, exitcode)
}

func Exit(message string, exitcode int) {
	fmt.Println(message)
	os.Exit(exitcode)
}
