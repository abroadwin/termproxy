package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/framing"
	"github.com/ogier/pflag"
)

var DEBUG = os.Getenv("DEBUG")

var (
	mutex                = new(sync.Mutex)
	connMutex            = new(sync.Mutex)
	winsizeMutex         = new(sync.Mutex)
	connections          = []net.Conn{}
	connectionWinsizeMap = map[string]*framing.Winch{}
)

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

func compareAndSetWinsize(host string, ws *framing.Winch, command *termproxy.Command) {
	winsizeMutex.Lock()
	connectionWinsizeMap[host] = ws

	var height, width uint16

	for _, wm := range connectionWinsizeMap {
		if height == 0 || width == 0 {
			height = wm.Height
			width = wm.Width
		}
		if wm.Height <= height {
			height = wm.Height
		}

		if wm.Width <= width {
			width = wm.Width
		}
	}

	winsize := &framing.Winch{Height: height, Width: width}

	myws, _ := termproxy.GetWinsize(command.PTY().Fd())

	if winsize.Height != myws.Height || winsize.Width != myws.Width {
		// send the clear only in the height case, it will resolve itself with width.

		if winsize.Height != myws.Height {
			termproxy.WriteClear(os.Stdout)
		}

		termproxy.SetWinsize(command.PTY().Fd(), winsize)

		connMutex.Lock()
		for i, c := range connections {
			var prune bool

			if err := winsize.WriteTo(c); err != nil {
				prune = true
			}

			if prune {
				var err error
				connections, err = pruneConnection(connections, i, err)
				if err != nil {
					termproxy.ErrorOut("Error closing connection", err, termproxy.ErrNetwork)
				}
			}
		}
		connMutex.Unlock()
	}

	winsizeMutex.Unlock()
}

func handleWinch(command *termproxy.Command) {
	ws, err := termproxy.GetWinsize(0)
	if err != nil {
		termproxy.ErrorOut("Could not retrieve the terminal size: %v", err, termproxy.ErrTerminal)
	}

	compareAndSetWinsize("localhost", ws, command)
}

func setPTYTerminal(command *termproxy.Command) {
	ws, err := termproxy.GetWinsize(0)
	if err != nil {
		termproxy.ErrorOut("Could not retrieve the terminal dimensions", err, termproxy.ErrTerminal)
	}

	compareAndSetWinsize("localhost", ws, command)

	if err := termproxy.SetWinsize(command.PTY().Fd(), ws); err != nil {
		termproxy.ErrorOut("Could not set the terminal size of the PTY", err, termproxy.ErrTerminal)
	}
}

func loadCerts() (tls.Certificate, *x509.CertPool) {
	cert := termproxy.LoadCert(*serverCertPath, *serverKeyPath)
	pool := x509.NewCertPool()
	termproxy.LoadCertIntoPool(pool, *caCertPath)

	return cert, pool
}

func listen(l net.Listener, input *bytes.Buffer, command *termproxy.Command) {
	for {
		c, err := l.Accept()
		if err != nil {
			continue
		}

		connMutex.Lock()
		connections = append(connections, c)
		connMutex.Unlock()

		go runStreamLoop(c, input, command)
	}
}

// presumes the lock has already been acquired
func pruneConnection(writers []net.Conn, i int, err error) ([]net.Conn, error) {
	connections := writers[:]
	delete(connectionWinsizeMap, writers[i].(*tls.Conn).RemoteAddr().String())
	writers[i].Close()

	if len(connections)+1 > len(connections) {
		connections = connections[:i]
	} else {
		connections = append(connections[:i], connections[i+1:]...)
	}

	return connections, nil
}

func closeHandler(command *termproxy.Command) {
	connMutex.Lock()
	// FIXME sloppy as heck but works for now.
	for _, conn := range connections {
		conn.Close()
	}
	connMutex.Unlock()

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

	command := termproxy.NewCommand(cmd)
	command.CloseHandler = closeHandler
	command.PTYSetupHandler = setPTYTerminal
	command.WinchHandler = handleWinch

	go launch(command)

	cert, pool := loadCerts()

	l, err := tls.Listen("tcp", listenSpec, &tls.Config{
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})

	if err != nil {
		termproxy.ErrorOut(fmt.Sprintf("Network Error trying to listen on %s", pflag.Arg(0)), err, termproxy.ErrNetwork)
	}

	input := new(bytes.Buffer)
	output := new(bytes.Buffer)

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGWINCH)

	go listen(l, input, command)

	copier := termproxy.NewCopier(nil)

	go copier.Copy(input, os.Stdin)
	go copier.Copy(output, command.PTY())

	copier.Handler = func(buf []byte, w io.Writer, r io.Reader) ([]byte, error) {
		input.Reset()
		return buf, nil
	}

	multiCopier := termproxy.NewMultiCopier(copier)
	multiCopier.ErrorHandler = pruneConnection
	multiCopier.Handler = func(buf []byte, writers []net.Conn, reader io.Reader) ([]byte, error) {
		if _, err := os.Stdout.Write(buf); err != nil {
			return nil, err
		}

		output.Reset()
		return buf, nil
	}

	// there's gotta be a good way to do this in an evented/blocking manner.
	for {
		if input.Len() > 0 {
			copier.Copy(command.PTY(), input)
		}

		if output.Len() > 0 {
			connMutex.Lock()
			connections, _ = multiCopier.CopyFrame(connections, output, output.Len())
			if err != nil {
				connMutex.Unlock()
			}
			connMutex.Unlock()
		}

		time.Sleep(20 * time.Millisecond)
	}

	termproxy.ErrorOut("Shell Exited!", nil, 0)
}

func runStreamLoop(c net.Conn, input io.Writer, command *termproxy.Command) {
	s := &framing.StreamParser{
		Reader: c,
		DataHandler: func(data *framing.Data) error {
			_, err := io.Copy(input, bytes.NewBuffer(data.Data))
			return err
		},
		WinchHandler: func(winch *framing.Winch) error {
			compareAndSetWinsize(c.(*tls.Conn).RemoteAddr().String(), winch, command)
			return nil
		},
	}

	s.Loop()
}
