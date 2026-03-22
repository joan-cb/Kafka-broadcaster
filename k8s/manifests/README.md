# Plain Kubernetes manifests

These replace the former Helm chart. Edit the files for your cluster (image, namespace, `ConfigMap` content), then apply:

```bash
kubectl apply -f k8s/manifests/
```

(`optional/` is not included — see below.)

**Config:** Edit `configmap.yaml` (`data.config.yaml`) with your Kafka brokers, topics, and routing rules. After changing the ConfigMap, restart the deployment so pods pick up the new file:

```bash
kubectl rollout restart deployment/kafka-broadcaster -n kafka-broadcaster
```

**ServiceMonitor (Prometheus Operator):** only if your cluster has the `ServiceMonitor` CRD:

```bash
kubectl apply -f k8s/manifests/optional/servicemonitor.yaml
```
