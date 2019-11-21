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

export KO_DOCKER_REPO=$(fats_image_repo '#' | cut -d '#' -f 1 | sed 's|/$||g')

echo "Initialize Helm"
source $FATS_DIR/macros/helm-init.sh
helm repo add projectriff https://projectriff.storage.googleapis.com/charts/releases
helm repo update

echo "Installing Cert Manager"
#TODO: change back to Helm after charts are updated with cert-manager v0.11.0
#helm install projectriff/cert-manager --name cert-manager --devel --wait
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
#TODO: ^^^
sleep 5
wait_pod_selector_ready app=cert-manager cert-manager
wait_pod_selector_ready app=webhook cert-manager

#TODO: change back to FATS after it is with cert-manager v0.11.0
# source $FATS_DIR/macros/no-resource-requests.sh
if [ $(kubectl get nodes -oname | wc -l) = "1" ]; then
  echo "Eliminate pod resource requests"
  fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/no-resource-requests-webhook/no-resource-requests-webhook-20191121210956-521ae2a8c3323540.yaml
  wait_pod_selector_ready app=webhook no-resource-requests
fi
#TODO: ^^^

echo "Installing kpack"
fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/internal/kpack/kpack-0.0.5-snapshot-5a4e635d.yaml

echo "Installing riff Build"
if [ $MODE = "push" ]; then
  fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-build-${slug}.yaml
elif [ $MODE = "pull_request" ]; then
  ko apply -f config/riff-build.yaml
fi
fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-application-clusterbuilder.yaml
fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-function-clusterbuilder.yaml

if [ $RUNTIME = "core" ]; then
  echo "Installing riff Core Runtime"
  if [ $MODE = "push" ]; then
    fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-core-${slug}.yaml
  elif [ $MODE = "pull_request" ]; then
    ko apply -f config/riff-core.yaml
  fi
elif [ $RUNTIME = "knative" ]; then
  echo "Installing Istio"
  helm install projectriff/istio --name istio --namespace istio-system --devel --wait --set gateways.istio-ingressgateway.type=${K8S_SERVICE_TYPE}
  echo "Checking for ready ingress"
  wait_for_ingress_ready 'istio-ingressgateway' 'istio-system'
  
  echo "Installing Knative Serving"
  fats_retry kubectl apply -f https://storage.googleapis.com/knative-releases/serving/previous/v0.9.0/serving-post-1.14.yaml

  echo "Installing riff Knative Runtime"
  if [ $MODE = "push" ]; then
    fats_retry kubectl apply -f https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-knative-${slug}.yaml
  elif [ $MODE = "pull_request" ]; then
    ko apply -f config/riff-knative.yaml
  fi
elif [ $RUNTIME = "streaming" ]; then
  echo "Streaming runtime is not implemented yet"
  exit 1
fi

wait_pod_selector_ready "component=build.projectriff.io,control-plane=controller-manager" riff-system
wait_pod_selector_ready "component=${RUNTIME}.projectriff.io,control-plane=controller-manager" riff-system
