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
	"os/signal"
	"syscall"

	"github.com/docker/docker/pkg/term"
	"github.com/erikh/termproxy/framing"
	"github.com/erikh/termproxy/tperror"
	"github.com/ogier/pflag"
	"golang.org/x/sys/unix"
)

var (
	windowState *term.State
)

var (
	caCertPath     = pflag.String("ca", "ca.crt", "Path to CA Certificate")
	serverCertPath = pflag.StringP("servercert", "s", "server.crt", "Path to Server Certificate")
	clientCertPath = pflag.StringP("cert", "c", "client.crt", "Path to Client Certificate")
	clientKeyPath  = pflag.StringP("key", "k", "client.key", "Path to Client Key")
)

func errorOut(err *tperror.TPError) {
	term.RestoreTerminal(0, windowState)
	tperror.ErrorOut(err)
}

func readCerts() (tls.Certificate, *x509.CertPool, *tperror.TPError) {
	content, err := ioutil.ReadFile(*caCertPath)
	if err != nil {
		return tls.Certificate{}, nil, &tperror.TPError{fmt.Sprintf("Could not read CA certificate '%s': %v", *caCertPath, err), tperror.ErrTLS}
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(content)

	content, err = ioutil.ReadFile(*serverCertPath)
	if err != nil {
		return tls.Certificate{}, nil, &tperror.TPError{fmt.Sprintf("Could not read server certificate '%s': %v", *serverCertPath, err), tperror.ErrTLS}
	}

	pool.AppendCertsFromPEM(content)

	cert, err := tls.LoadX509KeyPair(*clientCertPath, *clientKeyPath)
	if err != nil {
		return tls.Certificate{}, nil, &tperror.TPError{fmt.Sprintf("Could not read client keypair '%s' and '%s': %v", *clientCertPath, *clientKeyPath, err), tperror.ErrTLS}
	}

	return cert, pool, nil
}

func connect() net.Conn {
	cert, pool, tperr := readCerts()
	if tperr != nil {
		errorOut(tperr)
	}

	c, err := tls.Dial("tcp", pflag.Arg(0), &tls.Config{
		ClientCAs:    pool,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		errorOut(&tperror.TPError{fmt.Sprintf("Could not connect to server at %s: %v", pflag.Arg(0), err), tperror.ErrTLS | tperror.ErrNetwork})
	}

	return c
}

func configureTerminal() (*term.Winsize, *tperror.TPError) {
	var err error

	windowState, err = term.MakeRaw(0)
	if err != nil {
		return nil, &tperror.TPError{fmt.Sprintf("Could not create a raw terminal: %v", err), tperror.ErrTerminal}
	}

	ws, err := term.GetWinsize(0)
	if err != nil {
		return nil, &tperror.TPError{fmt.Sprintf("Error getting terminal size: %v", err), tperror.ErrTerminal}
	}

	return ws, nil
}

func writeTermSize(c net.Conn) *tperror.TPError {
	ws, tperr := configureTerminal()
	if tperr != nil {
		return tperr
	}

	winch := &framing.Winch{Height: ws.Height, Width: ws.Width}

	winch.WriteType(c)
	if err := winch.WriteTo(c); err != nil {
		return &tperror.TPError{fmt.Sprintf("Error writing terminal size to server: %v", err), tperror.ErrNetwork | tperror.ErrTerminal}
	}

	return nil
}

func copyStdin(c net.Conn) {
	var breakpressed bool
	var pPressed bool

	for {
		data := &framing.Data{}
		buf := make([]byte, 256)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			errorOut(&tperror.TPError{"Error reading from Stdin", tperror.ErrTerminal})
		}

		buf = buf[:n]

		if bytes.Contains(buf, []byte{16, 17}) {
			buf = bytes.Replace(buf, []byte{16, 17}, []byte{}, -1)
			breakpressed = true
		}

		if bytes.HasPrefix(buf, []byte{17}) && pPressed {
			buf = bytes.Replace(buf, []byte{17}, []byte{}, 1)
			breakpressed = true
		}

		if bytes.HasSuffix(buf, []byte{16}) {
			buf = bytes.TrimRight(buf, string([]byte{16}))
			pPressed = true
		}

		if breakpressed {
			term.RestoreTerminal(0, windowState)
			fmt.Println("\n\nConnection terminated!")
			os.Exit(0)
		}

		data.Data = buf
		data.WriteType(c)
		if err := data.WriteTo(c); err != nil && err != io.EOF {
			if neterr, ok := err.(*net.OpError); ok && neterr.Err == unix.EPIPE {
				term.RestoreTerminal(0, windowState)
				fmt.Println("\n\nConnection terminated!")
				os.Exit(0)
			} else {
				errorOut(&tperror.TPError{fmt.Sprintf("Error writing to server: %v", err), tperror.ErrNetwork})
			}
		}
	}
}

func sigwinchHandler(sigchan chan os.Signal, c net.Conn) {
	for {
		<-sigchan
		ws, err := term.GetWinsize(0)
		if err != nil {
			errorOut(&tperror.TPError{fmt.Sprintf("Error getting terminal size, aborting: %v", err), tperror.ErrTerminal})
		}

		winch := &framing.Winch{Height: ws.Height, Width: ws.Width}
		if err := winch.WriteType(c); err != nil {
			errorOut(&tperror.TPError{fmt.Sprintf("Error writing winch to server: %v", err), tperror.ErrNetwork})
		}

		if err := winch.WriteTo(c); err != nil {
			errorOut(&tperror.TPError{fmt.Sprintf("Error writing winch to server: %v", err), tperror.ErrNetwork})
		}
	}
}

func main() {
	pflag.Parse()

	if pflag.NArg() != 1 {
		fmt.Printf("usage: %s [host]\n", os.Args[0])
		pflag.PrintDefaults()
		os.Exit(1)
	}

	c := connect()
	writeTermSize(c)

	sigchan := make(chan os.Signal)

	signal.Notify(sigchan, syscall.SIGWINCH)

	go sigwinchHandler(sigchan, c)
	go copyStdin(c)

	s := framing.StreamParser{
		Reader: c,
		ErrorHandler: func(err error) {
			errorOut(&tperror.TPError{err.Error(), tperror.ErrCommand})
		},
		DataHandler: func(r io.Reader) error {
			data := &framing.Data{}
			if err := data.ReadFrom(r); err != nil {
				return err
			}

			if _, err := os.Stdout.Write(data.Data); err != nil {
				return err
			}

			return nil
		},
		WinchHandler: func(r io.Reader) error {
			winch := &framing.Winch{}
			if err := winch.ReadFrom(r); err != nil {
				return err
			}

			if err := term.SetWinsize(0, &term.Winsize{Height: winch.Height, Width: winch.Width}); err != nil {
				return err
			}

			return nil
		},
	}

	s.Loop()
}
