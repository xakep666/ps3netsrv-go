FROM gcr.io/distroless/base-debian13:debug-nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ps3netsrv-go /bin/ps3netsrv-go
COPY --chown=nonroot:nonroot package/layout/ /srv/ps3data/
COPY thirdparty/libchdr/libchdr-${TARGETPLATFORM//\//-}/libchdr.so* /usr/lib
ENV PS3NETSRV_ROOT=/srv/ps3data \
    PS3NETSRV_STRICT_ROOT=true \
    PS3NETSRV_ALLOW_WRITE=true \
    PS3NETSRV_LISTEN_ADDR=0.0.0.0:38008
VOLUME /srv/ps3data
EXPOSE 38008
ENTRYPOINT ["/bin/ps3netsrv-go"]
CMD ["server"]
