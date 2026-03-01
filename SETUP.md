# New Laptop Setup — MySQL Operator & Local Cluster

Use this guide to get the MySQL operator and a local Kind cluster running on a fresh machine (macOS).

## 1. Prerequisites

Install these before bringing up the cluster.

| Tool      | Version / Notes |
|-----------|-----------------|
| **Go**    | 1.24.0+ (required by `go.mod`) |
| **Docker**| 17.03+ or **Colima** (recommended on macOS) |
| **kubectl** | 1.11.3+ |
| **Kind**  | For local Kubernetes cluster |

### Install via Homebrew (macOS)

```bash
# Go (1.24+)
brew install go

# kubectl
brew install kubectl

# Kind (Kubernetes in Docker)
brew install kind
```

### Docker runtime: Colima (recommended on macOS)

```bash
brew install colima docker
colima start --cpu 4 --memory 8 --disk 60
# Optional, for Apple Silicon with Rosetta:
# colima start --cpu 6 --memory 12 --disk 100 --arch aarch64 --vm-type=vz --vz-rosetta
```

Ensure Docker is available:

```bash
docker info
```

### Verify versions

```bash
go version    # go1.24.x or higher
kubectl version --client
kind version
docker info   # or: colima status
```

---

## 2. One-time project setup

From the repo root:

```bash
cd /Users/sudhir/myworkspace/my-mysql-operator

# Download Go modules
go mod download

# Optional: install CRDs and build (Makefile will pull controller-gen, kustomize, etc.)
make install
```

---

## 3. Bring up the MySQL cluster (local dev)

### Option A: Use the setup script

```bash
./scripts/setup.sh
```

This script checks prerequisites, creates a Kind cluster, installs CRDs, and optionally runs the operator and creates a sample MySQL instance.

### Option B: Manual steps

**1. Start Docker (if using Colima)**

```bash
colima status || colima start
```

**2. Create Kind cluster**

```bash
kind create cluster --name mysql-operator-dev
kubectl cluster-info
kubectl get nodes
```

**3. Install CRDs**

```bash
make install
kubectl get crds | grep mysql
```

**4. Run the operator (terminal 1)**

```bash
make run
```

**5. Create a MySQL instance (terminal 2)**

```bash
kubectl apply -f config/samples/mysql-8.4.x.yaml
```

**6. Check status**

```bash
kubectl get mysqls
kubectl get pods
kubectl get mysql my-mysql-8.4.x -w
```

---

## 4. Quick reference

| Goal              | Command |
|-------------------|--------|
| Create Kind cluster | `kind create cluster --name mysql-operator-dev` |
| Install CRDs      | `make install` |
| Run operator locally | `make run` |
| Create sample MySQL | `kubectl apply -f config/samples/mysql-8.4.x.yaml` |
| Delete cluster    | `kind delete cluster --name mysql-operator-dev` |

---

## 5. Troubleshooting

- **Docker not running**: Start Colima with `colima start` or start Docker Desktop.
- **`kubectl` connection refused**: Ensure the Kind cluster exists and context is correct: `kubectl config use-context kind-mysql-operator-dev`.
- **CRDs not found**: Run `make install` from the repo root.
- **Operator not creating resources**: Ensure `make run` is running and that the MySQL CR has valid `spec.version` and `spec.storageSize`.

For more detail, see [README.md](README.md#troubleshooting).

---

## 6. Next steps (after cluster is running)

Once `kubectl get mysqls` shows **Running** and **Ready**, try these:

### 6.1 Verify MySQL is reachable

Port-forward and connect with a MySQL client:

```bash
# Terminal: port-forward the MySQL service (service name = <sanitized-cr-name>-service)
kubectl port-forward svc/my-mysql-8-4-x-service 3306:3306
```

In another terminal, install the MySQL client if needed, then connect:

```bash
brew install mysql-client
# Add to PATH for this session (Apple Silicon): export PATH="/opt/homebrew/opt/mysql-client/bin:$PATH"
mysql -h 127.0.0.1 -P 3306 -u root -p
# Password from the sample: mysecretnewpassword (see config/samples/mysql-8.4.x.yaml)
```

Then run `SELECT 1;` or `SHOW DATABASES;` to confirm the DB works.

### 6.2 Run e2e tests

Uses a separate Kind cluster, runs tests, then tears it down:

```bash
make test-e2e
```

### 6.3 Deploy the operator in-cluster

Instead of `make run` on your laptop, run the operator inside the cluster (closer to production):

```bash
make docker-build IMG=my-mysql-operator:local
kind load docker-image my-mysql-operator:local --name mysql-operator-dev
make deploy IMG=my-mysql-operator:local
```

Then you can stop `make run`; the operator will keep reconciliating from inside the cluster.

### 6.4 Create another MySQL instance

Apply the 8.0 sample and watch both:

```bash
kubectl apply -f config/samples/mysql-8.0.x.yaml
kubectl get mysqls
kubectl get pods
```
