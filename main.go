package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/go-nfs/nfsv3/nfs"
	"github.com/go-nfs/nfsv3/nfs/rpc"
	log "github.com/sirupsen/logrus"
)

const NFS_PATH_ENV_NAME = "NFS_PATH"
const DIR_PERM = "DIR_PERM"

// const NFS_VERSION = "NFS_VERSION"

//	func getNfsVersion() uint {
//		nfsVersion := os.Getenv(NFS_VERSION)
//		version, err := strconv.ParseUint(nfsVersion, 8, 32)
//		if err != nil {
//			return 3 // 默认权限
//		}
//		return uint(version)
//	}
func getDirPerm() os.FileMode {
	dirPerm := os.Getenv(DIR_PERM)
	perm, err := strconv.ParseUint(dirPerm, 8, 32)
	if err != nil {
		return 0775 // 默认权限
	}
	return os.FileMode(perm)
}
func markDir(path string) {
	dirPerm := getDirPerm()
	err := os.MkdirAll(path, dirPerm)
	if err != nil {
		log.Errorf("local mkdir: %s, Error: %v", path, err)
	} else {
		log.Infof("local mkdir: %s", path)
	}
}

func MkNFS3Dirs(v *nfs.Target, dir string) {
	f, _, err := v.Lookup(dir)
	if err == nil && f.IsDir() {
		// already exists
		return
	}
	dirPerm := getDirPerm()
	if _, err := v.Mkdir(dir, dirPerm); err != nil {
		if err.Error() == "file does not exist" {
			MkNFS3Dirs(v, filepath.Dir(dir))
			v.Mkdir(dir, dirPerm)
		} else {
			log.Errorf("nfs mkdir: %s, Error: %v", dir, err)
			return
		}
	}
	log.Infof("nfs mkdir: %s", dir)
}

func markNfsDir(cli *client.Client, volumeId string) {
	volumeData, _ := cli.VolumeInspect(context.Background(), volumeId)
	if volumeData.Driver == "local" && volumeData.Options["type"] == "nfs" {
		device := volumeData.Options["device"]
		device = strings.TrimPrefix(device, ":")

		addr := volumeData.Options["o"]
		addr = strings.Split(strings.TrimPrefix(addr, "addr="), ",")[0]
		path := os.Getenv(NFS_PATH_ENV_NAME)
		if path == "" {
			log.Errorf("NFS_PATH environment variable is not set.")
			return
		}
		paths := strings.Split(path, ",")
		for _, p := range paths {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(device, p) {
				MkNfsDir(addr, "", p, device)
				break
			}
		}

	}

}
func MkNfsDir(ip string, port string, nfsPath string, dir string) {
	dir = strings.TrimPrefix(dir, nfsPath)
	dir = strings.TrimPrefix(dir, "/")
	// connect
	server := ip
	if port != "" {
		server += ":" + port
	}

	mount, err := nfs.DialMount(server, true)
	if err != nil {
		log.Errorf("unable to dial MOUNT service: %v", err)
		return
	}
	defer mount.Close()
	// auth
	auth := rpc.NewAuthUnix("root", 0, 0)
	v, err := mount.Mount(nfsPath, auth.Auth())
	if err != nil {
		log.Errorf("unable to mount volume: %v", err)
		return
	}
	defer v.Close()
	MkNFS3Dirs(v, dir)
	v.Close()
	mount.Close()
}
func main() {
	// MkNfsDir("172.168.1.13", "", "/datadisk/nfs/test", "/datadisk/nfs/test/aa/bb/cc2/")
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Errorf("Error create docker client: %s", err)
		return
	}
	log.Info("Started.")
	ctx := context.Background()
	msgs, errs := cli.Events(ctx, events.ListOptions{})

	for {
		select {
		case err := <-errs:
			log.Errorf("Docker error event: %v", err)
			return
		case msg := <-msgs:
			if !strings.HasPrefix(fmt.Sprintf("%v", msg.Action), "exec_") {
				log.Infof("Event: %s %s  %s %s", msg.Action, msg.Type, msg.Actor.ID, msg.Actor.Attributes["name"])
			}
			if msg.Type == "service" && msg.Action == "create" {
				// 服务创建时，挂载卷
				data, _, _ := cli.ServiceInspectWithRaw(ctx, msg.Actor.ID, types.ServiceInspectOptions{})
				mountsList := data.Spec.TaskTemplate.ContainerSpec.Mounts
				for _, mountData := range mountsList {
					if mountData.Type == "bind" && !mountData.ReadOnly && !strings.HasSuffix(mountData.Target, ".sock") {
						markDir(mountData.Source)
					} else if mountData.Type == "volume" {
						// markNfsDir(cli, mountData.Source)
					}
				}
				// dataJson, _ := json.Marshal(data)
				// log.Infof("Service %s %s %s %s", msg.Actor.ID, msg.Type, msg.Action, string(dataJson))
			} else if msg.Type == "container" && msg.Action == "create" {
				// 容器创建时，挂载卷
				data, _ := cli.ContainerInspect(ctx, msg.Actor.ID)
				mountsList := data.Mounts
				for _, mountData := range mountsList {
					if mountData.Type == "bind" && mountData.RW && !strings.HasSuffix(mountData.Destination, ".sock") {
						markDir(mountData.Source)
					} else if mountData.Type == "volume" {
						markNfsDir(cli, mountData.Name)
					}
				}
			} else if msg.Type == "volume" && msg.Action == "create" {
				// 卷创建时，自动创建nfs目录
				markNfsDir(cli, msg.Actor.ID)
			} else if msg.Type == "container" && (msg.Action == "destroy") {
				// 容器销毁时，删除未使用的卷
				removeUnusedVolumes(ctx, cli)
			} else if msg.Type == "service" && (msg.Action == "destroy") {
				// 容器销毁时，删除未使用的卷
				removeUnusedVolumes(ctx, cli)
			}
		}
	}
}
func removeUnusedVolumes(ctx context.Context, cli *client.Client) {
	volumes, _ := cli.VolumeList(ctx, volume.ListOptions{})
	for _, volume := range volumes.Volumes {
		if volume.Driver == "local" && volume.Options["type"] == "nfs" {
			removeVolume(ctx, cli, volume.Name)
		}
	}
}
func removeVolume(ctx context.Context, cli *client.Client, volumeId string) {
	if !isVolumeUsed(ctx, cli, volumeId) {
		// 未使用则的删除卷
		err := cli.VolumeRemove(ctx, volumeId, false)
		if err != nil {
			log.Errorf("Failed to remove volume: %s", err)
		} else {
			log.Infof("Volume removed: %s", volumeId)
		}
	}
}
func isVolumeUsed(ctx context.Context, cli *client.Client, volumeId string) bool {
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return false
	}
	for _, container := range containers {
		for _, mount := range container.Mounts {
			if mount.Name == volumeId {
				return true
			}
		}
	}
	return false
}
