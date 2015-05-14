package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/erikh/termproxy/server"
	"github.com/erikh/termproxy/termproxy"
	"github.com/jawher/mow.cli"
)

const TIME_WAIT = 10 * time.Millisecond

var (
	usernameFlag, passwordFlag, hostkeyFlag *string
	readOnly                                *bool
)

func main() {
	tp := cli.App("termproxy", "Proxy your terminal over SSH to others")

	usernameFlag = tp.StringOpt("u username", "scott", "Username for SSH")
	passwordFlag = tp.StringOpt("p password", "tiger", "Password for SSH")
	hostkeyFlag = tp.StringOpt("k host-key", "host_key_rsa", "SSH private host key to present to clients")
	readOnly = tp.BoolOpt("r read-only", false, "Disallow remote clients from entering input")

	listenSpec := tp.StringArg("LISTEN", "0.0.0.0:1234", "The host:port to listen for SSH")
	command := tp.StringArg("COMMAND", "/bin/sh", "The program to run inside termproxy")

	tp.Action = func() {
		serve(*listenSpec, *command)
	}

	tp.Run(os.Args)
}

func setCommand(cmd string, s *server.SSHServer) *termproxy.Command {
	command := termproxy.NewCommand(cmd)
	command.CloseHandler = closeHandler(s)
	command.PTYSetupHandler = setPTYTerminal(s)
	command.WinchHandler = handleWinch(s)

	return command
}

func launch(command *termproxy.Command) {
	if err := command.Run(); err != nil {
		if err != nil {
			termproxy.ErrorOut(fmt.Sprintf("Could not start program %s", command.String()), err, termproxy.ErrCommand)
		}
	}

	termproxy.ErrorOut("Shell Exited!", nil, 0)
}

func serve(listenSpec string, cmd string) {
	termproxy.MakeRaw(0)
	os.Setenv("TERM", "screen-256color")

	s, err := server.NewSSHServer(listenSpec, *usernameFlag, *passwordFlag, *hostkeyFlag)

	if err != nil {
		termproxy.ErrorOut(fmt.Sprintf("Network Error trying to listen on %s", listenSpec), err, termproxy.ErrNetwork)
	}

	command := setCommand(cmd, s)
	go launch(command)

	input := new(bytes.Buffer)
	output := new(bytes.Buffer)

	ptyCopier := termproxy.NewCopier()
	ptyCopier.Handler = func(buf []byte, w io.Writer, r io.Reader) ([]byte, error) {
		input.Reset()
		return buf, nil
	}

	outputCopier := termproxy.NewCopier()
	inputCopier := termproxy.NewCopier()

	go inputCopier.Copy(input, os.Stdin)
	go writeOutputPty(outputCopier, output, command)
	go writePtyInput(ptyCopier, input, command)
	go writePtyOutput(output, s)

	s.AcceptHandler = func(c net.Conn) {
		c.Write([]byte("Connected to server\n"))
		time.Sleep(1 * time.Second)
		termproxy.WriteClear(c)
		if !*readOnly {
			inputCopier.Copy(input, c)
		}
	}

	s.CloseHandler = func(conn net.Conn) {
		winsizeMutex.Lock()
		delete(connectionWinsizeMap, conn.RemoteAddr().String())
		winsizeMutex.Unlock()
		conn.Close()
	}

	go func() {
		for {
			myWinch := <-s.InWinch
			compareAndSetWinsize(myWinch.Conn.RemoteAddr().String(), myWinch, command, s)
		}
	}()

	s.Listen()

	termproxy.ErrorOut("Shell Exited!", nil, 0)
}

func writeOutputPty(outputCopier *termproxy.Copier, output *bytes.Buffer, command *termproxy.Command) {
	for {
		if err := outputCopier.Copy(output, command.PTY()); err != nil {
			continue
		}
		time.Sleep(TIME_WAIT)
	}
}

func writePtyInput(ptyCopier *termproxy.Copier, input *bytes.Buffer, command *termproxy.Command) {
	for {
		if input.Len() == 0 {
			time.Sleep(TIME_WAIT)
			continue
		}

		ptyCopier.Copy(command.PTY(), input)
	}
}

func writePtyOutput(output *bytes.Buffer, t *server.SSHServer) {
	for {
		if output.Len() == 0 {
			time.Sleep(TIME_WAIT)
			continue
		}

		buf := output.Bytes()

		if _, err := os.Stdout.Write(buf); err != nil {
			break
		}

		t.MultiCopy(buf)
		output.Reset()
	}
}
