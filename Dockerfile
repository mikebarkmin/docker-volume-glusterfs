FROM golang:1.10-alpine as builder
COPY . /go/src/github.com/mikebarkmin/docker-volume-glusterfs
WORKDIR /go/src/github.com/mikebarkmin/docker-volume-glusterfs
RUN set -ex \
    && apk add --no-cache --virtual .build-deps \
    gcc libc-dev \
    && go install --ldflags '-extldflags "-static"' \
    && apk del .build-deps
CMD ["/go/bin/docker-volume-glusterfs"]

FROM ubuntu:18.04
RUN apt update && apt install software-properties-common -y
RUN add-apt-repository ppa:gluster/glusterfs-6 && apt update && apt install glusterfs-client -y
COPY --from=builder /go/bin/docker-volume-glusterfs .
CMD ["docker-volume-glusterfs"]

