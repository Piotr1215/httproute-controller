# HTTPRoute Controller

Kubernetes controller that automatically generates Gateway API HTTPRoutes from Service annotations.

## Overview

This controller watches Kubernetes Services and automatically creates Gateway API HTTPRoute and ReferenceGrant resources based on annotations. It eliminates the need to manually create HTTPRoute resources for each service you want to expose through a Gateway.

**Built with:**
- Kubebuilder v4.5.1
- Gateway API v1.2.1
- Controller-runtime v0.20.2
- Modern Kubernetes controller best practices (2024-2025)

## Architecture

**Controller Name:** `homelab.local/httproute-controller`

**Reconciliation Pattern:**
- **Level-based triggers** - Reconciles full state, not just events
- **Idempotent operations** - Safe to call multiple times with same inputs
- **OwnerReferences** - Automatic garbage collection
- **Cross-namespace** - Secure cross-namespace access via ReferenceGrant

## Features

✅ **Automatic HTTPRoute generation** from Service annotations
✅ **Cross-namespace support** via ReferenceGrant
✅ **OwnerReferences** for automatic cleanup
✅ **Configurable gateway** (name, namespace)
✅ **Idempotent reconciliation** (modern controller patterns)
✅ **Comprehensive test coverage** (TDD approach)

## Usage

### Annotations

| Annotation | Required | Default | Description |
|------------|----------|---------|-------------|
| `gateway.homelab.local/expose` | Yes | - | Set to `"true"` to enable |
| `gateway.homelab.local/hostname` | Yes | - | DNS hostname (e.g., `myapp.homelab.local`) |
| `gateway.homelab.local/gateway` | No | `homelab-gateway` | Gateway name |
| `gateway.homelab.local/gateway-namespace` | No | `envoy-gateway-system` | Gateway namespace |
| `gateway.homelab.local/port` | No | First port | Service port |

### Example

```yaml
apiVersion: v1
kind: Service
metadata:
  name: myapp
  namespace: default
  annotations:
    gateway.homelab.local/expose: "true"
    gateway.homelab.local/hostname: "myapp.homelab.local"
spec:
  selector:
    app: myapp
  ports:
  - port: 80
    targetPort: 8080
```

**Controller automatically creates:**

1. **HTTPRoute** (in gateway namespace):
   - Name: `default-myapp`
   - Hostname: `myapp.homelab.local`
   - Backend: Service `myapp` in namespace `default`
   - OwnerReference to Service (for cleanup)

2. **ReferenceGrant** (in service namespace):
   - Name: `myapp-backend`
   - Allows HTTPRoute from gateway namespace to reference Service
   - OwnerReference to Service (automatic garbage collection)

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/httproute-controller:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/httproute-controller:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/httproute-controller:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/httproute-controller/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

