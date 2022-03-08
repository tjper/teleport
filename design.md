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
  Memory         uint64
  Cpus           float32
  DeviceWriteBps uint64
  DeviceReadBps  uint64
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

##### Terms
- **parent**: jobworker application servicing gRPC requests
- **child**: jobworker application that has been executed by the **parent** in order to create a third process within the correct cgroup.
- **grandchild**: arbitrary command being run on host Linux system.

##### Starting

The starting of a `Job` will involve the following:
1. The **parent** process receives a request to start a command.
2. Two pipes are created for communication from the **parent** to the **child**.
  1. pipe for sending job details
  2. pipe for signaling when the **child** has been placed in a cgroup by the **parent**
3. The **parent** starts a goroutine and writes job details (including the command) on pipe 1.
4. Both pipes are passed to **child** process as `ExtraFiles` in the `os/exec Cmd`.
5. The **child** process is started with `os/exec Cmd.Start()`, executing `jobworker exec`.
6. The **parent** creates a cgroup.
7. The **parent** places the **child** in the cgroup created in step 6.
8. The **parent** signals to **child** that it has been placed in a cgroup by closing pipe 2.
9. The **parent** starts a goroutine calling `os/exec Cmd.Wait()`, waiting for the **child** to return in order to track `Job` status.
10. Send started job details back to client.

The **child** process executing `jobworker exec` will involve the following:
1. Application starts, and the following file descriptors are opened:
  - `/proc/self/fd/3` for the job details
  - `/proc/self/fd/4` for the cgroup placement signaling
2. Job details are read from from `/proc/self/fd/3` and used to create `os/exec Cmd` for the **grandchild** process.
3. **child** waits for **parent** to signal that it has been placed in the correct cgroup on `/proc/self/fd/4`.
4. **child** calls `os/exec Cmd.Start()` to start **grandchild**.
5. **child** calls `os/exec Cmd.Wait()` to wait for **grandchild** to exit.
6. **child** returns **grandchild's** exit code as its own.

##### Stopping

In order to stop a command, the following will be done:
1. When the **parent** creates the **child** `os/exec Cmd` the **child** will have its own`pgid`.
2. When the **child** creates the **grandchild** `os/exec Cmd` the `pgid` will be in the same `pgid` as the **child**.
3. When the **parent** creates the **child** `os/exec Cmd` a context cancellation function will be stored as part of the `Job`. Any process seeking to stop a `Job` may call the cancellation function. This will kill both the **child** and **grandchild**.
4. The Cgroup related to `Job` will also be removed in unrelated logic.

##### Status

The status of underlying **grandchild** will be returned through the **child** to **parent** as exitcodes. This status will be handling in step 9 of [Starting](#starting).

###### JobStatus

The JobStatus type indicates the status of a job. The status will be one of the following:

- *running*: Job has been started and is currently running.
- *stopped*: Job has been forcibly stopped by jobworker.
- *exited*: Job has exited. (with exit code)

##### Streaming Output

Streaming output will involve reading a log file from `/var/log/jobworker` in chunks and writing it to the client until the client ends the stream.

###### Implementation

When an output stream is requested, the following will occur:
1. `Job` will be fetched by `job.Service`
2. `Job.OutputStream` will be called in its own goroutine and accept `context.Context` and `chan<- byte` arguments.
3. `Job.OutputStream` will open the log file, and defer closing the fd.
4. `Job.OutputStream` will start a goroutine listening on `<-ctx.Done()`.
5. `Job.OutputStream` will read from log file, in chunks.
6. When `Read` returns `io.EOF`, the process will sleep for a second and continue reading. See [Streaming Tradoffs](#streaming-tradeoffs) below.
7. If the `context.Context` is cancelled, `job.OutputStream` will close the `io.ReaderCloser`, close the `chan<- byte`, and return.
8. Meanwhile, `grpc.Service.Output()` is streaming these log chunks to a client.

###### Streaming Tradeoffs

Step 6 specifies how to handle `io.EOF`, and I don't really like the approach of polling the file to see if more data is available. However, I don't want to invest effort in setting up a filewatcher for this challenge if it's not necessary.

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
- mounting when mount has already been performed
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
- job stopping properly kills all related processes
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
 `jobworker-cli start` Start a job. Job details are returned.

Usage:
  jobworker-cli [global flags] start [flags] command [args...]

Flags:
  --memory           maximum amount of memory a job can use in bytes
  --cpus             how much of the available CPU resources a job can use
  --disk-write-bps   limit write rate (bytes per second) to disk
  --disk-read-bps    limit read rate (bytes per second) from disk

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

```
