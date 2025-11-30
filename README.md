# my-mysql-operator

## Description

The my-mysql-operator is a Kubernetes operator that simplifies the deployment and management of MySQL database instances on Kubernetes clusters. The operator uses Custom Resource Definitions (CRDs) to allow users to declaratively define MySQL instances, and automatically creates and manages the necessary Kubernetes resources including:

- **Deployment**: MySQL container running the specified version
- **Service**: ClusterIP service exposing MySQL on port 3306
- **PersistentVolumeClaim**: Persistent storage for MySQL data
- **Secret**: Stores the MySQL root password

The operator watches for MySQL custom resources and ensures the desired state matches the actual state in the cluster.

## Getting Started

### Local Development Setup

For local development and testing, you can use Kind (Kubernetes in Docker) to create a local cluster.

**1. Ensure Docker is running:**

If using Colima (macOS):
```sh
colima status
# If not running, start it:
colima start --cpu 6 --memory 12 --disk 100 --arch aarch64 --vm-type=vz --vz-rosetta
```

**2. Create a Kind cluster:**

```sh
kind create cluster --name mysql-operator-dev
```

**3. Verify the cluster is running:**

```sh
kubectl cluster-info
kubectl get nodes
```

**4. Install CRDs:**

```sh
make install
# Or manually:
kubectl apply -f config/crd/bases/
```

**5. Run the operator locally (for development):**

```sh
make run
```

This runs the operator on your local machine and connects to your Kind cluster. The operator will watch for MySQL resources and create the necessary Kubernetes resources.

**6. In another terminal, create a MySQL instance:**

```sh
kubectl apply -f config/samples/mysql-8.4.x.yaml
```

**7. Verify the MySQL instance was created:**

```sh
kubectl get mysqls
kubectl get deployments
kubectl get pods
```

**8. Monitor the status transition:**

Watch the MySQL status change from Pending to Running:
```sh
kubectl get mysql my-mysql-8.4.x -w
```

You should see the status transition as the MySQL pod starts up and becomes ready.

### Prerequisites
- go version v1.24.0+
- docker version 17.03+ (or Docker Desktop, Colima, etc.)
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster (or use Kind for local development)
- [Kind](https://kind.sigs.k8s.io/) (optional, for local development)

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/my-mysql-operator:tag
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
make deploy IMG=<some-registry>/my-mysql-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**

You can apply the samples (examples) from the config/samples:

```sh
kubectl apply -k config/samples/
```

**Example MySQL Custom Resource:**

```yaml
apiVersion: database.mycompany.com/v1alpha1
kind: MySQL
metadata:
  name: my-test-mysql
  namespace: default
spec:
  version: "8.0"              # MySQL version (e.g., "8.0", "8.4", "5.7")
  storageSize: "5Gi"            # Persistent storage size for MySQL data
  rootPassword: "mypassword"   # Optional: MySQL root password (defaults to "changeme")
```

>**NOTE**: Ensure that the samples have default values to test it out.

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
make build-installer IMG=<some-registry>/my-mysql-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/my-mysql-operator/<tag or branch>/dist/install.yaml
```

### MySQL Custom Resource Specification

The MySQL CRD supports the following spec fields:

- **version** (required): MySQL version to deploy (e.g., "8.0", "8.4", "5.7")
- **storageSize** (required): Size of persistent storage for MySQL data (e.g., "10Gi", "20Gi")
- **rootPassword** (optional): MySQL root user password. If not specified, defaults to "changeme"

#### Status Fields

The operator automatically updates the MySQL resource status based on the actual state of the Deployment and Pod:

- **phase**: Current state of the MySQL instance
  - `Pending`: Resources are being created or MySQL pod is starting up
  - `Running`: MySQL pod is running and ready to accept connections
  - `Failed`: MySQL pod has failed to start or crashed
- **ready**: Boolean indicating if MySQL is ready to accept connections (true when phase is Running)
- **message**: Detailed information about the current state (e.g., "MySQL instance is running and ready", "Pod is pending, waiting for resources")

**Monitoring Status:**

Watch the status in real-time:
```sh
kubectl get mysql <mysql-instance-name> -w
```

Check detailed status:
```sh
kubectl get mysql <mysql-instance-name> -o yaml
```

View status in a table format:
```sh
kubectl get mysql
# Output shows: NAME, PHASE, READY, AGE
```

**Status Lifecycle:**

When you create a MySQL instance, the status typically transitions:
1. `Pending` → Resources are being created (Deployment, PVC, Service, Secret)
2. `Pending` → Pod is starting up or waiting for resources
3. `Running` → Pod is ready and MySQL is accepting connections

If something goes wrong, the status will show `Failed` with a descriptive message.

## Troubleshooting

### kubectl Connection Issues

**Error:** `The connection to the server localhost:8080 was refused`

**Solution:** Ensure you have a valid Kubernetes context configured:

```sh
# Check current context
kubectl config get-contexts

# If no context is set, ensure your cluster is accessible
# For Kind clusters:
kubectl cluster-info --context kind-mysql-operator-dev

# Set the context if needed
kubectl config use-context kind-mysql-operator-dev
```

### Docker/Colima Not Running

**Error:** `Cannot connect to the Docker daemon` or `colima is not running`

**Solution:** Start your Docker runtime:

```sh
# For Colima (macOS):
colima status
colima start

# For Docker Desktop:
# Ensure Docker Desktop is running
```

### CRDs Not Found

**Error:** `No resources found` when running `kubectl get crds | grep mysql`

**Solution:** Install the CRDs:

```sh
make install
# Or manually:
kubectl apply -f config/crd/bases/
```

Verify installation:
```sh
kubectl get crds | grep mysql
# Should show: mysqls.database.mycompany.com
```

### Operator Not Creating Resources

**Issue:** MySQL CR is created but no Deployment/PVC/Service is created

**Solution:**
1. Ensure the operator is running: `make run` (for local development) or check operator pod status
2. Check operator logs for errors
3. Verify RBAC permissions are correctly installed
4. Ensure the MySQL CR spec has valid values (version and storageSize are required)

### Verification Commands

Use these commands to verify your setup:

```sh
# Check cluster status
kubectl cluster-info
kubectl get nodes

# Check CRDs
kubectl get crds | grep mysql

# Check MySQL instances and their status
kubectl get mysqls
kubectl get mysqls -o wide  # Shows more details

# Check created resources
kubectl get deployments
kubectl get services
kubectl get pvc
kubectl get secrets
kubectl get pods -l app=<mysql-instance-name>

# Check MySQL pod status
kubectl get pods -l app=<mysql-instance-name>
kubectl describe pod <mysql-pod-name>

# Check MySQL status details
kubectl get mysql <mysql-instance-name> -o yaml | grep -A 5 status

# Check operator logs (if deployed)
kubectl logs -n <operator-namespace> deployment/my-mysql-operator-controller-manager

# For local development (make run), check terminal output for reconciliation logs
```

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

