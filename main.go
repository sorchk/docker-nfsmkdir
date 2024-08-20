package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/go-nfs/nfsv3/nfs"
	"github.com/go-nfs/nfsv3/nfs/rpc"
	log "github.com/sirupsen/logrus"
)

func markDir(path string) {
	err := os.MkdirAll(path, 0777)
	if err != nil {
		log.Errorf("local mkdir: %s, Error: %v\n", path, err)
	} else {
		log.Infof("local mkdir: %s\n", path)
	}
}

func MkDirs(v *nfs.Target, dir string) {
	if _, err := v.Mkdir(dir, 0775); err != nil {
		if err.Error() == "file does not exist" {
			MkDirs(v, filepath.Dir(dir))
			v.Mkdir(dir, 0775)
		} else {
			log.Errorf("nfs mkdir: %s, Error: %v\n", dir, err)
		}
	}
	log.Infof("nfs mkdir: %s\n", dir)
}

const NFS_HOST_ENV_NAME = "NFS_HOST"
const NFS_PORT_ENV_NAME = "NFS_PORT"
const NFS_PATH_ENV_NAME = "NFS_PATH"

func markNfsDir(cli *client.Client, volumeId string) {
	host := os.Getenv(NFS_HOST_ENV_NAME)
	port := os.Getenv(NFS_PORT_ENV_NAME)
	path := os.Getenv(NFS_PATH_ENV_NAME)

	volumeData, _ := cli.VolumeInspect(context.Background(), volumeId)
	if volumeData.Driver == "local" && volumeData.Options["type"] == "nfs" {
		device := volumeData.Options["device"]
		device = strings.TrimPrefix(device, ":")
		device = strings.TrimPrefix(device, path)
		device = strings.TrimPrefix(device, "/")
		MkNfsDir(host, port, path, device)
	}

}
func MkNfsDir(ip string, port string, nfsPath string, dir string) {
	// connect
	mount, err := nfs.DialMount(ip+":"+port, false)
	if err != nil {
		log.Errorf("unable to dial MOUNT service: %v\n", err)
	}
	defer mount.Close()
	// auth
	auth := rpc.NewAuthUnix("root", 0, 0)
	v, err := mount.Mount(nfsPath, auth.Auth())
	if err != nil {
		log.Errorf("unable to mount volume: %v\n", err)
	}
	defer v.Close()
	MkDirs(v, dir)
	v.Close()
	mount.Close()
}
func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Errorf("Error create docker client: %s\n", err)
		return
	}
	log.Info("Started.")
	msgs, errs := cli.Events(context.Background(), events.ListOptions{})

	for {
		select {
		case err := <-errs:
			log.Errorf("Docker error event: %v\n", err)
			return
		case msg := <-msgs:
			if msg.Type == "service" && msg.Action == "create" {
				log.Infof("Service %s %s %s\n", msg.Actor.ID, msg.Type, msg.Action)
				data, _, _ := cli.ServiceInspectWithRaw(context.Background(), msg.Actor.ID, types.ServiceInspectOptions{})
				mountsList := data.Spec.TaskTemplate.ContainerSpec.Mounts
				for _, mountData := range mountsList {
					if mountData.Type == "bind" {
						markDir(mountData.Source)
					}
				}
				dataJson, _ := json.Marshal(data)
				log.Infof("Service %s %s %s %s\n", msg.Actor.ID, msg.Type, msg.Action, string(dataJson))
			} else if msg.Type == "container" && msg.Action == "create" {
				log.Infof("Container %s %s %s\n", msg.Actor.ID, msg.Type, msg.Action)
				data, _ := cli.ContainerInspect(context.Background(), msg.Actor.ID)
				mountsList := data.Mounts
				for _, mountData := range mountsList {
					if mountData.Type == "bind" {
						markDir(mountData.Source)
					} else if mountData.Type == "volume" {
						markNfsDir(cli, mountData.Name)
					}
				}
			} else if msg.Type == "volume" && msg.Action == "create" {
				markNfsDir(cli, msg.Actor.ID)
			}
		}
	}
}
