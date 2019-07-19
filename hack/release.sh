#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1 && pwd)
readonly version=$(cat ${root}/VERSION)
readonly git_branch=${1:11} # drop 'refs/head/' prefix
readonly git_sha=$(git rev-parse HEAD)
readonly git_timestamp=$(TZ=UTC git show --quiet --date='format-local:%Y%m%d%H%M%S' --format="%cd")
readonly slug=${version}-${git_timestamp}-${git_sha:0:16}

echo "Building riff System"
(cd $root && KO_DOCKER_REPO="gcr.io/projectriff/system" ko resolve -P -t "${version}" -t "${slug}" -f config/ | \
  sed -e "s|projectriff.io/release: devel|projectriff.io/release: \"${version}\"|" > ${root}/riff-system.yaml)

echo "Publishing riff System"
gsutil cp -a public-read ${root}/riff-system.yaml gs://projectriff/riff-system/snapshots/riff-system-${slug}.yaml
gsutil cp -a public-read ${root}/riff-system.yaml gs://projectriff/riff-system/riff-system-${version}.yaml

echo "Publishing version references"
gsutil -h 'Content-Type: text/plain' -h 'Cache-Control: private' cp -a public-read <(echo "${slug}") gs://projectriff/riff-system/snapshots/versions/${git_branch}
gsutil -h 'Content-Type: text/plain' -h 'Cache-Control: private' cp -a public-read <(echo "${slug}") gs://projectriff/riff-system/snapshots/versions/${version}
if [[ ${version} != *"-snapshot" ]] ; then
  gsutil -h 'Content-Type: text/plain' -h 'Cache-Control: private' cp -a public-read <(echo "${version}") gs://projectriff/riff-system/versions/releases/${git_branch}
fi
