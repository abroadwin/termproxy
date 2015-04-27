package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/erikh/termproxy"
)

type TLSServer struct {
	AcceptHandler func(net.Conn)
	CloseHandler  func(net.Conn)

	copier      *termproxy.Copier
	connections []net.Conn

	listener    net.Listener
	certPool    *x509.CertPool
	certificate tls.Certificate

	mutex sync.Mutex
}

func defaultCloseHandler(conn net.Conn) {
	conn.Close()
}

func (t *TLSServer) loadCerts(serverCert, serverKey, caCert string) {
	t.certificate = termproxy.LoadCert(serverCert, serverKey)
	t.certPool = x509.NewCertPool()
	termproxy.LoadCertIntoPool(t.certPool, caCert)
}

func NewTLSServer(listenSpec, serverCert, serverKey, caCert string) (*TLSServer, error) {
	t := &TLSServer{}

	t.loadCerts(serverCert, serverKey, caCert)

	listener, err := tls.Listen("tcp", listenSpec, &tls.Config{
		RootCAs:      t.certPool,
		ClientCAs:    t.certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{t.certificate},
		MinVersion:   tls.VersionTLS12,
	})

	if err != nil {
		return nil, err
	}

	t.CloseHandler = defaultCloseHandler
	t.copier = termproxy.NewCopier(nil)
	t.listener = listener

	return t, nil
}

func (t *TLSServer) Listen() {
	for {
		c, err := t.listener.Accept()
		if err != nil {
			continue
		}

		t.mutex.Lock()
		t.connections = append(t.connections, c)
		t.mutex.Unlock()

		t.AcceptHandler(c)
	}
}

func (t *TLSServer) Prune(i int) {
	if len(t.connections)-1 == i {
		t.connections = t.connections[:i]
	} else {
		t.connections = append(t.connections[:i], t.connections[i+1:]...)
	}
}

func (t *TLSServer) Iterate(iterator func(t *TLSServer, conn net.Conn, index int) error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for i, conn := range t.connections {
		if err := iterator(t, conn, i); err != nil {
			t.CloseHandler(conn)
			t.Prune(i)
		}
	}
}

func (t *TLSServer) MultiCopy(buf []byte) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for i, conn := range t.connections {
		if err := t.copier.CopyFrames(conn, bytes.NewBuffer(buf)); err != nil && err != io.EOF {
			fmt.Println(err)
			t.CloseHandler(conn)
			t.Prune(i)
		}
	}
}
