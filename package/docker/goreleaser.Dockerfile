FROM alpine:3
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ps3netsrv-go /bin/ps3netsrv-go
ENTRYPOINT ["/bin/ps3netsrv-go"]
