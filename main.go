package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

func markDir(path string) {
	err := os.MkdirAll(path, 0777)
	if err != nil {
		log.Errorf("markDir: %s, Error: %v", path, err)
	} else {
		log.Infof("markDir: %s", path)
	}
}
func publicKeyAuthFunc() ssh.AuthMethod {
	key, err := os.ReadFile("/root/.ssh/id_ed25519")
	if err != nil {
		key = []byte(`-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACD/MHwmAOabpMki1kp6EXU91qEitiXddWyPKaDlgTtOaQAAAJhNz/XnTc/1
5wAAAAtzc2gtZWQyNTUxOQAAACD/MHwmAOabpMki1kp6EXU91qEitiXddWyPKaDlgTtOaQ
AAAEBa0wFQ3Epdz5t/3vaUCLAFjjS8h0jNgAQMRUFDGz0Civ8wfCYA5pukySLWSnoRdT3W
oSK2Jd11bI8poOWBO05pAAAAFHJvb3RAZG9ja2VyLW1hc3Rlci0xAQ==
-----END OPENSSH PRIVATE KEY-----`)
	}
	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatalf("ssh 关键签名失败, %s %s", err, key)
	}
	return ssh.PublicKeys(signer)
}

const NFS_SSH_PORT_ENV_NAME = "NFS_SSH_PORT"

func markNfsDir(cli *client.Client, volumeId string) {
	nfs_ssh_port, err := strconv.ParseInt(os.Getenv(NFS_SSH_PORT_ENV_NAME), 10, 64)
	if err != nil {
		nfs_ssh_port = 22
	}

	volumeData, _ := cli.VolumeInspect(context.Background(), volumeId)
	if volumeData.Driver == "local" && volumeData.Options["type"] == "nfs" {
		oList := strings.Split(volumeData.Options["o"], ",")
		addr := ""
		device := volumeData.Options["device"]
		if device[0:1] == ":" {
			device = device[1:]
		}
		for _, o := range oList {
			if o[0:5] == "addr=" {
				addr = o[5:]
				break
			}
		}
		if addr != "" {
			sshConfig := &ssh.ClientConfig{
				User: "root",
				Auth: []ssh.AuthMethod{
					publicKeyAuthFunc(),
				},
				// 忽略known_hosts检查
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         30 * time.Second,
			}
			// 建立SSH连接
			sshClient, dialErr := ssh.Dial("tcp", addr+":"+strconv.FormatInt(nfs_ssh_port, 10), sshConfig)
			if dialErr != nil {
				printHelp()
				log.Fatalf("Failed to conn ssh: %s", dialErr)
			}
			defer sshClient.Close()
			// 执行SSH命令
			session, sessionErr := sshClient.NewSession()
			if sessionErr != nil {
				log.Fatalf("Failed to create session: %s", sessionErr)
			}
			defer session.Close()
			// 运行命令
			command := "mkdir -p " + device
			comboErr := session.Run(command)
			if comboErr != nil {
				log.Fatalf("Failed to mkdir: %s", comboErr)
			}
			_ = session.Run("chmod -R 0777 " + device)
			_ = session.Run("chown -R nobody:nobody " + device)
		}
	}
}
func printHelp() {
	data, err := os.ReadFile("/root/.ssh/id_ed25519")
	if err == nil {
		log.Info("请在NFS服务器上添加免密登录的公钥：" + string(data))
	}
}
func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Errorf("Error create docker client: %s", err)
		return
	}
	printHelp()
	log.Info("Started.")
	msgs, errs := cli.Events(context.Background(), events.ListOptions{})

	for {
		select {
		case err := <-errs:
			log.Errorf("Docker error event: %v", err)
			return
		case msg := <-msgs:
			if msg.Type == "service" && msg.Action == "create" {
				log.Infof("Service %s %s %s", msg.Actor.ID, msg.Type, msg.Action)
				data, _, _ := cli.ServiceInspectWithRaw(context.Background(), msg.Actor.ID, types.ServiceInspectOptions{})
				mountsList := data.Spec.TaskTemplate.ContainerSpec.Mounts
				for _, mountData := range mountsList {
					if mountData.Type == "bind" {
						markDir(mountData.Source)
					}
				}
				dataJson, _ := json.Marshal(data)
				log.Infof("Service %s %s %s %s", msg.Actor.ID, msg.Type, msg.Action, string(dataJson))
			} else if msg.Type == "container" && msg.Action == "create" {
				log.Infof("Container %s %s %s", msg.Actor.ID, msg.Type, msg.Action)
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
