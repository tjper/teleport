# Design

This document outlines the design of the systems within this repo.

## Design Approach

There's two main aspects to my approach I tried to maintain within each package.
1. Have critical functionality hang off a `Service` type. This allows me to minimize global state, create a recognizable pattern for readers, and utilize interfaces if needed.
2. Always take the simplest approach. It was made clear in the challenge description a high performing nor a scalable system was being asked for. Therefore, I designed components in a manner I know will not scale in order to keep things simple. More on this in [Performance Concerns](#performance-concerns).


## API

Click [here](https://github.com/tjper/teleport/blob/design/proto/jobworker/v1/service_api.proto) for API definition.

## Authentication

Client and server will utilize mTLS.

TLS 1.3 will be used to ensure a secure cipher suite is utilized. As of Go 1.17, the cipher suites are ordered by `crypto/tls` based on range of factors. https://go.dev/blog/tls-cipher-suites

A set of certificates and secrets will exist in the `certs/` directory. All certificates will be signed by the included CA.

## Authorization

Once a client establishes a connection over TLS, the client certificate's `Common Name` will be used to determine which user has connected. A user will only be authorized to interact with jobs they started. All other jobs will be inaccessible to the user's client.

## Critical Libraries

### cgroups

Package cgroups will provide mechanisms to setup, cleanup, and interact with Linux cgroups.

The `Service` type will be responsible for the following:
- mounting/unmounting cgroup2
- directory `/cgroup2/jobworker` setup/cleanup
- creating/removing a cgroup
- updating cgroup controllers
- placing a process into a cgroup

#### Types

```
type Service struct {
  ID uuid.UUID
}

func (s Service) Cleanup() error
func (s Service) CreateCgroup(options ...CgroupOption) (*Cgroup, error)
func (s Service) RemoveCgroup(id uuid.UUID) error
func (s Service) PlaceInCgroup(cgroup Cgroup, pid int) error

type Cgroup struct {
  ID             uuid.UUID
  Memory         string
  Cpus           float32
  DeviceWriteBps string
  DeviceReadBps  string
}

type CgroupOption func(*Cgroup)

func WithMemory(limit string) CgroupOption
func WithCPUs(cpus float32) CgroupOption
func WithDeviceWriteBps(deviceLimit string) CgroupOption
func WithDeviceReadBps(deviceLimit string) CgroupOption
...
```

---

### job

Package job will provide a `Service` type for managing `Job` instances.

The `Service` type will be responsible for the following.
- Setup and cleanup of `var/log/jobworker` logging directory.
- Maintaining a concurrent-safe jobs map with `Job.ID` as the key and `Job` as the value.
- Provide methods to start, stop, and fetch managed `Job` instances

The `Job` type will be responsible for the following.
- Provide unexported mechanisms to start and stop arbitrary commands.
- Provide exported mechanisms to stream output to multiple clients concurrently.
- Ensure thread safe access to attributes where necessary.
- Ensure status reflects command state.

##### Start, Stop, and Status

The starting, stopping and monitoring of a `Job` instances command will be done mostly by utilizing the `os/exec` package.

In order to track command status, start (`os/exec.Cmd.Start()`) will be called and a goroutine will be launched to wait (`os/exec.Cmd.Wait()`) for command completion and record exit status.

##### Streaming Output

Streaming output will involve reading a log file from `/var/log/jobworker` in chunks and writing it to the client until EOF is reached. If the client uses the `--tail` flag, the output will be streamed until the client ends the stream.

##### JobStatus

The JobStatus type indicates the status of a job. The status will be one of the following:

- *running*: Job has been started and is currently running.
- *stopped*: Job has been forcibly stopped by jobworker.
- *exited*: Job has exited (will include exit code).

#### Types

```
type Service struct {
  ID            uuid.UUID
}

func (s Service) StartJob(job Job) error
func (s Service) StopJob(job Job) error
func (s Service) FetchJob(jobID uuid.UUID) (*Job, error)

type Job struct {
  ID      uuid.UUID
  command *exec.Cmd
  status  JobStatus
  output  io.ReadWriterCloser

  user string
  cgroupID uuid.UUID

// Locking mechanisms not included for brevity.
// ...
}

func (j Job) Output() []byte
func (j Job) StreamOutput(ctx context.Context) (<-chan byte, error)
```
---

### grpc

Package grpc will provide a `JobWorker` type that implements the gRPC server stub.

The `JobWorker` type will be responsible for the following.
- Ensuring all requests have the necessary permissions to be carried out.
- Utilize `job.Service` and `cgroups.Service` in coordination to satisfy requests.
- Validate request inputs.

#### Types

```
type JobWorker struct {
  authService    auth.Service
  jobService     job.Service
  cgroupsService cgroups.Service
}

func (jw JobWorker) Start(ctx context.Context, r *pb.StartRequest) (*pb.StartResponse, error)
func (jw JobWorker) Stop(ctx context.Context, r *pb.StopRequest) (*pb.StopResponse, error)
func (jw JobWorker) Status(ctx context.Context, r *pb.StartRequest) (*pb.StatusResponse, error)
func (jw JobWorker) Output(r *pb.OutputRequest, s pb.JobWokerService_OutputServer) error
```

## Performance Concerns

If I was going to scale this application, the first component I would revisit is output streaming. In-memory streaming that backed up to disk at some threshold could be considered.

## Testing

I wanted to highlight the areas of the application that are critical to test thoroughly in order to ensure correct function. I will likely not test everything listed as 100% test coverage is not necessary for this challenge.

#### Cgroups

Cgroups have a number of sharp edges and nuances to them. Tests will ensure the following:
- mounting/unmounting
- mounting when mount has alread been performed
- setup/cleanup
- setup when `/cgroup2/jobworker` already exists
- memory, cpus, and io controls are being applied and are reflected in process stats

#### Authorization

Authorization tests will be fairly simple. They will ensure clients are only able to access jobs they own.

#### Authentication

Authentication tests will ensure the following:
- client must be using TLS 1.3
- client certificate must be signed by the CA
- client that does not authenticate is unable to connect

#### Jobs

Job tests will ensure the following:
- logging setup/cleanup
- logging setup when `/var/log/jobworker` already exists
- job starting and supporting processes result in commands being processed and correct status tracking
- streaming output results are identical and correct across multiple concurrent clients

## CLI

```
A simple interface for interacting with a jobworker server.

Usage:
  jobworker-cli [global flags] command [command flags]

Available Commands:
  help        Help about any command
  start       Start job
  stop        Stop a job
  status      Fetch a job's status
  output      Fetch a job's output

Global Flags:
  --url       URL of jobworker server
  --cert      client x509 certificate
  --key       client private key
  --ca        certificate authority shared by server and client

---

Command:
 `jobworker-cli start` Start a job.

Usage:
  jobworker-cli [global flags] start [flags] command...

Flags:
  --memory           maximum amount of memory a job can use
  --cpus             how much of the available CPU resources a job can use
  --device-write-bps limit write rate (bytes per second) to a device
  --device-read-bps  limit read rate (bytes per second) from a device

---

Command:
`jobworker-cli stop` Stop a job.

Usage:
  jobworker-cli [global flags] stop job_id

---

Command:
`jobworker-cli status` Fetch job status.

Usage:
  jobworker-cli [global flags] status job_id

---

Command:
`jobworker-cli status` Fetch job output.

Usage:
  jobworker-cli [global flags] output [flags] job_id

Flags:
  --tail    tail output of job
```
