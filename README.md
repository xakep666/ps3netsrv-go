# ps3netsrv-go

It's an alternative implementation of original [ps3netsrv](https://github.com/aldostools/webMAN-MOD/tree/master/_Projects_/ps3netsrv)
which needed to install games using WebMAN/IrisMAN over network (without copying files to console).

I made it because original code is way hard to read and hard to build for some platforms. And for fun and education
(understanding and implementation custom network protocols, generating/serving iso image on-the-fly) of course.

This project written in Go because it's (cross-)compilation is much easier than C/C++ and resulting binaries
will run without any external library on target system.

## Features 

### Unique ✳️

* Write protection. Enabled by default, use flag `--allow-write` or corresponding parameter.
* TCP data exchange timeouts / auto-close of idle connections: configured by `--read-timeout` parameter.
* [Compressed images](#compressed-images) - save your disk space without filesystem-level compression.

### Supported ✅

* Simple file transfer / directory listing
* File streaming including game images in `PS3ISO`
* PSX images streaming
* Client addresses whitelist, capping amount of connections
* Virtual ISO: games in directory format (residing in `GAMES`). Note: https://github.com/xakep666/ps3netsrv-go/issues/28
* 3k3y/Redump images: if iso path is `<root>/PS3ISO/game.iso` than dedicated key expected at `<root>/PS3ISO/game.dkey` or at `<root>/REDKEY/game.dkey`

### Unsupported ❌

* Multipart files `*.666xx`
* Subdir scanning if requested by WebMAN https://github.com/xakep666/ps3netsrv-go/issues/29
* PS2 Games, more tests/debugging needed https://github.com/xakep666/ps3netsrv-go/issues/31

## Compressed images
`ps3netsrv-go` supports compressed images to help save disk space. Currently only MAME CHD format is supported.

### MAME CHD
CHD is a format for space-efficient lossless compression that preserves ability to randomly access data without full decompression of whole file.
Originally developed as part of [MAME](https://www.mamedev.org/) but today used in many emulators: [ScePSX](https://github.com/unknowall/ScePSX), [Duckstation](https://www.duckstation.org/), [PCSX2](https://pcsx2.net/) and others. Space-efficiency is achieved by combining multiple comression algorithms to different data types (audio, data, ...) inside a raw disk image.

Powered by:
* [libchdr](https://github.com/rtissera/libchdr) - a C-library used to read and decompress CHD files
* [purego](https://github.com/ebitengine/purego) - a loader that allows to call functions in a dynamically-loaded shared libraries without CGO.
* [zig cc](https://andrewkelley.me/post/zig-cc-powerful-drop-in-replacement-gcc-clang.html) - C toolchain with fantastic cross-compilation abilities.

#### Usage
`libchdr` is required to be installed on the system. See [Installation](#libchdr) for more details how to do this.

Just put your `.chd` images into necessary directory under server root: `PSXISO`, `PS2ISO` or even `PS3ISO`. 
In case of successful `libchdr` loading you will see a following log message on server start:
```
Mar 23 00:00:00.000 INF libchdr loaded, enabling chd support
```
PS3 will see such images as `.chd.iso` - server intentionally adds `.iso` extension to help console properly detecting a file type.

Use [chdman](https://docs.mamedev.org/tools/chdman.html) tool maintained by MAME to compress your existing images.

#### Compatibility
* PS1 (PSX) images: *tested* and **working** ✅ (kudos to @turbosagat for assistance)
* PS2 images: *tested* and **not working** ❌ (to be investigated in https://github.com/xakep666/ps3netsrv-go/issues/31)
* PS3 images: *untested* ❔ (technically should work because uses same codebase as PS1 images)

#### Limitations
* `purego` does not work on some platforms supported by Go (i.e. `aix` and `ppc64`). However, they're pretty exotic nowdays and it's highly unlikely to see `ps3netsrv-go` running on them.
* `libchdr` does not support `AVHuff` compression codec: https://github.com/rtissera/libchdr/issues/69. However it's used mainly for laserdiscs so it's very unlikely to meet it in videogame images.
* Mixed CD/non-CD codecs (`cdlz` and `lzma`) and mixed CD modes (`MODE1`, `MODE1/RAW`, etc. in image metadata) are not supported. It's possible to create such image only by specifying `-c` option in `chdman` and probably such images are not supported by other emulators as well.

## Installation
This project shipped in a multiple ways for convenient installation:
* Docker images: [`docker pull ghcr.io/xakep666/ps3netsrv-go`](https://ghcr.io/xakep666/ps3netsrv-go). `amd64` and `arm64` images are available.
* Linux packages: deb, rpm and archlinux. See [Releases](https://github.com/xakep666/ps3netsrv-go/releases). 
If your distro is based on other package manager you may want to use a simple binary and a [systemd unit](./package/linux/ps3netsrv-go.service).
* Archived binaries are also available in Releases.

### libchdr
This libarary is required to enable CHD images support. It's included in a following release types:
* Docker: present in a container image, should work out of the box
* Release archive: contains compiled version of library except Windows/arm64 build.

`libchdr` is **not included** in Linux packages but declared as a dependancy. Most distros contain it in their repos.
If necessary, getting it compiled on Linux is pretty straightforward if you're familiar with `CMake`.

## Configuration
Server supports configuration via environment variables and command line flags.
Environment variables names can be found in output of `ps3netsrv-go server --help` command.
I.e. in line:
```
--root="."                             Root directory with games ($PS3NETSRV_ROOT).
```
`PS3NETSRV_ROOT` is environment variable name.

Also server supports configuration via config file. Example:
```ini
[server]
root = /home/user/games
client-whitelist = 192.168.1.0/24
max-clients = 10
allow-write = true
```
Configuration keys names are the same as command line flags names without `--` prefix.

Config file discovered in following order:
* `--config` flag or `PS3NETSRV_CONFIG_FILE` environment variable
* `config.ini` file in current directory
* `<user config directory>/ps3netsrv-go/config.ini`, where `<user config directory>` is OS-specific directory for user configuration files:
  * `%APPDATA%` on Windows
  * `$XDG_CONFIG_HOME` or `~/.config` on Linux
  * `~/Library/Application Support` on macOS

## Running
### Simple binary
Download necessary archive from Releases, unpack it and run
```bash
$ ps3netsrv-go server
```
from your working directory to serve it.

Or specify custom root directory in `--root` flag of `server` subcommand:
```bash
$ ps3netsrv-go server --root=/home/user/games
```

To get help run:

```bash
$ ps3netsrv-go --help
```

To run "debug" server (for pprof, etc.) specify `--debug-server-listen-addr` flag.

### Docker
Recommended way to serve your directory is:
```bash
$ docker run \
  -u $(id -u):$(id -g) \
  -v <data directory>:/srv/ps3data \
  -p 38008:38008 \
  ghcr.io/xakep666/ps3netsrv-go
```
But note that listen address displayed in logs is not an address you can connect to because it's container internal address.
In-container persistent volume is also available in `/srv/ps3data`.

### Systemd service
Deb, rpm and archlinux packages are shipped with systemd unit. Run
```bash
$ systemctl daemon-reload
$ systemctl enable ps3netsrv-go
```
to enable automatic startup.

Config file location is `/etc/ps3netsrv-go/config.ini`. Data location is `/srv/ps3data`. Service is running under separate user `ps3netsrv`.

### Note for non-glibc Linux distros users
Due to usage of `purego` all Linux executables in releases are dynamically linked ones. 
By default they're linked to run with `glibc` because it's most popular and widespread libc.
However, some distros like Alpine uses different libc (`musl` in case of Alpine).
If you try to run `ps3netsrv-go` executable from release directly on such distro, you'll get an error like
```
exec /path/to/ps3netsrv-go: no such file or directory
```
There are two ways to resolve this issue:
* Compile from source code for necessary libc. Recommended way. See [Building](#building) for more details.
* Run with loader: `/lib/ld-musl-<arch>.so.1 /path/to/ps3netsrv-go`. Downside: `libchdr` likely will not be loaded so CHD support will be disabled.

### Windows
To run as a service it's recommended to use [NSSM](https://nssm.cc/usage). It allows to specify user, startup args and environment variables. 

## Performance tips
* Connect your console to the network using ethernet cable. To achieve maximum performance server and console
should be connected with 1Gbps network.
* Use SSD or NVMe drive to store games. It will reduce loading times. 
* Use decrypted ISOs. It will reduce CPU usage and loading times. You can decrypt images using `decrypt` subcommand.
* Use "compiled" ISOs instead of folder with files. It will reduce loading times.
You can build ISO image using `makeiso` subcommand.

## Exposing tips
* Use limits:
    * strict root to prevent possible directory traversal outside provided root: `--strict-root` flag
    * by IP address(es) using `--client-whitelist` flag: `$ ps3netsrv-go server --root=/home/games --client-whitelist=192.168.0.123`
    * by number of clients using `--max-clients` flag
    * idle connection time: `--read-timeout` flag
* To expose over NAT (non-public or "grey" IP) you can use:
    * [ngrok](https://ngrok.com/docs/secure-tunnels/tunnels/tcp-tunnels/) TCP tunnels
    * [Reverse SSH tunnel](https://jfrog.com/connect/post/reverse-ssh-tunneling-from-start-to-end/) to host with public IP
    * any other options
* To secure connection using TLS you may use two TLS-terminators (like [HAProxy](https://www.haproxy.org/)) configured with mutual TLS authentication. Note that desired terminator must support "wrapping" plain TCP connection to TLS with client certificate. 

## Requirements to build
[Go 1.26+](https://go.dev/dl/)

## Building
```bash
$ go mod download
$ go build -o ps3netsrv-go ./cmd/ps3netsrv-go/...
```

> [!IMPORTANT]
> Some platforms require extra build flags to be compiled successfully due to `purego` usage. See [support notes](https://github.com/ebitengine/purego/blob/main/README.md#support-notes) for details.

If you're building for non-glibc Linux distro (like Alpine) or building on non-glibc distro for glibc-based distro (like on Alpine for Ubuntu) you need to properly specify `ldso` path via `GO_LDSO` environment variable:
* `GO_LDSO=/lib/ld-musl-x86_64.so.1` for Alpine on x86_64 architecture
* `GO_LDSO=/lib64/ld-linux-x86-64.so.2` for any glibc-based distro (Ubuntu/Debian/Arch/...) on x86_64 architecture
