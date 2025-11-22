#!/usr/bin/env bash

set -euo pipefail

id -u ps3netsrv &>/dev/null || useradd \
    -c "PS3 Netserver user" \
    -d /srv/ps3data -M \
    -s $(which nologin) \
    -U \
    ps3netsrv
