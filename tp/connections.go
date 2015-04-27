package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net"
	"sync"

	"github.com/erikh/termproxy"
)

var (
	connMutex   = new(sync.Mutex)
	connections = []net.Conn{}
)

func setupListener(listenSpec string, pool *x509.CertPool, cert tls.Certificate) (net.Listener, error) {
	return tls.Listen("tcp", listenSpec, &tls.Config{
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
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

func loadCerts() (tls.Certificate, *x509.CertPool) {
	cert := termproxy.LoadCert(*serverCertPath, *serverKeyPath)
	pool := x509.NewCertPool()
	termproxy.LoadCertIntoPool(pool, *caCertPath)

	return cert, pool
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
