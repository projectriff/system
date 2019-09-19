#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." >/dev/null 2>&1 && pwd)
cd ${root}
readonly version=$(cat VERSION)
readonly ref=$1
readonly git_branch=${ref:11} # drop 'refs/head/' prefix
readonly git_sha=$(git rev-parse HEAD)
readonly git_timestamp=$(TZ=UTC git show --quiet --date='format-local:%Y%m%d%H%M%S' --format="%cd")
readonly slug=${version}-${git_timestamp}-${git_sha:0:16}

export IMG_REPO=gcr.io/projectriff/system
export IMG_TAG=$slug

# build
export COMPONENT=build
make docker-push
pushd config/$COMPONENT/manager
kustomize edit set image controller=$IMG_REPO/$COMPONENT-controller:$slug
popd

# core
export COMPONENT=core
make docker-push
pushd config/$COMPONENT/manager
kustomize edit set image controller=$IMG_REPO/$COMPONENT-controller:$slug
popd

# knative
export COMPONENT=knative
make docker-push
pushd config/$COMPONENT/manager
kustomize edit set image controller=$IMG_REPO/$COMPONENT-controller:$slug
popd

# streaming
export COMPONENT=streaming
make docker-push
pushd config/$COMPONENT/manager
kustomize edit set image controller=$IMG_REPO/$COMPONENT-controller:$slug
popd
