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
	"syscall"

	"github.com/erikh/termproxy"
	"github.com/erikh/termproxy/framing"
	"github.com/ogier/pflag"
	"golang.org/x/sys/unix"
)

var (
	caCertPath     = pflag.String("ca", "ca.crt", "Path to CA Certificate")
	serverCertPath = pflag.StringP("servercert", "s", "server.crt", "Path to Server Certificate")
	clientCertPath = pflag.StringP("cert", "c", "client.crt", "Path to Client Certificate")
	clientKeyPath  = pflag.StringP("key", "k", "client.key", "Path to Client Key")
)

func readCerts() (tls.Certificate, *x509.CertPool) {
	pool := x509.NewCertPool()
	termproxy.LoadCertIntoPool(pool, *caCertPath)
	termproxy.LoadCertIntoPool(pool, *serverCertPath)
	cert := termproxy.LoadCert(*clientCertPath, *clientKeyPath)

	return cert, pool
}

func connect(host string) net.Conn {
	cert, pool := readCerts()

	c, err := tls.Dial("tcp", host, &tls.Config{
		ClientCAs:    pool,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		termproxy.ErrorOut(fmt.Sprintf("Could not connect to server at %s", pflag.Arg(0)), err, termproxy.ErrTLS|termproxy.ErrNetwork)
	}

	return c
}

func configureTerminal() *framing.Winch {
	var err error

	termproxy.MakeRaw(0)

	ws, err := termproxy.GetWinsize(0)
	if err != nil {
		termproxy.Exit(fmt.Sprintf("Error getting terminal size: %v", err), termproxy.ErrTerminal)
	}

	return ws
}

func writeTermSize(c net.Conn) {
	ws := configureTerminal()

	if err := ws.WriteTo(c); err != nil {
		termproxy.ErrorOut("Error writing terminal size to server: %v", err, termproxy.ErrNetwork|termproxy.ErrTerminal)
	}
}

func copyStdin(c net.Conn) {
	var breakpressed bool
	var pPressed bool

	copier := termproxy.NewCopier(nil)

	copier.Handler = func(buf []byte, w io.Writer, r io.Reader) error {
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
			termproxy.ErrorOut("Connection terminated!", nil, 0)
		}

		return nil
	}

	if err := copier.CopyFrames(c, os.Stdin); err != nil {
		if neterr, ok := err.(*net.OpError); ok && neterr.Err == unix.EPIPE {
			termproxy.ErrorOut("Connection terminated!", nil, 0)
		} else {
			termproxy.ErrorOut("Error writing to server", err, termproxy.ErrNetwork)
		}
	}
}

func sigwinchHandler(sigchan chan os.Signal, c net.Conn) {
	for {
		<-sigchan
		ws, err := termproxy.GetWinsize(0)
		if err != nil {
			termproxy.ErrorOut("Error getting terminal size", err, termproxy.ErrTerminal)
		}

		if err := ws.WriteTo(c); err != nil {
			termproxy.ErrorOut("Error writing winch to server", err, termproxy.ErrNetwork)
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

	c := connect(pflag.Arg(0))
	writeTermSize(c)

	sigchan := make(chan os.Signal)

	signal.Notify(sigchan, syscall.SIGWINCH)

	go sigwinchHandler(sigchan, c)
	go copyStdin(c)

	s := framing.StreamParser{
		Reader: c,
		ErrorHandler: func(err error) {
			if err == io.EOF {
				termproxy.ErrorOut("Connection terminated!", nil, 0)
			} else {
				termproxy.ErrorOut("Error", err, termproxy.ErrNetwork)
			}
		},
		DataHandler: func(data *framing.Data) error {
			if _, err := os.Stdout.Write(data.Data); err != nil {
				return err
			}

			return nil
		},
		WinchHandler: func(winch *framing.Winch) error {
			termproxy.WriteClear(os.Stdout)
			return nil
		},
	}

	s.Loop()
}
