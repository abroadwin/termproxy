package termproxy

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/kr/pty"
)

type Command struct {
	CloseHandler    func(*Command)
	PTYSetupHandler func(*Command)
	WinchHandler    func(*Command)
	pty             *os.File
	command         *exec.Cmd
	commandString   string
}

func NewCommand(command string) *Command {
	return &Command{commandString: command}
}

func (c *Command) String() string {
	return c.commandString
}

func (c *Command) PTY() *os.File {
	return c.pty
}

func (c *Command) Quit() error {
	return c.command.Process.Signal(syscall.SIGTERM)
}

func (c *Command) Command() *exec.Cmd {
	return c.command
}

func (c *Command) Run() error {
	var err error

	cmd := exec.Command("/bin/sh", "-c", c.commandString)
	c.command = cmd
	c.pty, err = pty.Start(c.command)
	if err != nil {
		return err
	}

	if c.PTYSetupHandler != nil {
		c.PTYSetupHandler(c)
	}

	if c.WinchHandler != nil {
		sigchan := make(chan os.Signal)
		signal.Notify(sigchan, syscall.SIGWINCH)
		// FIXME leaky goroutine
		go func() {
			for {
				<-sigchan
				c.WinchHandler(c)
			}
		}()
	}

	c.waitForClose()
	return nil
}

func (c *Command) waitForClose() {
	c.command.Wait()

	if c.CloseHandler != nil {
		c.CloseHandler(c)
	}

	c.pty.Close()

	//ErrorOut("Shell Exited!", nil, 0)
}
