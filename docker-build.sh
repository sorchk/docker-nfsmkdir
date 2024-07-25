#!/bin/bash
name=dnam
ver=$1
build_date=$(date +"%Y%m%d")
if [ -z "${ver}" ]; then
  ver=0.1.0
fi
echo ${ver}_${build_date}
export DOCKER_CLI_EXPERIMENTAL=enabled
cat docker/key.bak | docker login --username ${docker_user} --password-stdin
docker build -t sorc/${name}:${ver}_${build_date} .
docker tag sorc/${name}:${ver}_${build_date} sorc/${name}:${ver}
docker tag sorc/${name}:${ver}_${build_date} sorc/${name}:latest
docker push sorc/${name}:${ver}
docker push sorc/${name}:${ver}_${build_date}
docker push sorc/${name}:latest


# docker buildx build \
#   --platform linux/amd64 \
#   --build-arg VER=${ver} \
#   --build-arg BUILD_DATE=${build_date} \
#   --build-arg HTTP_PROXY=http://10.10.10.41:2082 \
#   --build-arg HTTPS_PROXY=http://10.10.10.41:2082 \
#   --build-arg ALL_PROXY=http://10.10.10.41:2082 \
#   --push \
#   --tag sorc/${name}:${ver}_${build_date} \
#   --tag sorc/${name}:${ver} \
#   --tag sorc/${name}:latest .
