package main

import (
	"context"
	"os"
	"path/filepath"
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

func markDir(path string) {
	err := os.MkdirAll(path, 0777)
	if err != nil {
		log.Errorf("local mkdir: %s, Error: %v", path, err)
	} else {
		log.Infof("local mkdir: %s", path)
	}
}

func MkDirs(v *nfs.Target, dir string) {
	f, _, err := v.Lookup(dir)
	if err == nil && f.IsDir() {
		// already exists
		return
	}
	if _, err := v.Mkdir(dir, 0775); err != nil {
		if err.Error() == "file does not exist" {
			MkDirs(v, filepath.Dir(dir))
			v.Mkdir(dir, 0775)
		} else {
			log.Errorf("nfs mkdir: %s, Error: %v", dir, err)
			return
		}
	}
	log.Infof("nfs mkdir: %s", dir)
}

const NFS_PATH_ENV_NAME = "NFS_PATH"

func markNfsDir(cli *client.Client, volumeId string) {
	volumeData, _ := cli.VolumeInspect(context.Background(), volumeId)
	if volumeData.Driver == "local" && volumeData.Options["type"] == "nfs" {
		device := volumeData.Options["device"]
		device = strings.TrimPrefix(device, ":")

		addr := volumeData.Options["o"]
		addr = strings.Split(strings.TrimPrefix(addr, "addr="), ",")[0]
		path := os.Getenv(NFS_PATH_ENV_NAME)

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
	// log.Infof("NFS Server: %s, NFS Path: %s dir: %s", server, nfsPath, dir)
	mount, err := nfs.DialMount(server, false)
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
	MkDirs(v, dir)
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
			log.Infof("Event: %s      \t%s      \t%s \t%s", msg.Action, msg.Type, msg.Actor.ID, msg.Actor.Attributes["name"])
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
