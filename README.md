# teleport

## Problem Statement

Implement a prototype job worker service that provides an API to run arbitrary Linux processes.

#### Library

- Worker library with methods to start/stop/query status and get the output of a job.
- Library should be able to stream the output of a running job.
  - Output should be from start of process execution.
  - Multiple concurrent clients should be supported.
- Add resource control for CPU, Memory and Disk IO per job using cgroups.

#### API

- GRPC API to start/stop/get status/stream output of a running process.
- Use mTLS authentication and verify client certificate. Set up strong set of cipher suites for TLS and good crypto setup for certificates. Do not use any other authentication protocols on top of mTLS.
- Use a simple authorization scheme.

#### Client
- CLI should be able to connect to worker service and start, stop, get status, and stream output of a job.

## Design

The design of this system is outlined [here](https://github.com/tjper/teleport/blob/main/design.md).
