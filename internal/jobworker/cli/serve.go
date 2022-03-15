package cli

import (
	"context"
	"fmt"
	"net"

	"github.com/tjper/teleport/internal/encrypt"
	"github.com/tjper/teleport/internal/jobworker/cgroup"
	igrpc "github.com/tjper/teleport/internal/jobworker/grpc"
	"github.com/tjper/teleport/internal/jobworker/job"
	"github.com/tjper/teleport/internal/jobworker/user"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func runServe(ctx context.Context) int {
	switch {
	case len(*keyFlag) == 0:
		help("Option -key is required for the serve subcommand.")
		return ecUnrecognized
	case len(*certFlag) == 0:
		help("Option -cert is required for the serve subcommand.")
		return ecUnrecognized
	case len(*caCertFlag) == 0:
		help("Option -ca_cert is required for the serve subcommand.")
		return ecUnrecognized
	}

	cgroupSvc, err := cgroup.NewService()
	if err != nil {
		logger.Errorf("cgroup service setup; error: %v", err)
		return ecCgroupService
	}
	defer func() {
		if err := cgroupSvc.Cleanup(); err != nil {
			logger.Errorf("cgroup service cleanup; error: %v", err)
		}
	}()

	jobSvc, err := job.NewService(cgroupSvc)
	if err != nil {
		logger.Errorf("job service setup; error: %v", err)
		return ecJobService
	}
	defer func() {
		if err := jobSvc.Close(); err != nil {
			logger.Errorf("job service closing; error: %v", err)
		}
	}()

	userSvc := user.Service{}
	jw := igrpc.NewJobWorker(jobSvc, userSvc)

	tlsConfig, err := encrypt.NewServermTLSConfig(*certFlag, *keyFlag, *caCertFlag)
	if err != nil {
		logger.Errorf("setup mTLS config; error: %v", err)
		return ecTLSConfig
	}

	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	pb.RegisterJobWorkerServiceServer(srv, jw)

	addr := fmt.Sprintf(":%d", *portFlag)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Errorf("listen on %s; error: %v", addr, err)
		return ecListen
	}
	defer lis.Close()

	// TODO: clean server shutdown
	if err := srv.Serve(lis); err != nil {
		logger.Errorf("serve on %s; error: %v", addr, err)
		return ecServe
	}

	return ecSuccess
}
