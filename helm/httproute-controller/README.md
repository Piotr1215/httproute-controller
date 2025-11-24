# HTTPRoute Controller Helm Chart

Kubernetes controller that automatically generates Gateway API HTTPRoutes from Service annotations.

## Installation

```sh
helm install httproute-controller . \
  --namespace httproute-system \
  --create-namespace
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of controller replicas | `1` |
| `image.repository` | Controller image repository | `ghcr.io/piotr1215/httproute-controller` |
| `image.tag` | Controller image tag | `""` (uses appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `leaderElection.enabled` | Enable leader election | `true` |
| `metrics.enabled` | Enable metrics service | `true` |
| `metrics.port` | Metrics port | `8443` |

## Usage

After installation, annotate Services to expose them:

```sh
kubectl annotate service myapp \
  gateway.homelab.local/expose=true \
  gateway.homelab.local/hostname=myapp.homelab.local
```

The controller will automatically create:
- HTTPRoute in the gateway namespace
- ReferenceGrant for cross-namespace access

## Uninstall

```sh
helm uninstall httproute-controller -n httproute-system
```
