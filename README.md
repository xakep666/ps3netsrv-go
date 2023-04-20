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
Just run
```bash
$ ps3netsrv-go
```
from your working directory to serve it.

Or specify custom root directory in 1st positional argument:
```bash
$ ps3netsrv-go /home/user/games
```

To get help run:

```bash
$ ps3netsrv-go --help
```

To run "debug" server (for pprof, etc.) specify `--debug-server-listen-addr` flag.

## Exposing tips
* Use limits:
    * by IP address(es) using 2nd positinal argument: `$ ps3netsrv-go /home/games 192.168.0.123`
    * by number of clients using `--max-clients` flag
    * idle connection time: `--read-timeout` flag
    * mitigate againts slow clients: `--write-timeout` flag
* To expose over NAT (non-public or "grey" IP) you can use:
    * [ngrok](https://ngrok.com/docs/secure-tunnels/tunnels/tcp-tunnels/) TCP tunnels
    * [Reverse SSH tunnel](https://jfrog.com/connect/post/reverse-ssh-tunneling-from-start-to-end/) to host with public IP
    * any other options
* To secure connection using TLS you may use two TLS-terminators (like [HAProxy](https://www.haproxy.org/)) configured with mutual TLS authentication. Note that desired terminator must support "wrapping" plain TCP connection to TLS with client certificate. 

## Requirements to build
[Go 1.20+](https://go.dev/dl/)

## Building
```bash
$ go mod download
$ go build -o ps3netsrv-go ./cmd/ps3netsrv-go/...
```
