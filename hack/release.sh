#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

gcloud auth activate-service-account --key-file <(echo ${GCLOUD_CLIENT_SECRET} | base64 --decode)

readonly root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1 && pwd)
readonly version=$(cat ${root}/VERSION)
readonly gitsha=$(git rev-parse HEAD)

echo "Building riff System"
(cd $root && KO_DOCKER_REPO="gcr.io/cf-spring-pfs-eng/riff-system" ko resolve -P -t "${version}" -t "${version}-${gitsha}" -f config/ | \
  sed -e "s|projectriff.io/release: devel|projectriff.io/release: \"${version}\"|" > ${root}/riff-system.yaml)

echo "Publishing riff System"
gsutil cp -a public-read ${root}/riff-system.yaml gs://projectriff/riff-system/snapshots/riff-system-${version}-${gitsha}.yaml
gsutil cp -a public-read ${root}/riff-system.yaml gs://projectriff/riff-system/riff-system-${version}.yaml
