# gungnir
(pronounced “GUNG-neer”)

[![Build Status](https://travis-ci.com/Comcast/codex-gungnir.svg?branch=master)](https://travis-ci.com/Comcast/codex-gungnir)
[![codecov.io](http://codecov.io/github/Comcast/codex-gungnir/coverage.svg?branch=master)](http://codecov.io/github/Comcast/codex-gungnir?branch=master)
[![Code Climate](https://codeclimate.com/github/Comcast/codex-gungnir/badges/gpa.svg)](https://codeclimate.com/github/Comcast/codex-gungnir)
[![Issue Count](https://codeclimate.com/github/Comcast/codex-gungnir/badges/issue_count.svg)](https://codeclimate.com/github/Comcast/codex-gungnir)
[![Go Report Card](https://goreportcard.com/badge/github.com/Comcast/codex-gungnir)](https://goreportcard.com/report/github.com/Comcast/codex-gungnir)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/Comcast/codex-gungnir/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/Comcast/codex-gungnir.svg)](CHANGELOG.md)


## Summary

Gungnir provides an API to get information about a device, based upon events 
found in the database.  It can also return the events themselves, which are
[WRP Messages](https://github.com/Comcast/wrp-c/wiki/Web-Routing-Protocol).
For more information on how Gungnir fits into codex, check out [the codex README](https://github.com/Comcast/codex).

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
GO111MODULE=on go get github.com/Comcast/codex-gungnir
```

You can also clone the repository yourself and build using make:

```bash
mkdir -p $GOPATH/src/github.com/Comcast
cd $GOPATH/src/github.com/Comcast
git clone git@github.com:Comcast/codex-gungnir.git
cd codex-gungnir
make build
```

### Makefile

The Makefile has the following options you may find helpful:
* `make build`: builds the Gungnir binary
* `make rpm`: builds an rpm containing Gungnir
* `make docker`: builds a docker image for Gungnir, making sure to get all 
   dependencies
* `make local-docker`: builds a docker image for Gungnir with the assumption
   that the dependencies can be found already
* `make it`: runs `make docker`, then deploys Gungnir and a cockroachdb 
   database into docker.
* `make test`: runs unit tests with coverage for Gungnir
* `make clean`: deletes previously-built binaries and object files

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

For deploying on Docker or in Kubernetes, refer to the [deploy README](https://github.com/Comcast/codex/tree/master/deploy/README.md).

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