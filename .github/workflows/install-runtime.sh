#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly version=$(cat VERSION)
readonly git_sha=$(git rev-parse HEAD)
readonly git_timestamp=$(TZ=UTC git show --quiet --date='format-local:%Y%m%d%H%M%S' --format="%cd")
readonly slug=${version}-${git_timestamp}-${git_sha:0:16}

readonly tiller_service_account=tiller
readonly tiller_namespace=kube-system

source ${FATS_DIR}/.configure.sh

retry() {
  local result=0
  local count=1
  while [ $count -le 3 ]; do
    [ $result -ne 0 ] && {
      echo -e "\n${ANSI_RED}The command \"$@\" failed. Retrying, $count of 3.${ANSI_RESET}\n" >&2
    }
    # ! { } ignores set -e, see https://stackoverflow.com/a/4073372
    ! { "$@"; result=$?; }
    [ $result -eq 0 ] && break
    count=$(($count + 1))
    sleep 1
  done
    [ $count -gt 3 ] && {
      echo -e "\n${ANSI_RED}The command \"$@\" failed 3 times.${ANSI_RESET}\n" >&2
    }
  return $result
}

apply_yaml() {
  local file=$1
  local tmpfile=$(mktemp)
  
  ytt --ignore-unknown-comments -f .github/overlays/ -f ${file} --file-mark $(basename ${file}):type=yaml-plain > $tmpfile
  retry kubectl apply -f $tmpfile
  rm $tmpfile
}

echo "Initialize Helm"
kubectl create serviceaccount ${tiller_service_account} -n ${tiller_namespace}
kubectl create clusterrolebinding "${tiller_service_account}-cluster-admin" --clusterrole cluster-admin --serviceaccount "${tiller_namespace}:${tiller_service_account}"
helm init --wait --service-account ${tiller_service_account}

helm repo add projectriff https://projectriff.storage.googleapis.com/charts/releases
helm repo update

echo "Installing Knative Build"
apply_yaml https://storage.googleapis.com/knative-releases/build/previous/v0.7.0/build.yaml

echo "Installing riff Build"
apply_yaml https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-build-${slug}.yaml
apply_yaml https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-application-clusterbuildtemplate.yaml
apply_yaml https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-function-clusterbuildtemplate.yaml

if [ $RUNTIME = "core" ]; then
  echo "Installing riff Core Runtime"
  apply_yaml https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-core-${slug}.yaml
elif [ $RUNTIME = "knative" ]; then
  echo "Installing Istio"
  helm install projectriff/istio --name istio --namespace istio-system --wait --set gateways.istio-ingressgateway.type=${K8S_SERVICE_TYPE}
  echo "Checking for ready ingress"
  wait_for_ingress_ready 'istio-ingressgateway' 'istio-system'
  
  echo "Installing Knative Serving"
  apply_yaml https://storage.googleapis.com/knative-releases/serving/previous/v0.9.0/serving-post-1.14.yaml

  echo "Installing riff Knative Runtime"
  apply_yaml https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-knative-${slug}.yaml
elif [ $RUNTIME = "streaming" ]; then
  echo "Streaming runtime is not implemented yet"
  exit 1
fi
