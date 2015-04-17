package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/docker/pkg/term"
	"github.com/erikh/termproxy/framing"
	"github.com/erikh/termproxy/tperror"
	"github.com/kr/pty"
	"github.com/ogier/pflag"
)

var DEBUG = os.Getenv("DEBUG")

var (
	mutex       = new(sync.Mutex)
	connMutex   = new(sync.Mutex)
	connections = []net.Conn{}
	windowState *term.State
)

var (
	caCertPath     = pflag.String("ca", "ca.crt", "Path to CA Certificate")
	serverCertPath = pflag.StringP("cert", "c", "server.crt", "Path to server certificate")
	serverKeyPath  = pflag.StringP("key", "k", "server.key", "Path to server key")
)

func diag(m error) {
	if DEBUG != "" {
		fmt.Fprintf(os.Stderr, "%v", m)
	}
}

func errorOut(e *tperror.TPError) {
	if err := term.RestoreTerminal(0, windowState); err != nil {
		tperror.ErrorOut(&tperror.TPError{fmt.Sprintf("Could not restore the terminal size during exit: %v", err), tperror.ErrTerminal})
	}

	tperror.ErrorOut(e)
}

func main() {
	pflag.Usage = func() {
		fmt.Printf("usage: %s <options> [host] [program]\n", filepath.Base(os.Args[0]))
		pflag.PrintDefaults()
		os.Exit(int(tperror.ErrUsage))
	}

	pflag.Parse()

	if pflag.NArg() != 2 {
		pflag.Usage()
	}

	serve()
}

func setTerminal() error {
	var err error
	windowState, err = term.MakeRaw(0)
	return err
}

func waitForClose(cmd *exec.Cmd, pty *os.File) {
	cmd.Wait()

	// FIXME sloppy as heck but works for now.
	for _, c := range connections {
		c.Close()
	}

	pty.Close()

	if err := term.RestoreTerminal(0, windowState); err != nil {
		errorOut(&tperror.TPError{fmt.Sprintf("Could not restore the terminal size during exit: %v", err), tperror.ErrTerminal})
	}

	fmt.Println()
	fmt.Println("Shell exited!")

	os.Exit(0)
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

func setPTYTerminal(pty *os.File) *tperror.TPError {
	ws, err := term.GetWinsize(0)
	if err != nil {
		return &tperror.TPError{fmt.Sprintf("Could not retrieve the terminal dimensions: %v", err), tperror.ErrTerminal}
	}

	if err := term.SetWinsize(pty.Fd(), ws); err != nil {
		return &tperror.TPError{fmt.Sprintf("Could not set the terminal size of the PTY: %v", err), tperror.ErrTerminal}
	}

	return nil
}

func loadCerts() (tls.Certificate, *x509.CertPool, *tperror.TPError) {
	cert, err := tls.LoadX509KeyPair(*serverCertPath, *serverKeyPath)
	if err != nil {
		return tls.Certificate{}, nil, &tperror.TPError{fmt.Sprintf("TLS certificate load error for %s, %s: %v", err), tperror.ErrTLS}
	}

	content, err := ioutil.ReadFile(*caCertPath)
	if err != nil {
		return tls.Certificate{}, nil, &tperror.TPError{fmt.Sprintf("TLS certificate load error for %s: %v", err), tperror.ErrTLS}
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(content)

	return cert, pool, nil
}

func readRemoteInput(c net.Conn, input *bytes.Buffer) {
	for {
		buf := make([]byte, 256)
		n, err := c.Read(buf)
		if err != nil {
			return
		}

		mutex.Lock()
		input.Write(buf[:n])
		mutex.Unlock()
	}
}

func listen(l net.Listener, pty *os.File, input *bytes.Buffer) {
	for {
		c, err := l.Accept()
		if err != nil {
			diag(err)
			continue
		}

		connMutex.Lock()
		connections = append(connections, c)
		connMutex.Unlock()

		go runStreamLoop(c, pty, input)
	}
}

func copyStdin(input *bytes.Buffer) {
	for {
		buf := make([]byte, 256)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}

		mutex.Lock()
		input.Write(buf[:n])
		mutex.Unlock()
	}
}

func copyPTY(pty *os.File, output io.Writer) {
	for {
		buf := make([]byte, 256)
		n, err := pty.Read(buf)
		if err != nil {
			return
		}

		mutex.Lock()
		output.Write(buf[:n])
		mutex.Unlock()
	}
}

func serve() {
	if err := setTerminal(); err != nil {
		errorOut(&tperror.TPError{fmt.Sprintf("Could not create a raw terminal: %v", err), tperror.ErrTerminal})
	}

	cmd := pflag.Arg(1)

	pty, err := startCommand(cmd)
	if err != nil {
		errorOut(&tperror.TPError{fmt.Sprintf("Could not start program %s: %v", cmd, err), tperror.ErrCommand})
	}

	if tperr := setPTYTerminal(pty); tperr != nil {
		errorOut(tperr)
	}

	cert, pool, tperr := loadCerts()
	if tperr != nil {
		errorOut(tperr)
	}

	l, err := tls.Listen("tcp", pflag.Arg(0), &tls.Config{
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})

	if err != nil {
		errorOut(&tperror.TPError{fmt.Sprintf("Network Error trying to listen on %s: %v", pflag.Arg(0)), tperror.ErrNetwork})
	}

	input := new(bytes.Buffer)
	output := new(bytes.Buffer)

	go listen(l, pty, input)
	go copyStdin(input)
	go copyPTY(pty, output)

	// there's gotta be a good way to do this in an evented/blocking manner.
	for {
		if input.Len() > 0 {
			mutex.Lock()
			if _, err := input.WriteTo(pty); err != nil {
				break
			}

			input.Reset()
			mutex.Unlock()
		}

		if output.Len() > 0 {
			mutex.Lock()

			connMutex.Lock()
			for i, c := range connections {
				data := &framing.Data{}
				data.WriteType(c)
				data.Data = output.Bytes()
				if err := data.WriteTo(c); err != nil {
					if len(connections)+1 > len(connections) {
						connections = connections[:i]
					} else {
						c.Close()
						connections = append(connections[:i], connections[i+1:]...)
					}
				}
			}
			connMutex.Unlock()

			if _, err := os.Stdout.Write(output.Bytes()); err != nil {
				break
			}

			output.Reset()
			mutex.Unlock()
		}

		time.Sleep(20 * time.Millisecond)
	}

	term.RestoreTerminal(0, windowState)
	fmt.Println("Shell exited!")
}

func runStreamLoop(c net.Conn, pty *os.File, input io.Writer) {
	s := &framing.StreamParser{
		Reader: c,
		DataHandler: func(r io.Reader) error {
			data := &framing.Data{}
			data.ReadFrom(r)
			_, err := io.Copy(input, bytes.NewBuffer(data.Data))
			return err
		},
		WinchHandler: func(r io.Reader) error {
			winch := &framing.Winch{}
			if err := winch.ReadFrom(r); err != nil {
				return err
			}

			if err := term.SetWinsize(pty.Fd(), &term.Winsize{Height: winch.Height, Width: winch.Width}); err != nil {
				return err
			}

			return nil
		},
	}

	s.Loop()
}
