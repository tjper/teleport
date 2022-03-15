package jobworker

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/tjper/teleport/internal/encrypt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var port = flag.Int("port", 8080, "port jobworker API is serving content on")

func TestAuthentication(t *testing.T) {
	flag.Parse()

	type expected struct {
		err error
	}
	tests := map[string]struct {
		clientCert string
		clientKey  string
		caCert     string
		exp        expected
	}{
		"authenticate": {
			clientCert: "../../certs/alpha_user.crt",
			clientKey:  "../../certs/alpha_user.key",
			caCert:     "../../certs/ca.crt",
			exp:        expected{err: nil},
		},
		"not signed by ca": {
			clientCert: "../../certs/unknown_user.crt",
			clientKey:  "../../certs/unknown_user.key",
			caCert:     "../../certs/ca.crt",
			exp:        expected{err: context.DeadlineExceeded},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := encrypt.NewClientTLSConfig(test.clientCert, test.clientKey, test.caCert)
			if err != nil {
				t.Errorf("unexpected error: %+v", err)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			conn, err := grpc.DialContext(
				ctx,
				fmt.Sprintf(":%d", *port),
				grpc.WithTransportCredentials(credentials.NewTLS(config)),
				grpc.WithBlock(),
			)
			if !errors.Is(err, test.exp.err) {
				t.Errorf("unexpected error; actual: %v, expected: %v", err, test.exp.err)
			}
			if err == nil {
				conn.Close()
			}
		})
	}
}

// func TestStartJob(t *testing.T) {

// }
