#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

source ${FATS_DIR}/.configure.sh

# setup namespace
kubectl create namespace ${NAMESPACE}
fats_create_push_credentials ${NAMESPACE}

if [ $RUNTIME = "core" ] || [ $RUNTIME = "knative" ]; then
  for location in cluster local; do
    for test in function application; do
      name=system-${RUNTIME}-${location}-uppercase-node-${test}
      image=$(fats_image_repo ${name})

      echo "##[group]Run function $name"

      if [ $location = 'cluster' ] ; then
        riff $test create $name --image $image --namespace $NAMESPACE --tail \
          --git-repo https://github.com/${FATS_REPO}.git --git-revision ${FATS_REFSPEC} --sub-path ${test}s/uppercase/node &
      else
        riff $test create $name --image $image --namespace $NAMESPACE --tail \
          --local-path ${FATS_DIR}/${test}s/uppercase/node
      fi

      riff $RUNTIME deployer create $name --${test}-ref $name --namespace $NAMESPACE --tail
      if [ $test = 'function' ] ; then
        source ${FATS_DIR}/macros/invoke_${RUNTIME}_deployer.sh $name "-H Content-Type:text/plain -H Accept:text/plain -d system" SYSTEM
      else
        source ${FATS_DIR}/macros/invoke_${RUNTIME}_deployer.sh $name "--get --data-urlencode input=system" SYSTEM
      fi
      riff $RUNTIME deployer delete $name --namespace $NAMESPACE

      riff $test delete $name --namespace $NAMESPACE
      fats_delete_image $image

      echo "##[endgroup]"
    done
  done

elif [ $RUNTIME = "streaming" ]; then
  echo "TODO: test the streaming runtime"

fi
