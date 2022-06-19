# gungnir
(pronounced “GUNG-neer”)

[![Build Status](https://github.com/xmidt-org/gungnir/actions/workflows/ci.yml/badge.svg)](https://github.com/xmidt-org/gungnir/actions/workflows/ci.yml)
[![Dependency Updateer](https://github.com/xmidt-org/gungnir/actions/workflows/updater.yml/badge.svg)](https://github.com/xmidt-org/gungnir/actions/workflows/updater.yml)
[![codecov.io](http://codecov.io/github/xmidt-org/gungnir/coverage.svg?branch=main)](http://codecov.io/github/xmidt-org/gungnir?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/gungnir)](https://goreportcard.com/report/github.com/xmidt-org/gungnir)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=xmidt-org_gungnir&metric=alert_status)](https://sonarcloud.io/dashboard?id=xmidt-org_gungnir)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/gungnir/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/release/xmidt-org/gungnir.svg)](CHANGELOG.md)


## Summary

Gungnir provides an API to get information about a device, based upon events 
found in the database.  It can also return the events themselves, which are
[WRP Messages](https://github.com/xmidt-org/wrp-c/wiki/Web-Routing-Protocol).
For more information on how Gungnir fits into codex, check out [the codex README](https://github.com/xmidt-org/codex).

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Details](#details)
- [Build](#build)
- [Deploy](#deploy)
- [Contributing](#contributing)

## Code of Conduct

This project and everyone participating in it are governed by the [XMiDT Code Of Conduct](https://xmidt.io/code_of_conduct/). 
By participating, you agree to this Code.

## Details

Gungnir has two endpoints currently:
* `/device/{deviceID}/events` provides a list of events for the specified 
  device id, ordered in descending order by record `birth date`.  The list of 
  events are a list of WRP messages extended to also include the `BirthDate` of 
  the record.
* `/device/{deviceID}/status` provides the status of the device according to 
  the most recent `birth date`.  The values it returns are:
  * the device id
  * the state
  * the record's birth date
  * the current time
  * the reason the device went offline most recently

When Gungnir received a request to either endpoint, it first validates that 
the request is authorized.  This authorization is configurable.  Then, Gungnir 
gets records for that device id, limited by a configurable max number of 
records but sorted in descending order by `birth date`.  Gungnir checks how the 
record is encrypted, if at all, and decrypts it.  Then it decodes the message 
into a struct so that it can parse the information as necessary before returning 
the information the consumer asked for.

The events are stored in the database as [MsgPack](https://msgpack.org/index.html),
and gungnir uses [ugorji's implementation](https://github.com/ugorji/go) to 
decode them.

## Build

### Source

In order to build from the source, you need a working Go environment with 
version 1.11 or greater. Find more information on the [Go website](https://golang.org/doc/install).

You can directly use `go get` to put the Gungnir binary into your `GOPATH`:
```bash
GO111MODULE=on go get github.com/xmidt-org/gungnir
```

You can also clone the repository yourself and build using make:

```bash
mkdir -p $GOPATH/src/github.com/xmidt-org
cd $GOPATH/src/github.com/xmidt-org
git clone git@github.com:xmidt-org/gungnir.git
cd gungnir
make build
```

### Makefile

The Makefile has the following options you may find helpful:
* `make build`: builds the Gungnir binary
* `make docker`: builds a docker image for Gungnir, making sure to get all 
   dependencies
* `make local-docker`: builds a docker image for Gungnir with the assumption
   that the dependencies can be found already
* `make it`: runs `make docker`, then deploys Gungnir and a cockroachdb 
   database into docker.
* `make test`: runs unit tests with coverage for Gungnir
* `make clean`: deletes previously-built binaries and object files

### RPM

First have a local clone of the source and go into the root directory of the 
repository.  Then use rpkg to build the rpm:
```bash
rpkg srpm --spec <repo location>/<spec file location in repo>
rpkg -C <repo location>/.config/rpkg.conf sources --outdir <repo location>'
```

### Docker

The docker image can be built either with the Makefile or by running a docker 
command.  Either option requires first getting the source code.

See [Makefile](#Makefile) on specifics of how to build the image that way.

For running a command, either you can run `docker build` after getting all 
dependencies, or make the command fetch the dependencies.  If you don't want to 
get the dependencies, run the following command:
```bash
docker build -t gungnir:local -f deploy/Dockerfile .
```
If you want to get the dependencies then build, run the following commands:
```bash
GO111MODULE=on go mod vendor
docker build -t gungnir:local -f deploy/Dockerfile.local .
```

For either command, if you want the tag to be a version instead of `local`, 
then replace `local` in the `docker build` command.

### Kubernetes

WIP. TODO: add info

## Deploy

For deploying on Docker or in Kubernetes, refer to the [deploy README](https://github.com/xmidt-org/codex-deploy/tree/main/deploy/README.md).

For running locally, ensure you have the binary [built](#Source).  If it's in 
your `GOPATH`, run:
```
gungnir
```
If the binary is in your current folder, run:
```
./gungnir
```

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).
