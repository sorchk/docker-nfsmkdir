#!/bin/bash
name=dnam
ver=$1
build_date=$(date +"%Y%m%d")
if [ -z "${ver}" ]; then
  ver=0.1.0
fi
echo ${ver}_${build_date}
export DOCKER_CLI_EXPERIMENTAL=enabled
cat ~/.docker/key.bak | docker login --username sorc --password-stdin
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VER=${ver} \
  --build-arg BUILD_DATE=${build_date} \
  --push \
  --tag sorc/${name}:${ver}_${build_date} \
  --tag sorc/${name}:${ver} \
  --tag sorc/${name}:latest .
