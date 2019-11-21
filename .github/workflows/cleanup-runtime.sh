#!/usr/bin/env bash

set -o nounset

readonly version=$(cat VERSION)
readonly git_sha=$(git rev-parse HEAD)
readonly git_timestamp=$(TZ=UTC git show --quiet --date='format-local:%Y%m%d%H%M%S' --format="%cd")
readonly slug=${version}-${git_timestamp}-${git_sha:0:16}

source $FATS_DIR/macros/cleanup-user-resources.sh

echo "Cleanup riff Build"
kubectl delete -f https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-build-${slug}.yaml
kubectl delete -f https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-application-clusterbuilder.yaml
kubectl delete -f https://storage.googleapis.com/projectriff/riff-buildtemplate/riff-function-clusterbuilder.yaml

echo "Cleanup kpack"
kubectl delete -f https://storage.googleapis.com/projectriff/internal/kpack/kpack-0.0.5-snapshot-5a4e635d.yaml

if [ $RUNTIME = "core" ]; then
  echo "Cleanup riff Core Runtime"
  kubectl delete -f https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-core-${slug}.yaml

elif [ $RUNTIME = "knative" ]; then
  echo "Cleanup Istio"
  helm delete --purge istio
  kubectl delete namespace istio-system
  kubectl get customresourcedefinitions.apiextensions.k8s.io -oname | grep istio.io | xargs -L1 kubectl delete

  echo "Cleanup Knative Serving"
  kubectl delete knative -n $NAMESPACE --all
  retry kubectl apply -f https://storage.googleapis.com/knative-releases/serving/previous/v0.9.0/serving-post-1.14.yaml

  echo "Cleanup riff Knative Runtime"
  kubectl delete -f https://storage.googleapis.com/projectriff/riff-system/snapshots/riff-knative-${slug}.yaml

fi

echo "Cleanup Cert Manager"
#TODO: change back to Helm after charts are updated with cert-manager v0.11.0
#helm delete --purge cert-manager
kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
#TODO: ^^^
kubectl delete customresourcedefinitions.apiextensions.k8s.io -l app.kubernetes.io/managed-by=Tiller,app.kubernetes.io/instance=cert-manager 

echo "Remove Helm"
source $FATS_DIR/macros/helm-reset.sh
