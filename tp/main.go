package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/framing"
	"github.com/kr/pty"
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

func compareAndSetWinsize(host string, ws *framing.Winch, pty *os.File) {
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

	myws, _ := termproxy.GetWinsize(pty.Fd())

	if winsize.Height != myws.Height || winsize.Width != myws.Width {
		// Using all those BBSes in high school really mattered.
		termproxy.WriteClear(os.Stdout)

		termproxy.SetWinsize(pty.Fd(), winsize)
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

func handleWinch(sigchan chan os.Signal, pty *os.File) {
	for {
		<-sigchan
		ws, err := termproxy.GetWinsize(0)
		if err != nil {
			termproxy.ErrorOut("Could not retrieve the terminal size: %v", err, termproxy.ErrTerminal)
		}

		compareAndSetWinsize("localhost", ws, pty)
	}
}

func waitForClose(cmd *exec.Cmd, pty *os.File) {
	cmd.Wait()

	// FIXME sloppy as heck but works for now.
	for _, c := range connections {
		c.Close()
	}

	pty.Close()

	termproxy.ErrorOut("Shell Exited!", nil, 0)
}

func startCommand(command string) (*os.File, error) {
	cmd := exec.Command(command)
	pty, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	go waitForClose(cmd, pty)

	return pty, nil
}

func setPTYTerminal(pty *os.File) {
	ws, err := termproxy.GetWinsize(0)
	if err != nil {
		termproxy.ErrorOut("Could not retrieve the terminal dimensions", err, termproxy.ErrTerminal)
	}

	compareAndSetWinsize("localhost", ws, pty)

	if err := termproxy.SetWinsize(pty.Fd(), ws); err != nil {
		termproxy.ErrorOut("Could not set the terminal size of the PTY", err, termproxy.ErrTerminal)
	}
}

func loadCerts() (tls.Certificate, *x509.CertPool) {
	cert := termproxy.LoadCert(*serverCertPath, *serverKeyPath)
	pool := x509.NewCertPool()
	termproxy.LoadCertIntoPool(pool, *caCertPath)

	return cert, pool
}

func listen(l net.Listener, pty *os.File, input *bytes.Buffer) {
	for {
		c, err := l.Accept()
		if err != nil {
			continue
		}

		connMutex.Lock()
		connections = append(connections, c)
		connMutex.Unlock()

		go runStreamLoop(c, pty, input)
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

func serve(listenSpec, cmd string) {
	termproxy.MakeRaw(0)

	pty, err := startCommand(cmd)
	if err != nil {
		termproxy.ErrorOut(fmt.Sprintf("Could not start program %s", cmd), err, termproxy.ErrCommand)
	}

	setPTYTerminal(pty)

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
	go handleWinch(sigchan, pty)
	signal.Notify(sigchan, syscall.SIGWINCH)

	copier := termproxy.NewCopier(nil)

	go listen(l, pty, input)
	go copier.Copy(input, os.Stdin)
	go copier.Copy(output, pty)

	copier.Handler = func(buf []byte, w io.Writer, r io.Reader) error {
		input.Reset()
		return nil
	}

	multiCopier := termproxy.NewMultiCopier(copier)
	multiCopier.ErrorHandler = pruneConnection
	multiCopier.Handler = func(buf []byte, writers []net.Conn, reader io.Reader) error {
		if _, err := os.Stdout.Write(buf); err != nil {
			return err
		}

		output.Reset()
		return nil
	}

	// there's gotta be a good way to do this in an evented/blocking manner.
	for {
		if input.Len() > 0 {
			copier.Copy(pty, input)
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

func runStreamLoop(c net.Conn, pty *os.File, input io.Writer) {
	s := &framing.StreamParser{
		Reader: c,
		DataHandler: func(data *framing.Data) error {
			_, err := io.Copy(input, bytes.NewBuffer(data.Data))
			return err
		},
		WinchHandler: func(winch *framing.Winch) error {
			compareAndSetWinsize(c.(*tls.Conn).RemoteAddr().String(), winch, pty)
			return nil
		},
	}

	s.Loop()
}
