#!/usr/bin/env bash

set -o nounset

source $FATS_DIR/macros/cleanup-user-resources.sh

if [ $RUNTIME = "core" ]; then
  echo "Cleanup riff Core Runtime"
  kapp delete -n apps -a riff-core-runtime -y

elif [ $RUNTIME = "knative" ]; then
  echo "Cleanup riff Knative Runtime"
  kapp delete -n apps -a riff-knative-runtime -y

  echo "Cleanup Knative Serving"
  kapp delete -n apps -a knative -y

  echo "Cleanup Istio"
  kapp delete -n apps -a istio -y
  kubectl get customresourcedefinitions.apiextensions.k8s.io -oname | grep istio.io | xargs -L1 kubectl delete

elif [ $RUNTIME = "streaming" ]; then
  echo "Cleanup Kafka"
  kapp delete -n apps -a internal-only-kafka -y

  echo "Cleanup riff Streaming Runtime"
  kapp delete -n apps -a riff-streaming-runtime -y

  echo "Cleanup KEDA"
  kapp delete -n apps -a keda -y

fi

echo "Cleanup riff Build"
kapp delete -n apps -a riff-build -y
kapp delete -n apps -a riff-builders -y

echo "Cleanup kpack"
kapp delete -n apps -a kpack -y

echo "Cleanup Cert Manager"
kapp delete -n apps -a cert-manager -y
