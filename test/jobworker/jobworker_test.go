package jobworker

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/tjper/teleport/internal/encrypt"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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
				t.Fatalf("unexpected error: %v", err)
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
				t.Fatalf("unexpected error; actual: %v, expected: %v", err, test.exp.err)
			}
			if err == nil {
				conn.Close()
			}
		})
	}
}

func TestStart(t *testing.T) {
	type expected struct {
		resp *pb.StartResponse
		code codes.Code
	}
	tests := map[string]struct {
		req *pb.StartRequest
		exp expected
	}{
		"ls": {
			req: &pb.StartRequest{
				Command: &pb.Command{Name: "ls"},
				Limits:  &pb.Limits{},
			},
			exp: expected{
				resp: &pb.StartResponse{
					Command: &pb.Command{Name: "ls"},
					Status:  &pb.StatusDetail{Status: pb.Status_STATUS_PENDING, ExitCode: -1},
					Limits:  &pb.Limits{},
				},
				code: codes.OK,
			},
		},
		"ls -la": {
			req: &pb.StartRequest{
				Command: &pb.Command{Name: "ls", Args: []string{"-la"}},
				Limits:  &pb.Limits{},
			},
			exp: expected{
				resp: &pb.StartResponse{
					Command: &pb.Command{Name: "ls", Args: []string{"-la"}},
					Status:  &pb.StatusDetail{Status: pb.Status_STATUS_PENDING, ExitCode: -1},
					Limits:  &pb.Limits{},
				},
				code: codes.OK,
			},
		},
		"ls w/ limits": {
			req: &pb.StartRequest{
				Command: &pb.Command{Name: "ls"},
				Limits: &pb.Limits{
					Memory:       100000,
					Cpus:         1.5,
					DiskWriteBps: 10000,
					DiskReadBps:  10000,
				},
			},
			exp: expected{
				resp: &pb.StartResponse{
					Command: &pb.Command{Name: "ls"},
					Status:  &pb.StatusDetail{Status: pb.Status_STATUS_PENDING, ExitCode: -1},
					Limits: &pb.Limits{
						Memory:       100000,
						Cpus:         1.5,
						DiskWriteBps: 10000,
						DiskReadBps:  10000,
					},
				},
				code: codes.OK,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite := setup(t)
			defer suite.close(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := suite.client.Start(ctx, test.req)
			if status.Code(err) != test.exp.code {
				t.Fatalf("unexpected code; actual: %v, expected: %v", status.Code(err), test.exp.code)
			}

			if len(resp.JobId) == 0 {
				t.Fatal("expected JobId in response")
			}
			resp.JobId = ""

			if !proto.Equal(resp, test.exp.resp) {
				t.Fatalf("unexpected response; actual: %v, expected: %v", resp, test.exp.resp)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	type expected struct {
		resp *pb.StatusResponse
		code codes.Code
	}
	tests := map[string]struct {
		start *pb.StartRequest
		wait  time.Duration
		exp   expected
	}{
		"ls": {
			start: &pb.StartRequest{
				Command: &pb.Command{Name: "ls"},
				Limits:  &pb.Limits{},
			},
			wait: 200 * time.Millisecond,
			exp: expected{
				resp: &pb.StatusResponse{
					Status: &pb.StatusDetail{Status: pb.Status_STATUS_EXITED, ExitCode: 0},
				},
				code: codes.OK,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite := setup(t)
			defer suite.close(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			startResp, err := suite.client.Start(ctx, test.start)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			time.Sleep(test.wait)

			resp, err := suite.client.Status(ctx, &pb.StatusRequest{JobId: startResp.JobId})
			if status.Code(err) != test.exp.code {
				t.Fatalf("unexpected code; actual: %v, expected: %v", status.Code(err), test.exp.code)
			}
			if !proto.Equal(resp, test.exp.resp) {
				t.Fatalf("unexpected response; actual: %v, expected: %v", resp, test.exp.resp)
			}
		})
	}
}

func TestStop(t *testing.T) {
	type expected struct {
		code codes.Code
	}
	tests := map[string]struct {
		start *pb.StartRequest
		exp   expected
	}{
		"sleep 10": {
			start: &pb.StartRequest{
				Command: &pb.Command{Name: "sleep", Args: []string{"10"}},
				Limits:  &pb.Limits{},
			},
			exp: expected{
				code: codes.OK,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			suite := setup(t)
			defer suite.close(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			startResp, err := suite.client.Start(ctx, test.start)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			_, err = suite.client.Stop(ctx, &pb.StopRequest{JobId: startResp.JobId})
			if status.Code(err) != test.exp.code {
				t.Fatalf("unexpected code; actual: %v, expected: %v", status.Code(err), test.exp.code)
			}

			statusResp, err := suite.client.Status(ctx, &pb.StatusRequest{JobId: startResp.JobId})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if statusResp.Status.Status != pb.Status_STATUS_STOPPED {
				t.Fatalf("unexpected status; actual: %s, expected: %s", statusResp.Status.Status, pb.Status_STATUS_STOPPED)
			}
		})
	}
}

func setup(t *testing.T) *suite {
	clientCert := "../../certs/alpha_user.crt"
	clientKey := "../../certs/alpha_user.key"
	caCert := "../../certs/ca.crt"

	config, err := encrypt.NewClientTLSConfig(clientCert, clientKey, caCert)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf(":%d", *port),
		grpc.WithTransportCredentials(credentials.NewTLS(config)),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return &suite{
		conn:   conn,
		client: pb.NewJobWorkerServiceClient(conn),
	}
}

type suite struct {
	conn   *grpc.ClientConn
	client pb.JobWorkerServiceClient
}

func (s suite) close(t *testing.T) {
	if err := s.conn.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
