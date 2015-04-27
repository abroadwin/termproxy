package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/framing"
	"github.com/erikh/termproxy/server"
	"github.com/ogier/pflag"
)

var DEBUG = os.Getenv("DEBUG")

var (
	caCertPath     = pflag.String("ca", "ca.crt", "Path to CA Certificate")
	serverCertPath = pflag.StringP("cert", "c", "server.crt", "Path to server certificate")
	serverKeyPath  = pflag.StringP("key", "k", "server.key", "Path to server key")
)

func main() {
	pflag.Usage = func() {
		fmt.Printf("usage: %s <options> [host] [program]\n", filepath.Base(os.Args[0]))
		pflag.PrintDefaults()
		os.Exit(int(termproxy.ErrUsage))
	}

	pflag.Parse()

	if pflag.NArg() != 2 {
		pflag.Usage()
	}

	serve(pflag.Arg(0), pflag.Arg(1))
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

	ioLock := new(sync.Mutex)
	t, err := server.NewTLSServer(listenSpec, *serverCertPath, *serverKeyPath, *caCertPath)

	if err != nil {
		termproxy.ErrorOut(fmt.Sprintf("Network Error trying to listen on %s", pflag.Arg(0)), err, termproxy.ErrNetwork)
	}

	command := termproxy.NewCommand(cmd)
	command.CloseHandler = closeHandler(t)
	command.PTYSetupHandler = setPTYTerminal(t)
	command.WinchHandler = handleWinch(t)

	go launch(command)

	input := new(bytes.Buffer)
	output := new(bytes.Buffer)

	t.AcceptHandler = func(c net.Conn) {
		go runStreamLoop(c, input, command, t)
	}

	t.CloseHandler = func(conn net.Conn) {
		winsizeMutex.Lock()
		delete(connectionWinsizeMap, conn.(*tls.Conn).RemoteAddr().String())
		winsizeMutex.Unlock()
		conn.Close()
	}

	go t.Listen()

	ptyCopier := termproxy.NewCopier(ioLock)
	ptyCopier.Handler = func(buf []byte, w io.Writer, r io.Reader) ([]byte, error) {
		input.Reset()
		return buf, nil
	}

	outputCopier := termproxy.NewCopier(nil)
	inputCopier := termproxy.NewCopier(nil)

	go inputCopier.Copy(input, os.Stdin)

	go func() {
		for {
			if err := outputCopier.Copy(output, command.PTY()); err != nil {
				fmt.Println(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	go func() {
		for {
			if input.Len() == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			ptyCopier.Copy(command.PTY(), input)
		}
	}()

	for {
		if output.Len() == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		//ioLock.Lock()
		buf := output.Bytes()

		if _, err := os.Stdout.Write(buf); err != nil {
			break
		}

		t.MultiCopy(buf)
		output.Reset()
		//ioLock.Unlock()
	}

	termproxy.ErrorOut("Shell Exited!", nil, 0)
}

func runStreamLoop(c net.Conn, input io.Writer, command *termproxy.Command, t *server.TLSServer) {
	s := &framing.StreamParser{
		Reader: c,
		DataHandler: func(data *framing.Data) error {
			_, err := io.Copy(input, bytes.NewBuffer(data.Data))
			return err
		},
		WinchHandler: func(winch *framing.Winch) error {
			compareAndSetWinsize(c.(*tls.Conn).RemoteAddr().String(), winch, command, t)
			return nil
		},
	}

	s.Loop()
}
