// Package grpc provides types that implement a jobworker grpc server.
package grpc

import (
	"context"
	"errors"
	"os"

	"github.com/google/uuid"
	"github.com/tjper/teleport/internal/jobworker/job"
	"github.com/tjper/teleport/internal/jobworker/reexec"
	"github.com/tjper/teleport/internal/log"
	"github.com/tjper/teleport/internal/validator"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// logger is an object for logging package events to stdout.
var logger = log.New(os.Stdout, "grpc")

// NewJobWorker creates a JobWorker instance.
func NewJobWorker(jobSvc *job.Service) *JobWorker {
	return &JobWorker{jobSvc: jobSvc}
}

var _ pb.JobWorkerServiceServer = (*JobWorker)(nil)

// Jobworker provides mechanisms for starting, stopping, fetching status, and
// output streaming jobs.
// Jobworker implements pb.JobWorkerServiceServer.
type JobWorker struct {
	jobSvc *job.Service
}

func (jw JobWorker) Start(ctx context.Context, req *pb.StartRequest) (*pb.StartResponse, error) {
	// TODO: ensure nil req.Command.Args are not being referenced.
	valid := validator.New()
	valid.AssertFunc(func() bool { return req.Command != nil }, "command empty")
	valid.Assert(req.Command.Name != "", "command name empty")
	valid.AssertFunc(func() bool { return req.Limits != nil }, "limits empty")
	if err := valid.Err(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	j, err := job.New(
		"owner",
		reexec.Command{
			Name: req.GetCommand().Name,
			Args: req.Command.Args,
		},
	)
	if err != nil {
		logger.Errorf("building job; error: %s", err)
		return nil, status.Error(codes.Internal, "build job")
	}

	if err := jw.jobSvc.StartJob(ctx, *j); err != nil {
		logger.Errorf("starting job; error: %s", err)
		return nil, status.Errorf(codes.Internal, "start job")
	}

	return &pb.StartResponse{
		JobId:   j.ID.String(),
		Command: req.Command,
		Status: &pb.StatusDetail{
			Status:   toStatus(j.Status()),
			ExitCode: uint32(j.ExitCode()),
		},
		Limits: req.Limits,
	}, nil
}

func (jw JobWorker) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	if req.JobId != "" {
		return nil, status.Error(codes.InvalidArgument, validator.Format("empty job ID"))
	}

	j, err := jw.fetchJob(ctx, req.JobId)
	if err != nil {
		return nil, err
	}

	if j.Status() != job.Running {
		return nil, status.Error(codes.FailedPrecondition, "job is not running")
	}

	if err := jw.jobSvc.StopJob(ctx, j.ID); err != nil {
		return nil, status.Error(codes.Internal, "stop job")
	}

	return &pb.StopResponse{}, nil
}

func (jw JobWorker) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	if req.JobId != "" {
		return nil, status.Error(codes.InvalidArgument, validator.Format("empty job ID"))
	}

	j, err := jw.fetchJob(ctx, req.JobId)
	if err != nil {
		return nil, err
	}

	return &pb.StatusResponse{
		Status: &pb.StatusDetail{
			Status:   toStatus(j.Status()),
			ExitCode: uint32(j.ExitCode()),
		},
	}, nil
}

func (jw JobWorker) Output(req *pb.OutputRequest, stream pb.JobWorkerService_OutputServer) error {
	if req.JobId != "" {
		return status.Error(codes.InvalidArgument, validator.Format("empty job ID"))
	}

	j, err := jw.fetchJob(stream.Context(), req.JobId)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// TODO: to buffer or not to buffer (buffer)
	outputc := make(chan []byte)
	go func() {
		if err := j.StreamOutput(ctx, outputc); err != nil {
			logger.Errorf("streaming output from job; error: %s", err)
		}
		close(outputc)
	}()

	for b := range outputc {
		if err := stream.Send(&pb.OutputResponse{Output: b}); err != nil {
			logger.Errorf("streaming output to client; error: %s", err)
			return err
		}
	}

	return nil
}

func (jw JobWorker) fetchJob(ctx context.Context, jobID string) (*job.Job, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, validator.Format("job ID not UUID"))
	}

	j, err := jw.jobSvc.FetchJob(ctx, id)
	if errors.Is(err, job.ErrJobNotFound) {
		return nil, status.Error(codes.NotFound, "unknown job ID")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "fetch job")
	}

	// TODO: retrieve owner from auth context
	if j.Owner != "owner" {
		return nil, status.Error(codes.PermissionDenied, "incorrect owner")
	}

	return j, nil
}