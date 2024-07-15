docker run -d \
--name nfsmkdir \
-v /var/run/docker.sock:/var/run/docker.sock \
-v /datadisk:/datadisk \
sorc/dnam

docker rmi -f sorc/dnam 
docker run --rm -it  \
--name nfsmkdir \
-v /var/run/docker.sock:/var/run/docker.sock \
-v /datadisk:/datadisk \
sorc/dnam /bin/sh

# ssh -o StrictHostKeyChecking=no root@172.168.1.13
docker service create \
--mode global \
--name nfsmkdir \
--mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock \
--mount type=bind,src=/datadisk,dst=/datadisk \
sorc/dnam

docker service create \
--name nginx4 \
--network public \
--replicas 3 \
--publish 81:80 \
-e TZ="Asia/Shanghai" \
--mount 'type=volume,source=nfs_nginx_test4,target=/wwwdata,volume-driver=local,volume-opt=type=nfs,volume-opt=device=:/datadisk/nfs/test/nginxtest4,volume-opt=o=addr=172.168.1.13' \
nginx:latest

