package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const socketAddress = "/run/docker/plugins/glusterfs.sock"

type glusterfsVolume struct {
	connections      int
	Subdir           string
	SubdirMountpoint string
	Servers          []string
	Volname          string
	Options          []string
	Mountpoint       string
}

type glusterfsDriver struct {
	sync.RWMutex

	root      string
	statePath string
	volumes   map[string]*glusterfsVolume
	defaultVolname string
	defaultServers string
}

func newGlusterfsDriver(root string, defaultServers string, defaultVolname string) (*glusterfsDriver, error) {
	logrus.WithField("method", "new driver").Debug(root)

	d := &glusterfsDriver{
		root:      filepath.Join(root, "volumes"),
		statePath: filepath.Join(root, "state", "gfs-state.json"),
		volumes:   map[string]*glusterfsVolume{},
		defaultVolname: defaultVolname,
		defaultServers: defaultServers,
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
	v := &glusterfsVolume{
		Subdir: r.Name,
		Volname: d.defaultVolname,
		Servers: strings.Split(d.defaultServers, ","),
	}

	for key, val := range r.Options {
		switch key {
		case "subdir":
			v.Subdir = val
			break
		case "volname":
			v.Volname = val
			break
		case "servers":
			v.Servers = strings.Split(val, ",")
		default:
			if val != "" {
				v.Options = append(v.Options, key+"="+val)
			} else {
				v.Options = append(v.Options, key)
			}
		}
	}

	if v.Subdir == "" {
		return logError("'subdir' option required")
	}

	if v.Volname == "" {
		return logError("'volname' option required")
	}

	if len(v.Servers) < 1 {
		return logError("'servers' option required")
	}

	v.Mountpoint = filepath.Join(d.root, fmt.Sprintf("%x/%x", md5.Sum([]byte(v.Volname)), md5.Sum([]byte(v.Subdir))))

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

func (d *glusterfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
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
		return &volume.MountResponse{}, logError("volume %s not found", r.Name)
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

	return &volume.MountResponse{Mountpoint: v.SubdirMountpoint}, nil
}

func (d *glusterfsDriver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	v, ok := d.volumes[r.Name]
	if !ok {
		return logError("volume %s not found", r.Name)
	}

	v.connections--

	if v.connections <= 0 {
		if err := d.unmountVolume(v.Mountpoint); err != nil {
			return logError(err.Error())
		}
		v.connections = 0
	}

	return nil
}

func (d *glusterfsDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return &volume.GetResponse{}, logError("volume %s not found", r.Name)
	}

	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.SubdirMountpoint}}, nil
}

func (d *glusterfsDriver) List() (*volume.ListResponse, error) {
	logrus.WithField("method", "list").Debugf("")

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.volumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.Mountpoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *glusterfsDriver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *glusterfsDriver) mountVolume(v *glusterfsVolume) error {
	cmd := exec.Command("mount", "-t", "glusterfs")

	for _, option := range v.Options {
		cmd.Args = append(cmd.Args, "-o", option)
	}

	var servers = strings.Join(v.Servers, ",")
	var path = fmt.Sprintf("/%s", v.Volname)
	cmd.Args = append(cmd.Args, fmt.Sprintf("%s:%s", servers, path), v.Mountpoint)

	logrus.Debug(cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return logError("glusterfs command execute failed: %v (%s)", err, output)
	}

	var subdir = filepath.Join(v.Mountpoint, v.Subdir)
	fi, err := os.Lstat(subdir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(subdir, 0755); err != nil {
			return logError(err.Error())
		}
	} else if err != nil {
		return logError(err.Error())
	}

	if fi != nil && !fi.IsDir() {
		return logError("subdir %v already exist and it's not a directory", subdir)
	}

	v.SubdirMountpoint = subdir

	return nil
}

func (d *glusterfsDriver) unmountVolume(target string) error {
	cmd := fmt.Sprintf("umount %s", target)
	logrus.Debug(cmd)
	return exec.Command("sh", "-c", cmd).Run()
}

func logError(format string, args ...interface{}) error {
	logrus.Errorf(format, args...)
	return fmt.Errorf(format, args...)
}

func main() {
	debug := os.Getenv("DEBUG")
	if ok, _ := strconv.ParseBool(debug); ok {
		logrus.SetLevel(logrus.DebugLevel)
	}

	d, err := newGlusterfsDriver("/mnt", os.Getenv("SERVERS"), os.Getenv("VOLNAME"))
	if err != nil {
		log.Fatal(err)
	}

	h := volume.NewHandler(d)
	u, _ := user.Lookup("root")
	gid, _ := strconv.Atoi(u.Gid)
	logrus.Infof("listening on %s", socketAddress)
	logrus.Error(h.ServeUnix(socketAddress, gid))
}
