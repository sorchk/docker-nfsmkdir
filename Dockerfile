# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM alpine:3.9
LABEL author=sorc@sction.org
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0
COPY ./dist/dnam_${TARGETARCH} /app/dnam
RUN apk add tzdata \
&& cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
&& echo "Asia/Shanghai" > /etc/timezone \
&& apk del tzdata \
&& chmod +x /app/dnam
WORKDIR /app
CMD ["/app/dnam"]