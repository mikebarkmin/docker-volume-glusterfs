FROM golang:1.15 as builder
COPY . /go/src/github.com/mikebarkmin/docker-volume-glusterfs
WORKDIR /go/src/github.com/mikebarkmin/docker-volume-glusterfs
RUN go mod vendor
RUN go install --ldflags '-extldflags "-static"'
CMD ["/go/bin/docker-volume-glusterfs"]

FROM ubuntu:20.04
RUN apt-get update \
  && apt-get install software-properties-common -y
RUN add-apt-repository ppa:gluster/glusterfs-6 \ 
  && apt-get update \
  && apt-get install glusterfs-client -y \
  && rm -rf /var/lib/apt/lists/*
COPY --from=builder /go/bin/docker-volume-glusterfs .
CMD ["docker-volume-glusterfs"]

