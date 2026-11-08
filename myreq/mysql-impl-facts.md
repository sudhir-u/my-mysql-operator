# MySQL Operator — Implementation Facts & Reference

> **Purpose:** Post-implementation reference — what exists in the repo and cluster, lessons learned, and code traceability.  
> **Not** the pre-implementation requirements source of truth. For that, see [`mysql-req.md`](mysql-req.md).

**Captured:** 2026-11-08  
**Repo:** `github.com/sudhir-u/my-mysql-operator`  
**Stack in use:** Kubebuilder v4, controller-runtime, Kind (`mysql-operator-dev`), Colima/Docker

**Status legend (delivery tracking):** `Implemented` | `Partial` | `Proposed` | `Out of scope (for now)`

---

## 1. Product intent (as delivered)

Kubernetes operator that declaratively provisions and reports status for MySQL instances via Custom Resource `MySQL`, suitable for local/dev learning and as a base for future DBaaS-style capabilities.

### Non-goals (current generation)

- Full DBaaS failover / primary–standby / ProxySQL integration (discussed; not implemented)
- MySQL-native replication + Orchestrator (Classic-era pattern; not the Gen2 target)
- Production-grade secrets management (password currently optional plain field on CR)
- Cross-region block storage HA (platform concern; outside this operator today)

---

## 2. Features as implemented

### F1 — MySQL Custom Resource API

| Field | Status | Spec |
|-------|--------|------|
| API group/version | Implemented | `database.mycompany.com/v1alpha1`, kind `MySQL` (namespaced) |
| `spec.version` | Implemented | Required. MySQL image tag channel (e.g. `"8.0"`, `"8.4"`). Maps to image `mysql:<version>`. |
| `spec.storageSize` | Implemented | Required. PVC request size (e.g. `"2Gi"`). |
| `spec.rootPassword` | Implemented | Optional. Stored in a Secret; default `"changeme"` if omitted. |
| Status subresource | Implemented | `status.phase`, `status.ready`, `status.message` |
| Printer columns | Implemented | Phase, Ready, Age |

**Acceptance (observed):** Valid `MySQL` CRs are accepted; required fields enforced via CRD OpenAPI.

**Primary code:** `api/v1alpha1/mysql_types.go`, `config/crd/bases/`

---

### F2 — Child resource provisioning

On reconcile, the operator ensures these owned resources exist:

| Child | Name pattern | Status |
|-------|--------------|--------|
| Secret | `<sanitizedName>-secret` | Implemented (create-if-missing) |
| PVC | `<sanitizedName>-pvc` | Implemented (create-if-missing) |
| Deployment | `<sanitizedName>` | Implemented (create-if-missing) |
| Service | `<sanitizedName>-service` | Implemented (create-if-missing) |

**Rules (as coded):**

- Resource names are DNS-safe: CR name dots → dashes (`sanitizeName`). Example: CR `my-mysql-8.4.x` → Deployment `my-mysql-8-4-x`, Service `my-mysql-8-4-x-service`.
- Owner references set so GC can cascade on CR delete.
- MySQL container: image `mysql:<spec.version>`, port 3306, data at `/var/lib/mysql` via PVC, `MYSQL_ROOT_PASSWORD` from Secret.
- Single replica Deployment (not StatefulSet).

**Update (2026-11-08):** StatefulSet image and `imagePullPolicy` are reconciled on Spec change (`reconcileStatefulSet`). MySQL container uses `imagePullPolicy: Always`. PVC/volumeClaimTemplates remain create-time only.

**Update (R6):** Channel pins live in `config/mysql-channel-pins.yaml` → ConfigMap `my-mysql-operator-mysql-channel-pins`. Operator resolves channels via `ResolveImageTag`, watches the ConfigMap, gates pin-driven STS upgrades with `spec.maintenanceWindow`, sets `status.resolvedImage`. Stakeholder doc: `docs/mysql-version-and-upgrades.md`.

**Primary code:** `internal/controller/mysql_controller.go` (`Reconcile`, `*ForMySQL` builders)

---

### F3 — Observed status from real workload state

| Phase | Meaning | Status |
|-------|---------|--------|
| `Pending` | Creating / starting / not ready | Implemented |
| `Running` | Pod ready and accepting work (per readiness) | Implemented |
| `Failed` | Pod failed / terminated with message | Implemented |
| `status.ready` | `true` when Running and ready | Implemented |
| `status.message` | Human-readable detail | Implemented |

**Rules (as coded):**

- Status derived from Deployment replica readiness and Pod phase/conditions (not hardcoded Running).
- Status updates use **retry-on-conflict** (`statusUpdateWithRetry` / `k8s.io/client-go/util/retry`) to avoid resourceVersion races.

**Primary code:** `updateMySQLStatus`, `statusUpdateWithRetry`

