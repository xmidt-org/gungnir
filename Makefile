DEFAULT: build

GO           ?= go
GOFMT        ?= $(GO)fmt
DOCKER_ORG   := xmidt
APP          := gungnir
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
GUNGNIR    := $(FIRST_GOPATH)/bin/gungnir

PROGVER = $(shell git describe --tags `git rev-list --tags --max-count=1` | tail -1 | sed 's/v\(.*\)/\1/')
RPM_VERSION=$(shell echo $(PROGVER) | sed 's/\(.*\)-\(.*\)/\1/')
RPM_RELEASE=$(shell echo $(PROGVER) | sed -n 's/.*-\(.*\)/\1/p'  | grep . && (echo "$(echo $(PROGVER) | sed 's/.*-\(.*\)/\1/')") || echo "1")
BUILDTIME = $(shell date -u '+%Y-%m-%d %H:%M:%S')
GITCOMMIT = $(shell git rev-parse --short HEAD)

.PHONY: go-mod-vendor
go-mod-vendor:
	GO111MODULE=on $(GO) mod vendor

.PHONY: build
build: go-mod-vendor
	$(GO) build -o gungnir

rpm:
	mkdir -p ./OPATH/SOURCES
	tar -czvf ./OPATH/SOURCES/$(APP)-$(RPM_VERSION)-$(RPM_RELEASE).tar.gz . --exclude ./.git --exclude ./OPATH --exclude ./conf --exclude ./deploy --exclude ./vendor
	cp conf/$(APP).service ./OPATH/SOURCES/
	cp conf/$(APP).yaml  ./OPATH/SOURCES/
	cp LICENSE ./OPATH/SOURCES/
	cp NOTICE ./OPATH/SOURCES/
	cp CHANGELOG.md ./OPATH/SOURCES/
	rpmbuild --define "_topdir $(CURDIR)/OPATH" \
		--define "_version $(RPM_VERSION)" \
		--define "_release $(RPM_RELEASE)" \
		-ba deploy/packaging/$(APP).spec

.PHONY: version
version:
	@echo $(PROGVER)


# If the first argument is "update-version"...
ifeq (update-version,$(firstword $(MAKECMDGOALS)))
  # use the rest as arguments for "update-version"
  RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  # ...and turn them into do-nothing targets
  $(eval $(RUN_ARGS):;@:)
endif

.PHONY: update-version
update-version:
	@echo "Update Version $(PROGVER) to $(RUN_ARGS)"
	git tag v$(RUN_ARGS)


.PHONY: install
install: go-mod-vendor
	go install -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)"

.PHONY: release-artifacts
release-artifacts: go-mod-vendor
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)" -o ./OPATH/gungnir-$(PROGVER).darwin-amd64
	GOOS=linux  GOARCH=amd64 go build -ldflags "-X 'main.BuildTime=$(BUILDTIME)' -X main.GitCommit=$(GITCOMMIT) -X main.Version=$(PROGVER)" -o ./OPATH/gungnir-$(PROGVER).linux-amd64

.PHONY: docker
docker:
	docker build \
		--build-arg VERSION=$(PROGVER) \
		--build-arg GITCOMMIT=$(GITCOMMIT) \
		--build-arg BUILDTIME='$(BUILDTIME)' \
		-f ./deploy/Dockerfile -t $(DOCKER_ORG)/gungnir:$(PROGVER) .

# build docker without running modules
.PHONY: local-docker
local-docker:
	docker build \
		--build-arg VERSION=$(PROGVER)+local \
		--build-arg GITCOMMIT=$(GITCOMMIT) \
		--build-arg BUILDTIME='$(BUILDTIME)' \
		-f ./deploy/Dockerfile.local -t $(DOCKER_ORG)/gungnir:local .

.PHONY: style
style:
	! gofmt -d $$(find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

.PHONY: test
test: go-mod-vendor
	GO111MODULE=on go test -v -race  -coverprofile=cover.out ./...

.PHONY: test-cover
test-cover: test
	go tool cover -html=cover.out

.PHONY: codecov
codecov: test
	curl -s https://codecov.io/bash | bash

.PHONEY: it
it:
	./it.sh

.PHONY: clean
clean:
	rm -rf ./gungnir ./OPATH ./coverage.txt ./vendor
