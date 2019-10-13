FROM golang:alpine as builder
MAINTAINER Jack Murdock <jack_murdock@comcast.com>

WORKDIR /go/src/github.com/xmidt-org/gungnir
ARG VERSION=undefined

RUN apk update && apk upgrade && \
    apk add --no-cache bash git openssh

COPY . .
RUN GO111MODULE=on go build -ldflags "-X 'main.BuildTime=`date -u '+%Y-%m-%d %H:%M:%S'`' -X main.GitCommit=`git rev-parse --short HEAD` -X main.Version=$VERSION" -o gungnir_linux_amd64

FROM alpine

RUN apk --no-cache add ca-certificates
RUN mkdir -p /etc/gungnir
VOLUME /etc/gungnir

EXPOSE 8080

COPY --from=builder /go/src/github.com/xmidt-org/gungnir/gungnir_linux_amd64 /
COPY gungnir.yaml /
ENTRYPOINT ["/gungnir_linux_amd64"]