---

### F4 — Operator runtime & deploy modes

| Capability | Status |
|------------|--------|
| Local run (`make run`) against Kind | Implemented |
| In-cluster deploy (`make deploy IMG=…`) | Implemented |
| CRD install (`make install`) | Implemented |
| Leader election flag / metrics / probes (scaffold) | Implemented (scaffold defaults) |
| Watches | Implemented: For `MySQL`; Owns Deployment, Service, PVC, Secret |

**Acceptance (observed):** Operator reconciles when run locally or as Deployment in `my-mysql-operator-system`.

**Primary code:** `cmd/main.go`, `config/default`, `Makefile`

---

### F5 — Local developer experience

| Capability | Status |
|------------|--------|
| Kind cluster `mysql-operator-dev` workflow | Implemented (docs + script) |
| `SETUP.md` prerequisites (Go 1.24+, Colima/Docker, kubectl, Kind) | Implemented |
| `scripts/setup.sh` (deps check, Kind, CRDs) | Implemented |
| Samples `mysql-8.0.x`, `mysql-8.4.x` | Implemented |
| Documented port-forward + mysql client connect | Implemented |

**Live cluster note (2026-11-08):** Context `kind-mysql-operator-dev`; CRs `my-mysql-8.0.x` and `my-mysql-8.4.x` Running; operator Deployment in `my-mysql-operator-system`. Pod for 8.4 started ~2026-03-01, `restartCount: 0`, image tag `mysql:8.4`, `SELECT VERSION()` → `8.4.8`.

---

### F6 — Gaps / lessons captured

| ID | Observation | Implication |
|----|-------------|-------------|
| L1 | `spec.version: "8.4"` is a **floating tag channel**, not a frozen patch | Running `SELECT VERSION()` may show e.g. `8.4.8` while CR still says `8.4` |
| L2 | Floating tags only advance on **image pull + new container** | Long-lived pods with `restartCount: 0` keep the digest from first pull |
| L3 | Create-only Deployment reconcile | Changing Spec alone does not update image |
| L4 | Default `ImagePullPolicy` (IfNotPresent) + node cache | New pods may reuse stale `mysql:8.4` layers |
| L5 | Password on CR Spec | Demo-only; production should use Secret refs |

**Historical note:** MySQL 8.4.8 released 2026-01-20; this cluster’s 8.4 pod was created 2026-03-01. No in-place “jump” later — first pull of floating `mysql:8.4` already resolved to 8.4.8. Newer upstream 8.4.x patches after that date are not in the running container until re-pull/rollout.

---

## 3. Platform / architecture notes (context, not coded)

Captured for future Gen2 / IBM Cloud DBaaS discussions; **not** implemented in this repo:

- Storage-level replication (RFS / future block) reduces reliance on MySQL replication for HA.
- Prefer primary/standby + routing (ProxySQL / MySQL Router) + split-brain prevention over returning to Orchestrator + DB replication for Gen2.
- K8s Lease alone is disputed for large-DB primary election; consider stronger consensus/witness patterns.
- This operator today = **instance provisioning + status**, not failover controller.

---

## 4. Proposed next work (mirrors `mysql-req.md` §4) — implementation sketch

Intent: new deployments / Spec-driven rollouts get current upstream image for the chosen version channel.

| ID | Requirement (summary) | Suggested code touchpoints |
|----|----------------------|----------------------------|
| G1 | `imagePullPolicy: Always` on mysql container | `deploymentForMySQL` |
| G2 | Update existing Deployment when desired image/pull policy differs | `Reconcile` (end create-only for image) |
| G3 | Document channel vs pin; optional CRD default `8.4` | types comments, README, samples, `make manifests` |
| G4 | Non-requirements: no registry API poller, no timer auto-upgrade, no PVC wipe | — |

**Verification sketch:** rebuild/redeploy operator → reconcile → optional pod delete → `SELECT VERSION()`.

**Commit theme:** Ensure Deployments pull current upstream images for floating tags; update on reconcile.

---

## 5. Traceability map

| Spec ID | Code / docs |
|---------|-------------|
| F1 | `api/v1alpha1/mysql_types.go` |
| F2–F3 | `internal/controller/mysql_controller.go` |
| F4 | `cmd/main.go`, `Makefile`, `config/` |
| F5 | `SETUP.md`, `scripts/setup.sh`, `config/samples/` |
| G1–G2 | (proposed) `deploymentForMySQL`, `Reconcile` |
| G3 | (proposed) types comments, README, samples |

---

## 6. Related documents

| Document | Role |
|----------|------|
| [`mysql-req.md`](mysql-req.md) | Pre-implementation style requirements (SDD planning) |
| This file | Facts, gaps, code map after delivery |
| `SETUP.md` / `README.md` | Operator how-to |
