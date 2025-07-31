FROM docker.io/library/golang:1.19-alpine as builder

WORKDIR /src

ARG VERSION
ARG GITCOMMIT
ARG BUILDTIME

RUN apk add --no-cache --no-progress \
    ca-certificates \
    make \
    curl \
    git \
    openssh \
    gcc \
    libc-dev \
    upx

RUN mkdir -p /go/bin && \
    curl -o /go/bin/spruce https://github.com/geofffranks/spruce/releases/download/v1.29.0/spruce-linux-amd64 && \
    chmod +x /go/bin/spruce
COPY . .
RUN make test release

FROM alpine:3.12.1

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /src/gungnir /src/gungnir.yaml /src/deploy/packaging/entrypoint.sh /go/bin/spruce /src/Dockerfile /src/NOTICE /src/LICENSE /src/CHANGELOG.md /

RUN mkdir /etc/gungnir/ && touch /etc/gungnir/gungnir.yaml && chmod 666 /etc/gungnir/gungnir.yaml

USER nobody

ENTRYPOINT ["/entrypoint.sh"]

EXPOSE 6600
EXPOSE 6601
EXPOSE 6602
EXPOSE 6603

CMD ["/gungnir"]
