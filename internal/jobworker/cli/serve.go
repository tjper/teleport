package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/tjper/teleport/internal/jobworker/cgroup"
	igrpc "github.com/tjper/teleport/internal/jobworker/grpc"
	"github.com/tjper/teleport/internal/jobworker/job"
	"github.com/tjper/teleport/internal/jobworker/user"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func runServe(ctx context.Context) int {
	cgroupSvc, err := cgroup.NewService()
	if err != nil {
		logger.Errorf("cgroup service setup; error: %v", err)
		return ecCgroupService
	}
	defer cgroupSvc.Cleanup()

	jobSvc, err := job.NewService(cgroupSvc)
	if err != nil {
		logger.Errorf("job service setup; error: %v", err)
		return ecJobService
	}
	defer jobSvc.Close()

	userSvc := user.Service{}
	jw := igrpc.NewJobWorker(jobSvc, userSvc)

	cert, err := tls.LoadX509KeyPair(*cert, *key)
	if err != nil {
		logger.Errorf("load x509 key pair; error: %v", err)
		return ecLoadx509
	}

	ca := x509.NewCertPool()
	b, err := ioutil.ReadFile(*caCert)
	if err != nil {
		logger.Errorf("load CA certificate; error: %v", err)
		return ecLoadCaCert
	}
	if ok := ca.AppendCertsFromPEM(b); !ok {
		logger.Errorf("failed to build ca; cert: %s", caCert)
		return ecBuildCaCert
	}

	tlsConfig := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    ca,
	}

	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	pb.RegisterJobWorkerServiceServer(srv, jw)

	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Errorf("listen on %s; error: %v", addr, err)
		return ecListen
	}
	defer lis.Close()

	if err := srv.Serve(lis); err != nil {
		logger.Errorf("serve on %s; error: %v", addr, err)
		return ecServe
	}

	return ecSuccess
}
