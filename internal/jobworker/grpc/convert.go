package grpc

import (
	"github.com/tjper/teleport/internal/jobworker/job"
	pb "github.com/tjper/teleport/proto/gen/go/jobworker/v1"
)

func toStatus(s job.Status) pb.Status {
	switch s {
	case job.Running:
		return pb.Status_STATUS_RUNNING
	case job.Stopped:
		return pb.Status_STATUS_STOPPED
	case job.Exited:
		return pb.Status_STATUS_EXITED
	default:
		return pb.Status_STATUS_UNSPECIFIED
	}
}
