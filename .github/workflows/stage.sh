#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly version=$(cat VERSION)
readonly git_sha=$(git rev-parse HEAD)
readonly git_timestamp=$(TZ=UTC git show --quiet --date='format-local:%Y%m%d%H%M%S' --format="%cd")
readonly slug=${version}-${git_timestamp}-${git_sha:0:16}

export IMG_REPO=gcr.io/projectriff/system
export IMG_TAG=$slug

buildImage() {
  local component=$1

  ( cd config/$component/manager && kustomize edit set image controller=$IMG_REPO/$component-controller:$slug )
  COMPONENT=$component make docker-push
}

echo "Build riff System"
buildImage build
buildImage core
buildImage knative
buildImage streaming

make package

echo "Stage riff System"
gsutil cp -a public-read config/riff-build.yaml gs://projectriff/riff-system/snapshots/riff-build-${slug}.yaml
gsutil cp -a public-read config/riff-core.yaml gs://projectriff/riff-system/snapshots/riff-core-${slug}.yaml
gsutil cp -a public-read config/riff-knative.yaml gs://projectriff/riff-system/snapshots/riff-knative-${slug}.yaml
gsutil cp -a public-read config/riff-streaming.yaml gs://projectriff/riff-system/snapshots/riff-streaming-${slug}.yaml
