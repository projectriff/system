apiVersion: v1
kind: ConfigMap
metadata:
  name: inmemory-gateway
data:
  gatewayImage: scothis/liiklus:test-pulsar-fix
  provisionerImage: {{ gcloud container images describe gcr.io/projectriff/nop-provisioner/provisioner:0.1.0-snapshot --format="value(image_summary.fully_qualified_digest)" }}
