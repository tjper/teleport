// Package encrypt provides and API for encryption utilities.
package encrypt

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/pkg/errors"
)

var errInvalidCaCert = errors.New("invalid ca cert")

// NewServerTLSConfig creates a tls.Config suited for a server using mTLS.
func NewServermTLSConfig(serverCert, serverKey, caCert string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ca := x509.NewCertPool()
	b, err := ioutil.ReadFile(caCert)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if ok := ca.AppendCertsFromPEM(b); !ok {
		return nil, errors.WithStack(errInvalidCaCert)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    ca,
	}, nil
}

// NewClientTLSConfig creates a tls.Config suited for a client using mTLS.
func NewClientTLSConfig(clientCert, clientKey, caCert string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ca := x509.NewCertPool()
	b, err := ioutil.ReadFile(caCert)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if ok := ca.AppendCertsFromPEM(b); !ok {
		return nil, errors.WithStack(errInvalidCaCert)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		ServerName:   "localhost",
		Certificates: []tls.Certificate{cert},
		RootCAs:      ca,
	}, nil
}
