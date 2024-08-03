# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM alpine:3.9
LABEL author=sorc@sction.org
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0
COPY ./ssh/* /root/.ssh/
COPY ./dist/dnam_${TARGETARCH} /app/dnam
RUN apk add --no-cache tzdata && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && apk del tzdata && chmod +x /app/dnam && chmod 600 /root/.ssh/id_ed25519
ENV TZ="Asia/Shanghai"
WORKDIR /app
CMD ["/app/dnam"]