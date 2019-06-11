package main

import (
  "encoding/json"
	"fmt"
  "io/ioutil"
	"log"
	"os"
  "os/exec"
  "path/filepath"
  "strconv"
	"strings"
  "sync"

  "github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const socketAddress = "/run/docker/plugins/gfs.sock"

type glusterfsVolume struct {
  Connections int
  Volume string
  Options []string
  Mountpoint string
}

type glusterfsDriver struct {
  sync.RWMutex
  servers []string

  root string
  statePath string
  volumes map[string]*glusterfsVolume
}

func newGlusterfsDriver(root string) (*glusterfsDriver, error) {
  logrus.WithField("method", "new driver").Debug(root)

  var servers []string
	if os.Getenv("SERVERS") != "" {
		servers = strings.Split(os.Getenv("SERVERS"), ",")
	}

  d := &glusterfsDriver{
    root: filepath.Join(root, "volumes")
    statePath: filepath.Join(root, "state", "gfs-state.json")
    volumes: map[string]*glusterfsVolume{},
    servers: servers
  }

  data, err := ioutil.ReadFile(d.statePath)
  if err != nil {
    if os.IsNotExist(err) {
      logrus.WithField("statePath", d.statePath).Debug("no state found")
    } else {
      return nil, err
    }
  } else {
    if err := json.Unmarshal(data, &d.volumes); err != nil {
      return nil, err
    }
  }

  return d, nil
}

func (d *glusterfsDriver) saveState() {
  data, err := json.Marshal(d.volumes)
  if err != nil {
    logrus.WithField("statePath", d.statePath).Error(err)
    return
  }

  if err := ioutil.WriteFile(d.statePath, data, 0644); err != nil {
    logrus.WithField("savestate", d.statePath).Error(err)
  }
}

func (d *glusterfsDriver) Create(r *volume.CreateRequest) error {
  logrus.WithField("method", "create").Debugf("%#v", r)

  d.Lock()
  defer d.Unlock()
  v := &glusterfsVolume{}

  for key, val := range r.Options {
    switch key {
    case "volume":
      v.Volume = val
    default:
      if val != "" {
        v.Options = append(v.Options, key+"="+val)
      } else {
        v.Options = append(v.Options, key)
      }
    }
  }

  if v.volume == "" {
    return logError("'volume' option required")
  }
  
  v.Mountpoint = filepath.Join(d.root, fmt.Sprintf("%x", md5.Sum([]byte(v.Volume))))

  d.volumes[r.Name] = v

  d.saveState()
  
  return nil
}

func (d *glusterfsDriver) Remove(r *volume.RemoveRequest) error {
  logrus.WithField("method", "remove").Debugf("%#v", r)

  d.Lock()
  defer d.Unlock()

  v, ok := d.volumes[r.Name]
  if !ok {
    return logError("volume %s not found", r.Name)
  }

  if v.connections != 0 {
    return logError("volume %s is currently used by a container", r.Name)
  }

  if err := os.RemoveAll(v.Mountpoint); err != nil {
    return logError(err.Error())
  }
  delete(d.volumes, r.Name)
  d.saveState()
  return nil
}

func (d *glusterfsDriver) Path(r *volume.PathRequestA) (*volume.PathResponse, error) {
  logrus.WithField("method", "path").Debugf("%#v", r)

  d.RLock()
  defer d.RUnlock()

  v, ok := d.volumes[r.Name]
  if !ok {
    return &volume.PathResponse{}, logError("volume %s not found", r.Name)
  }

  return &volume.PathResponse{Mountpoint: v.Mountpoint}, nil
}

func (d *glusterfsDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
  logrus.WithField("method", "mount").Debugf("%#v", r)

  d.Lock()
  defer d.Unlock()

  v, ok := d.volumes[r.Name]
  if !ok {
    return &volume.MountRequest{}, logError("volume %s not found", r.Name)
  }

  if v.connections == 0 {
    fi, err := os.Lstat(v.Mountpoint)
    if os.IsNotExist(err) {
      if err := os.MkdirAll(v.Mountpoint, 0755); err != nil {
        return &volume.MountResponse{}, logError(err.Error())
      }
    } else if err != nil {
      return &volume.MountResponse{}, logError(err.Error())
    }

    if fi != nil && !fi.IsDir() {
      return &volume.MountResponse{}, logError("%v already exist and it's not a directory", v.Mountpoint)
    }

    if err := d.mountVolume(v); err != nil {
      return &volume.MountResponse{}, logError(err.Error())
    }
  }

  v.connections++

  return &volume.MountResponse{Mountpoint: v.Mountpoint}, nil
}


func (p *gfsDriver) Validate(req *volume.CreateRequest) error {

	_, serversDefinedInOpts := req.Options["servers"]
	_, glusteroptsInOpts := req.Options["glusteropts"]

	if len(p.servers) > 0 && (serversDefinedInOpts || glusteroptsInOpts) {
		return fmt.Errorf("SERVERS is set, options are not allowed")
	}
	if serversDefinedInOpts && glusteroptsInOpts {
		return fmt.Errorf("servers is set, glusteropts are not allowed")
	}
	if len(p.servers) == 0 && !serversDefinedInOpts && !glusteroptsInOpts {
		return fmt.Errorf("One of SERVERS, driver_opts.servers or driver_opts.glusteropts must be specified")
	}

	return nil
}

func (p *gfsDriver) MountOptions(req *volume.CreateRequest) []string {

	servers, serversDefinedInOpts := req.Options["servers"]
	glusteropts, _ := req.Options["glusteropts"]

	var args []string

	if len(p.servers) > 0 {
		for _, server := range p.servers {
			args = append(args, "-s", server)
		}
		args = AppendVolumeOptionsByVolumeName(args, req.Name)
	} else if serversDefinedInOpts {
		for _, server := range strings.Split(servers, ",") {
			args = append(args, "-s", server)
		}
		args = AppendVolumeOptionsByVolumeName(args, req.Name)
	} else {
		args = strings.Split(glusteropts, " ")
	}

	return args
}

// AppendVolumeOptionsByVolumeName appends the command line arguments into the current argument list given the volume name
func AppendVolumeOptionsByVolumeName(args []string, volumeName string) []string {
	parts := strings.SplitN(volumeName, "/", 2)
	ret := append(args, "--volfile-id="+parts[0])
	if len(parts) == 2 {
		ret = append(ret, "--subdir-mount=/"+parts[1])
	}
	return ret
}

func main() {
  debug := os.Getenv("DEBUG")
  if ok, _ := strconv.ParseBool(debug); ok {
    logrus.SetLevel(logrus.DebugLevel)
  }

  d, err := newGlusterfsDriver("/mnt")
  if err != nil {
    log.Fatal(err)
  }

  h := volume.NewHandler(d)
  logrus.Infof("listening on %s", socketAddress)
  logrus.Error(h.ServeUnix(sockerAddress, 0))
}
