# my-mysql-operator

## Description

The my-mysql-operator is a Kubernetes operator that simplifies the deployment and management of MySQL database instances on Kubernetes clusters. The operator uses Custom Resource Definitions (CRDs) to allow users to declaratively define MySQL instances, and automatically creates and manages the necessary Kubernetes resources including:

- **Deployment**: MySQL container running the specified version
- **Service**: ClusterIP service exposing MySQL on port 3306
- **PersistentVolumeClaim**: Persistent storage for MySQL data
- **Secret**: Stores the MySQL root password

The operator watches for MySQL custom resources and ensures the desired state matches the actual state in the cluster.

## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

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

The operator updates the MySQL resource status with:

- **phase**: Current state of the MySQL instance (Pending, Running, Failed)
- **ready**: Boolean indicating if MySQL is ready to accept connections
- **message**: Additional information about the current state

You can check the status using:

```sh
kubectl get mysql <mysql-instance-name> -o yaml
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

