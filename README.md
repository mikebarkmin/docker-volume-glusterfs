# Docker volume plugin for GlusterFS

This is a managed Docker volume plugin to allow Docker containers to access
GlusterFS volumes.  The GlusterFS client does not need to be installed on the
host and everything is managed within the plugin.

[![TravisCI](https://travis-ci.org/mikebarkmin/docker-volume-glusterfs.svg)](https://travis-ci.org/mikebarkmin/docker-volume-glusterfs)
[![Go Report Card](https://goreportcard.com/badge/github.com/mikebarkmin/docker-volume-glusterfs)](https://goreportcard.com/report/github.com/mikebarkmin/docker-volume-glusterfs)

## Usage

1 - Install the plugin
```
docker plugin install mikebarkmin/glusterfs

# or to enable debug
docker plugin install mikebarkmin/glusterfs DEBUG=1
```

2 - Create a volume

> Make sure the ***gluster volume exists***.
>
> Or the mounting of the volume will fail.

```
$ docker volume create -d mikebarkmin/glusterfs -o servers=<server1,server2,...,serverN> -o volname=<volname> -o subdir=<subdir> glustervolume
glustervolume
$ docker volume ls
DRIVER                       VOLUME NAME
mikebarkmin/glusterfs:next   glustervolume
```

3 - Use the volume
```
$ docker run -it -v glustervolume:<path> bash ls <path>
```

## Options

* servers [required]: A comma-separated list of servers e.g.: 192.168.2.1,192.168.1.1
* volname [required]: The name of the glusterfs volume e.g.: gv0. Needs to be defined on the glusterfs cluster.
* subdir [required]: The name of the subdir. Will be created, if not found.

For additional options see [man mount.glusterfs](https://github.com/gluster/glusterfs/blob/release-6/doc/mount.glusterfs.8).

## TODO

* write integration tests

## LICENSE

MIT
