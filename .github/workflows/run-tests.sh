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
  # TODO move setup to fats
  ${FATS_DIR}/install.sh helm
  helm repo add incubator http://storage.googleapis.com/kubernetes-charts-incubator
  helm install --name my-kafka incubator/kafka --set replicas=1,zookeeper.replicaCount=1,zookeeper.env.ZK_HEAP_SIZE=128m --namespace $NAMESPACE --wait

  riff streaming kafka-provider create franz --bootstrap-servers my-kafka:9092 --namespace $NAMESPACE

  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: dev-utils
spec:
  containers:
  - name: dev-utils
    image: projectriff/dev-utils
EOF
  kubectl create clusterrolebinding dev-util-stream --clusterrole=riff-streaming-readonly-role --serviceaccount=default:default

  for test in java java-boot; do
    name=system-streaming-cluster-repeater-${test}
    image=$(fats_image_repo ${name})

    riff function create $name --image $image --namespace $NAMESPACE --tail \
          --git-repo https://github.com/${FATS_REPO}.git --git-revision ${FATS_REFSPEC} --sub-path functions/repeater/${test} &

    riff streaming stream create letters --namespace $NAMESPACE --provider franz-kafka-provisioner --content-type 'text/plain'
    riff streaming stream create numbers --namespace $NAMESPACE --provider franz-kafka-provisioner --content-type 'application/json'
    riff streaming stream create result --namespace $NAMESPACE --provider franz-kafka-provisioner --content-type 'text/plain'

    riff streaming processor create $name --function-ref $name --namespace $NAMESPACE --input letters --input numbers --output result --tail

    kubectl exec dev-utils -- publish letters --payload foo
    kubectl exec dev-utils -- publish numbers --payload 2 --content-type "application/json"
    ${FATS_DIR}/install.sh jq
    kubectl exec dev-utils -- publish letters --payload foo > result.txt &
    while [ $cnt -lt 60 ]; do
      cat result.txt.out
      actual_data=`cat result.out | jq -r .payload`
      echo "actual_data: $actual_data"
      if [ "$actual_data" == "[foo, foo]" ]; then
        echo "Check succedded!"
        return 0
      fi
      sleep 1
      cnt=$((cnt+1))
    done

  done

fi
