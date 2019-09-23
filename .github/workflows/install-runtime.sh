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
  local tmpfile=$(mktemp)
  
  if [ $CLUSTER = "minikube" ]; then
    ytt --ignore-unknown-comments -f .github/overlays/ -f - > $tmpfile
  else
    cat > $tmpfile
  fi
  retry kubectl apply -f $tmpfile
  rm $tmpfile
}

export KO_DOCKER_REPO=$(fats_image_repo '#' | cut -d '#' -f 1 | sed 's|/$||g')

echo "Initialize Helm"
kubectl create serviceaccount ${tiller_service_account} -n ${tiller_namespace}
kubectl create clusterrolebinding "${tiller_service_account}-cluster-admin" --clusterrole cluster-admin --serviceaccount "${tiller_namespace}:${tiller_service_account}"
helm init --wait --service-account ${tiller_service_account}

helm repo add projectriff https://projectriff.storage.googleapis.com/charts/releases
helm repo update

echo "Installing Knative Build"
curl -Ls https://storage.googleapis.com/knative-releases/build/previous/v0.7.0/build.yaml | apply_yaml

echo "Installing riff Build"
if [ $MODE = "push" ]; then
  curl -Ls https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-build-${slug}.yaml | apply_yaml
elif [ $MODE = "pull_request" ]; then
  ko resolve -f config/riff-build.yaml | apply_yaml
fi
curl -Ls https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-application-clusterbuildtemplate.yaml | apply_yaml
curl -Ls https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-function-clusterbuildtemplate.yaml | apply_yaml

if [ $RUNTIME = "core" ]; then
  echo "Installing riff Core Runtime"
  if [ $MODE = "push" ]; then
    curl -Ls https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-core-${slug}.yaml | apply_yaml
  elif [ $MODE = "pull_request" ]; then
    ko resolve -f config/riff-core.yaml | apply_yaml
  fi
elif [ $RUNTIME = "knative" ]; then
  echo "Installing Istio"
  helm install projectriff/istio --name istio --namespace istio-system --wait --set gateways.istio-ingressgateway.type=${K8S_SERVICE_TYPE}
  echo "Checking for ready ingress"
  wait_for_ingress_ready 'istio-ingressgateway' 'istio-system'
  
  echo "Installing Knative Serving"
  curl -Ls https://storage.googleapis.com/knative-releases/serving/previous/v0.9.0/serving-post-1.14.yaml | apply_yaml

  echo "Installing riff Knative Runtime"
  if [ $MODE = "push" ]; then
    curl -Ls https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-knative-${slug}.yaml | apply_yaml
  elif [ $MODE = "pull_request" ]; then
    ko resolve -f config/riff-knative.yaml | apply_yaml
  fi
elif [ $RUNTIME = "streaming" ]; then
  echo "Streaming runtime is not implemented yet"
  exit 1
fi
