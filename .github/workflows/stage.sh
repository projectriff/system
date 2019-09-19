#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

source config.sh

buildImage() {
  local component=$1
  local img=$img_repo/$component-controller:$slug

  ( cd config/$component/manager && kustomize edit set image controller=$img )
  COMPONENT=$component IMG=$img make docker-push
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
