#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

source config.sh
readonly git_branch=${1:11} # drop 'refs/head/' prefix

retagImage() {
  docker pull $1
  docker tag $1 $2
  docker push $2
}

publishComponent() {
  local component=$1

  gsutil cp -a public-read gs://projectriff/riff-system/snapshots/riff-${component}-${slug}.yaml gs://projectriff/riff-system/riff-${component}-${version}.yaml
  # NOTE: retagged images are consumed by /config/riff-${comonent}.yaml, but not the publisehd yaml, which is still pinned to a gitsha
  retagImage $img_repo/$component-controller:$slug $img_repo/$component-controller:$version
}

echo "Publishing riff System"
publishComponent build
publishComponent core
publishComponent knative
publishComponent streaming

echo "Publishing version references"
gsutil -h 'Content-Type: text/plain' -h 'Cache-Control: private' cp -a public-read <(echo "${slug}") gs://projectriff/riff-system/snapshots/versions/${git_branch}
gsutil -h 'Content-Type: text/plain' -h 'Cache-Control: private' cp -a public-read <(echo "${slug}") gs://projectriff/riff-system/snapshots/versions/${version}
if [[ ${version} != *"-snapshot" ]] ; then
  gsutil -h 'Content-Type: text/plain' -h 'Cache-Control: private' cp -a public-read <(echo "${version}") gs://projectriff/riff-system/versions/releases/${git_branch}
fi
