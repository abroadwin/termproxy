package termproxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
)

func LoadCert(certPath, keyPath string) tls.Certificate {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		ErrorOut(fmt.Sprintf("TLS certificate load error for %s, %s", certPath, keyPath), err, ErrTLS)
	}

	return cert
}

func LoadCertIntoPool(pool *x509.CertPool, certPath string) {
	content, err := ioutil.ReadFile(certPath)
	if err != nil {
		ErrorOut(fmt.Sprintf("TLS certificate load error for %s", certPath), err, ErrTLS)
	}

	pool.AppendCertsFromPEM(content)
}
