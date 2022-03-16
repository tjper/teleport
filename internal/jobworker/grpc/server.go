// Package grpc provides types that implement a jobworker grpc server.
package grpc

import (
	"context"
	"errors"
	"os"

	"github.com/tjper/teleport/internal/jobworker/cgroup"
	"github.com/tjper/teleport/internal/jobworker/job"
	"github.com/tjper/teleport/internal/jobworker/reexec"
	"github.com/tjper/teleport/internal/log"
	"github.com/tjper/teleport/internal/validator"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// logger is an object for logging package events to stdout.
var logger = log.New(os.Stdout, "grpc")

// NewJobWorker creates a JobWorker instance.
func NewJobWorker(jobSvc *job.Service, userSvc IUserService) *JobWorker {
	return &JobWorker{jobSvc: jobSvc, userSvc: userSvc}
}

var _ pb.JobWorkerServiceServer = (*JobWorker)(nil)

// IUserService provides an API for interacting with jobworker users.
type IUserService interface {
	// User retrieves the user associated with the ctx. The ok return value
	// should indicate if the user could be retrieved. The user return value
	// should be the user's unique identifer.
	User(ctx context.Context) (string, bool)
}

// Jobworker provides mechanisms for starting, stopping, fetching status, and
// output streaming jobs.
// Jobworker implements pb.JobWorkerServiceServer.
type JobWorker struct {
	jobSvc  *job.Service
	userSvc IUserService
}

func (jw JobWorker) Start(ctx context.Context, req *pb.StartRequest) (*pb.StartResponse, error) {
	user, ok := jw.userSvc.User(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	valid := validator.New()
	valid.AssertFunc(func() bool { return req.Command != nil }, "command empty")
	valid.Assert(req.Command.Name != "", "command name empty")
	valid.AssertFunc(func() bool { return req.Limits != nil }, "limits empty")
	if err := valid.Err(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	j, err := job.New(
		user,
		reexec.Command{
			Name: req.Command.Name,
			Args: req.Command.Args,
		},
	)
	if err != nil {
		logger.Errorf("building job; error: %s", err)
		return nil, status.Error(codes.Internal, "error building job")
	}

	if err := jw.jobSvc.StartJob(
		ctx,
		*j,
		cgroupOptions(req.Limits)...,
	); err != nil {
		logger.Errorf("starting job; error: %s", err)
		return nil, status.Error(codes.Internal, "error starting job")
	}

	return &pb.StartResponse{
		JobId:   j.ID.String(),
		Command: req.Command,
		Status: &pb.StatusDetail{
			Status:   toStatus(j.Status()),
			ExitCode: int32(j.ExitCode()),
		},
		Limits: req.Limits,
	}, nil
}

func (jw JobWorker) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	user, ok := jw.userSvc.User(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, validator.Format("empty job ID"))
	}

	j, err := jw.fetchJob(ctx, user, req.JobId)
	if err != nil {
		return nil, err
	}

	if j.Status() != job.Running {
		return nil, status.Error(codes.FailedPrecondition, "job is not running")
	}

	if err := jw.jobSvc.StopJob(ctx, j.ID); err != nil {
		logger.Errorf("stop job; job: %s, error: %v", j.ID, err)
		return nil, status.Error(codes.Internal, "error stopping job")
	}

	return &pb.StopResponse{}, nil
}

func (jw JobWorker) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	user, ok := jw.userSvc.User(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, validator.Format("empty job ID"))
	}

	j, err := jw.fetchJob(ctx, user, req.JobId)
	if err != nil {
		return nil, err
	}

	return &pb.StatusResponse{
		Status: &pb.StatusDetail{
			Status:   toStatus(j.Status()),
			ExitCode: int32(j.ExitCode()),
		},
	}, nil
}

func (jw JobWorker) Output(req *pb.OutputRequest, stream pb.JobWorkerService_OutputServer) error {
	user, ok := jw.userSvc.User(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}

	if req.JobId == "" {
		return status.Error(codes.InvalidArgument, validator.Format("empty job ID"))
	}

	j, err := jw.fetchJob(stream.Context(), user, req.JobId)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// TODO: to buffer or not to buffer (buffer)
	outputc := make(chan []byte, streamBuffer)
	go func() {
		if err := j.StreamOutput(ctx, outputc, chunkSize); err != nil {
			logger.Errorf("streaming output from job; job: %s, error: %v", j.ID, err)
		}
		close(outputc)
	}()

	for b := range outputc {
		if err := stream.Send(&pb.OutputResponse{Output: b}); err != nil {
			logger.Errorf("streaming output to client; job: %s, error: %s", j.ID, err)
			return err
		}
	}

	return nil
}

func (jw JobWorker) fetchJob(ctx context.Context, user string, jobID string) (*job.Job, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, validator.Format("job ID not UUID"))
	}

	j, err := jw.jobSvc.FetchJob(ctx, id)
	if errors.Is(err, job.ErrJobNotFound) {
		return nil, status.Error(codes.NotFound, "unknown job ID")
	}
	if err != nil {
		logger.Errorf("fetch job; job: %s, error: %v", id, err)
		return nil, status.Error(codes.Internal, "error fetching job")
	}

	if j.Owner != user {
		// Here we return codes.NotFound to prevent clients from determining that a
		// job IDs exists without having access to it.
		return nil, status.Error(codes.NotFound, "unknown job ID")
	}

	return j, nil
}

// cgroupOptions builds a slice of cgroup.CgroupOptions based on the limits.
func cgroupOptions(limits *pb.Limits) []cgroup.CgroupOption {
	var cgroups []cgroup.CgroupOption
	add := func(condition bool, option cgroup.CgroupOption) {
		if condition {
			cgroups = append(cgroups, option)
		}
	}

	add(limits.Memory > 0, cgroup.WithMemory(limits.Memory))
	add(limits.Cpus > 0, cgroup.WithCpus(limits.Cpus))
	add(limits.DiskReadBps > 0, cgroup.WithDiskReadBps(limits.DiskReadBps))
	add(limits.DiskWriteBps > 0, cgroup.WithDiskWriteBps(limits.DiskWriteBps))

	return cgroups
}

const (
	// streamBuffer is the default stream buffer size. This is the number of
	// chunks that may be held in memory.
	streamBuffer = 16

	// chunkSize is the size in bytes of each chunk to stream.
	chunkSize = 128
)
