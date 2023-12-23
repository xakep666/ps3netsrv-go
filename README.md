# ps3netsrv-go

It's a minimal alternative implementation of original [ps3netsrv](https://github.com/aldostools/webMAN-MOD/tree/master/_Projects_/ps3netsrv)
which needed to install games using WebMAN/IrisMAN over network (without copying files to console).

I made it because original code is way hard to read and hard to build for some platforms. And for fun and education
(understanding and implementation custom network protocols, generating/serving iso image on-the-fly) of course.

This project written in Go because it's (cross-)compilation is much easier than C/C++ and resulting binaries
will run without any external library on target system.

Currently multipart files are not supported. But I've added tcp data exchange timeouts to reduce amount of "hang" connections.

Receiving files from console is supported now! Use flag `--allow-write` to enable this.

Decryption of 3k3y/redump images on-the-fly is supported now! Keys search behaviour completely matches with original `ps3netsrv`:
at first we try to find `.dkey` file for `.iso` in `PS3ISO` directory. Then we try to find `.dkey` in `REDKEY` directory.
You can also use 
```bash
$ ps3netsrv-go decrypt
```
tool to decrypt images.

## Running
Just run
```bash
$ ps3netsrv-go server
```
from your working directory to serve it.

Or specify custom root directory in 1st positional argument of `server` subcommand:
```bash
$ ps3netsrv-go server /home/user/games
```

To get help run:

```bash
$ ps3netsrv-go --help
```

To run "debug" server (for pprof, etc.) specify `--debug-server-listen-addr` flag.

## Configuration
Server supports configuration via environment variables and command line flags.
Environment variables names can be found in output of `ps3netsrv-go server --help` command.
I.e. in line:
```
Arguments:
  [<root>]    Root directory with games ($PS3NETSRV_ROOT).
```
`PS3NETSRV_ROOT` is environment variable name.

## Exposing tips
* Use limits:
    * by IP address(es) using `--client-whitelist` flag: `$ ps3netsrv-go server /home/games --client-whitelist=192.168.0.123`
    * by number of clients using `--max-clients` flag
    * idle connection time: `--read-timeout` flag
* To expose over NAT (non-public or "grey" IP) you can use:
    * [ngrok](https://ngrok.com/docs/secure-tunnels/tunnels/tcp-tunnels/) TCP tunnels
    * [Reverse SSH tunnel](https://jfrog.com/connect/post/reverse-ssh-tunneling-from-start-to-end/) to host with public IP
    * any other options
* To secure connection using TLS you may use two TLS-terminators (like [HAProxy](https://www.haproxy.org/)) configured with mutual TLS authentication. Note that desired terminator must support "wrapping" plain TCP connection to TLS with client certificate. 

## Requirements to build
[Go 1.21+](https://go.dev/dl/)

## Building
```bash
$ go mod download
$ go build -o ps3netsrv-go ./cmd/ps3netsrv-go/...
```
