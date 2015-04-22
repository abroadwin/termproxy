package termproxy

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/kr/pty"
)

type Command struct {
	CloseHandler    func(*os.File, *exec.Cmd)
	PTYSetupHandler func(*os.File, *exec.Cmd)
	WinchHandler    func(*os.File, *exec.Cmd)
	pty             *os.File
	command         *exec.Cmd
	commandString   string
}

func NewCommand(command string) *Command {
	return &Command{commandString: command}
}

func (c *Command) PTY() *os.File {
	return c.pty
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
		c.PTYSetupHandler(c.pty, c.command)
	}

	if c.WinchHandler != nil {
		sigchan := make(chan os.Signal)
		signal.Notify(sigchan, syscall.SIGWINCH)
		go func() {
			for {
				<-sigchan
				c.WinchHandler(c.pty, c.command)
			}
		}()
	}

	c.waitForClose()
	return nil
}

func (c *Command) waitForClose() {
	c.command.Wait()

	if c.CloseHandler != nil {
		c.CloseHandler(c.pty, c.command)
	}

	c.pty.Close()

	ErrorOut("Shell Exited!", nil, 0)
}
