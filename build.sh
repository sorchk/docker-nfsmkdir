#!/bin/bash
go env -w CGO_ENABLED=0
go build -o build/dnam main.go
ssh root@172.168.1.31 "rm -f /root/dnam"
scp build/dnam root@172.168.1.31:/root/
ssh root@172.168.1.31 "chmod +x /root/dnam"
ssh root@172.168.1.31 "/root/dnam"

#创建buildx环境,并使用本机代理

#docker buildx create --use --name mybuilder2 \
#    --driver-opt env.http_proxy=http://10.10.10.41:2082 \
#    --driver-opt env.https_proxy=http://10.10.10.41:2082

#修改所使用的buildx环境
#docker buildx use mybuilder

#删除环境
#docker buildx rm mybuilder
