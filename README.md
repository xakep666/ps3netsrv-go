# ps3netsrv-go

It's a minimal alternative implementation of original [ps3netsrv](https://github.com/aldostools/webMAN-MOD/tree/master/_Projects_/ps3netsrv)
which needed to install games using WebMAN/IrisMAN over network (without copying files to console).

I made it because original code is way hard to read and hard to build for some platforms. And for fun and education
(understanding and implementation custom network protocols, generating/serving iso image on-the-fly) of course.

This project written in Go because it's (cross-)compilation is much easier than C/C++ and resulting binaries
will run without any external library on target system.

Currently this project only support serving simple files or plain game directories (without encrypted 3k3yredump images and multipart files).
Copying files from console to server not supported now. But I've added tcp data exchange timeouts to reduce amount of "hang" connections.

## Running
To get help run:

```bash
$ ps3netsrv-go --help
```

To specify custom root directory use `--root` flag.

To run "debug" server (for pprof, etc.) specify `--debug-server-listen-addr` flag.

## Requirements to build
[Go 1.15+](https://golang.org/dl/)

## Building
```bash
$ go mod download
$ go build -o ps3netsrv-go ./cmd/ps3netsrv-go/...
```
