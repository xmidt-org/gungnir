#!/usr/bin/env bash

docker --version
docker-compose --version

function check() {
  if [[ $1 -ne 0 ]] ; then
      exit 1
  fi
}

function gungnir-docker {
    GUNGNIR_VERSION="$(make version -s)"
    make docker
    check $?
}

function deploy {
    echo "Deploying Cluster"
    git clone https://github.com/xmidt-org/codex-deploy.git 2> /dev/null || true
    pushd codex-deploy/deploy/docker-compose
    GUNGNIR_VERSION=$GUNGNIR_VERSION docker-compose up -d db db-init gungnir
    check $?
    popd
    printf "\n"
}

gungnir-docker
echo "Gungnir V:$GUNGNIR_VERSION"
deploy
go get -d github.com/xmidt-org/codex-deploy/tests/...
printf "Starting Tests \n\n\n"
go run github.com/xmidt-org/codex-deploy/tests/runners/travis -feature=codex-deploy/tests/features/gungnir/travis
check $?
