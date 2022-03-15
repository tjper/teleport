package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"

	"github.com/tjper/teleport/internal/encrypt"
	"github.com/tjper/teleport/internal/jobworker/cgroup"
	igrpc "github.com/tjper/teleport/internal/jobworker/grpc"
	"github.com/tjper/teleport/internal/jobworker/job"
	"github.com/tjper/teleport/internal/jobworker/user"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"
	"golang.org/x/sys/unix"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// runServe initialzes and configures a gprc.JobWorker instance to serve
// authenticated clients.
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

	// Register grpc.JobWorker instance as gRPC server.
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	pb.RegisterJobWorkerServiceServer(srv, jw)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Listen for SIGINT and SIGTERM to stop gRPC server.
	stopc := make(chan os.Signal, 1)
	signal.Notify(stopc, unix.SIGINT, unix.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
			return
		case signal := <-stopc:
			logger.Infof("signal received; signal: %s", signal.String())
			srv.GracefulStop()
		}
	}()

	addr := fmt.Sprintf(":%d", *portFlag)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Errorf("listen on %s; error: %v", addr, err)
		return ecListen
	}
	defer lis.Close()

	logger.Infof("jobworker API listening on %s", addr)
	if err := srv.Serve(lis); err != nil {
		logger.Errorf("serve on %s; error: %v", addr, err)
		return ecServe
	}

	return ecSuccess
}
