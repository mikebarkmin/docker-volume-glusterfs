FROM ubuntu:18.04 as builder
COPY . /go/src/github.com/mikebarkmin/docker-volume-glusterfs
WORKDIR /go/src/github.com/mikebarkmin/docker-volume-glusterfs
ENV GOPATH=/go
RUN set -ex \
    && apt update && apt install -y gcc libc-dev golang-go \
    && go install --ldflags '-extldflags "-static"'
CMD ["/go/bin/docker-volume-glusterfs"]

FROM ubuntu:18.04
RUN apt update \
  && apt install software-properties-common -y
RUN add-apt-repository ppa:gluster/glusterfs-6 \ 
  && apt update \
  && apt install glusterfs-client -y \
  && rm -rf /var/lib/apt/lists/*
COPY --from=builder /go/bin/docker-volume-glusterfs .
CMD ["docker-volume-glusterfs"]

