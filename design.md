# Design

This document outlines the design of the systems within this repo.

## Terms

- *jobworker*: Application responsible for managing the execution of arbitrary commands on the host Linux system.
- *command*: An arbitrary command started by the *jobworker*.
- *output*: The data typically written to stdout and stderr by a *command*.
- *log directory*: Location of *jobworker* *command* *output*, /var/log/jobworker.

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
  ID uuid.UUID
}

type CgroupOption func(*Cgroup)

func WithMemoryHigh(high string) CgroupOption
func WithCPUMax(max int) CgroupOption
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
- Ensure thread safe access to attributes where necessary.
- Ensure status reflects *command* state.
- Provide unexported mechanisms to start and stop *command*.
- Provide exported mechanisms to stream *output* to multiple clients concurrently.

##### Start, Stop, and Status

The starting, stopping and monitoring of a `Job` instances *command* will be done mostly by utilizing the `os/exec` package.

In order to track *command* status, start (`os/exec.Cmd.Start()`) will be called and a goroutine will be launched to wait (`os/exec.Cmd.Wait()`) for *command* completion and record exit status.

##### Streaming Output

Streaming output will involve reading a log file from `/var/log/jobworker` in chunks and writing it to the client until the client stops tailing the output.

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
func (s Service) StopJob(jobID uuid.UUID) error
func (s Service) FetchJob(jobID uuid.UUID) (*Job, error)

type Job struct {
  ID      uuid.UUID
  command *exec.Cmd
  status  JobStatus
  output  io.ReadWriterCloser

// Locking mechanisms not included for brevity.
// ...
}

func (j Job) Output() []byte
func (j Job) StreamOutput(ctx context.Context) (<-chan byte, error)
```


### grpc

Package grpc will provide a `JobWorker` type that implements the gRPC server stub.

The `JobWorker` type will be responsible for the following.
- Ensuring all requests have the necessary permissions to be carried out.
- Utilize `job.Service` and `cgroups.Service` in coordination to satisfy requests.
- Validate request inputs.

##### Types

```
type JobWorker struct {
  authService    auth.Service
  jobService     job.Service
  cgroupsService cgroups.Service
}

func (jw JobWorker) Start(ctx context.Context, r *pb.StartRequest) (*pb.StartResponse, error)
func (jw JobWorker) Stop(ctx context.Context, r *pb.StopRequest) (*pb.StopResponse, error)
func (jw JobWorker) Status(ctx context.Context, r *pb.StartRequest) (*pb.StatusResponse, error)
func (jw JobWorker) Output(ctx context.Context, r *pb.OutputRequest) (*pb.OutputResponse, error)
```

## API

Click [here](https://github.com/tjper/teleport/blob/main/proto/jobworker/v1/service_api.proto) for API definition.

## Authentication

Client and server will utilize mTLS.

TLS 1.3 will be used to ensure a secure cipher suite is utilized. As of Go 1.17, the cipher suites are ordered by `crypto/tls` based on range of factors. https://go.dev/blog/tls-cipher-suites

A set of certificates and secrets will exist in the `certs/` directory. All certificates will be signed by the included CA.

- ca.crt              CA certificate
- jobworker.key       jobworker server private key
- jobworker.crt       jobworker certificate
- user_alpha.key      user_alpha private key
- user_alpha.crt      user_alpha client certificate
- user_bravo.key      user_bravo private key
- user_bravo.crt      user_bravo client certificate

**user_alpha** will have permission to *mutate* and *query*
**user_bravo** will have permission to *query*

## Authorization

Once a client establishes a connection over TLS, the client certificate's `Common Name` will be used to determine which user has connected. An internal map will specify which roles each user has, and will be used to determine if the request should be processed.

All roles are specified below:
- *mutate*: allows a job to be started or stopped
- *query*: allows job status to be retrieved or job output to be streamed

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


Command:
 `jobworker-cli start` Start a job.

Usage:
  jobworker-cli [global flags] start [flags] command...

Flags:
  --memory-min
  --memory-low
  --memory-high
  --memory-max
  --cpu-weight
  --cpu-max
  --cpu-burst-max
  --io-weight
  --io-max-rbps
  --io-max-wbps
  --io-max-riops
  --io-max-wiops

Command:
`jobworker-cli stop` Stop a job.

Usage:
  jobworker-cli [global flags] stop job_id

Command:
`jobworker-cli status` Fetch job status.

Usage:
  jobworker-cli [global flags] status job_id

Command:
`jobworker-cli status` Fetch job output.

Usage:
  jobworker-cli [global flags] output [flags] job_id

Flags:
  --tail    tail output of job
