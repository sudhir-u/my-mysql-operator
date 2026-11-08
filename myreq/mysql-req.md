# MySQL Operator — Requirements

> Pre-implementation requirements for specs-driven development (SDD).  
> Use this document to agree scope, generate plans, and define acceptance before coding.  
> For what was actually built, lessons, and code paths, see [`mysql-impl-facts.md`](mysql-impl-facts.md).

**Requirement states:** `Required` | `Optional` | `Proposed` | `Out of scope`

---

## 1. Product intent

Provide a Kubernetes operator that declaratively provisions MySQL database instances and reports their observed state via a Custom Resource (`MySQL`).

The operator is intended for local and development use, and as a foundation for future DBaaS-style capabilities. HA/failover is out of scope for the current generation (see §5).

### Non-goals

- Full DBaaS failover (primary/standby orchestration, ProxySQL/MySQL Router integration)
- MySQL-native replication managed by Orchestrator (or equivalent Classic-era topology tooling)
- Production-grade secrets management (Secret references, rotation, external secret stores)
- Cross-region storage HA and platform storage product choices

---

## 2. Functional requirements

### R1 — MySQL Custom Resource API

| ID | Requirement | State |
|----|-------------|-------|
| R1.1 | Expose a namespaced CRD for kind `MySQL` under a stable API group/version (e.g. `database.mycompany.com/v1alpha1`). | Required |
| R1.2 | `spec.version` (required): identifies the MySQL image tag to run (major.minor channel or explicit patch). Image form: `mysql:<version>`. | Required |
| R1.3 | `spec.storageSize` (required): size of persistent data volume (e.g. `2Gi`). | Required |
| R1.4 | `spec.rootPassword` (optional): root password material; if omitted, a documented demo default may be used. | Required (field optional) |
| R1.5 | Status must include at least: `phase`, `ready`, `message`. | Required |
| R1.6 | Invalid or incomplete Specs must be rejected by API validation. | Required |

**Acceptance (R1):**

- [ ] A valid `MySQL` manifest can be applied successfully.
- [ ] Missing required Spec fields are rejected.
- [ ] `kubectl get mysql` (or equivalent) can show phase and readiness.

---

### R2 — Provisioned cluster resources

When a `MySQL` CR exists, the operator must ensure equivalent owned resources exist and remain tied to the CR for lifecycle (create and delete).

| ID | Requirement | State |
|----|-------------|-------|
| R2.1 | A Secret holding the root password. | Required |
| R2.2 | A PersistentVolumeClaim sized per `spec.storageSize`. | Required |
| R2.3 | A workload that runs a single MySQL container (`mysql:<spec.version>`), port 3306, data directory on the PVC, password from the Secret. | Required |
| R2.4 | A ClusterIP Service exposing MySQL on port 3306. | Required |
| R2.5 | Child resource names must be DNS-compatible even when the CR name contains dots (sanitize consistently). | Required |
| R2.6 | Owner references (or equivalent) so deleting the CR cleans up owned children. | Required |
| R2.7 | Desired Spec for the MySQL workload image (and related pull policy once defined) must be reconciled onto the live workload—not create-once only. | Required |

**Acceptance (R2):**

- [ ] After apply: Secret, PVC, workload, and Service exist.
- [ ] MySQL pod becomes Ready.
- [ ] Clients can connect through the Service (e.g. port-forward to Service port 3306).
- [ ] Changing `spec.version` updates the workload image without requiring CR delete.
- [ ] Deleting the CR removes owned children (subject to PVC reclaim policy).

---

### R3 — Observed status

| ID | Requirement | State |
|----|-------------|-------|
| R3.1 | `phase` reflects real workload state: at least `Pending`, `Running`, `Failed`. | Required |
| R3.2 | `ready` is true only when MySQL is ready to accept connections (per pod readiness). | Required |
| R3.3 | `message` provides a short human-readable explanation of the current state. | Required |
| R3.4 | Status updates must remain correct under concurrent updates (no permanent stuck state due to conflicts). | Required |

**Acceptance (R3):**

- [ ] Status transitions Pending → Running as the instance becomes ready.
- [ ] Failure conditions surface as `Failed` (or equivalent) with a useful message.
- [ ] Rapid reconcile / concurrent status writers do not leave status permanently wrong.

---

### R4 — Operator runtime

| ID | Requirement | State |
|----|-------------|-------|
| R4.1 | Operator can run against a local Kubernetes cluster for development. | Required |
| R4.2 | Operator can be deployed in-cluster and continue to reconcile. | Required |
| R4.3 | CRDs can be installed independently of (or as part of) operator deploy. | Required |

**Acceptance (R4):**

- [ ] Creating a `MySQL` CR is reconciled in both local-run and in-cluster modes.

---

### R5 — Developer enablement

| ID | Requirement | State |
|----|-------------|-------|
| R5.1 | Documented path to run a local cluster, install CRDs, run or deploy the operator, and apply sample CRs. | Required |
| R5.2 | At least one sample manifest for a current LTS-oriented MySQL line (e.g. 8.4 channel). | Required |
| R5.3 | Documented way to connect a MySQL client to a provisioned instance. | Required |

**Acceptance (R5):**

- [ ] A developer following the docs can provision an instance and run a simple SQL check (e.g. `SELECT VERSION()`).

---

### R6 — Upgrade to latest MySQL version safely

| ID | Requirement | State |
|----|-------------|-------|
| R6.1 | At any time, we will require to look up the vendor for latest minor versions for the respective major versions, and keep handy.  | Required |
| R6.2 | Any existing deployments will require to be upgraded to the available/downloaded latest versions from the vendor. In no way, the underlying data could be lost. Downtime is accepted.| Required |
| R6.3 | A concept of maintenance window will have to introduced so that in future upgrades, the customers can be informed. | Required |

**Acceptance (R5):**

- [ ] Provisioning an instance will always be in the latest minor version. 
- [ ] An existing instance be upgraded to the latest available minor version. 
- [ ] After a maintenance window is introduced, An existing instance be upgraded only during the maintenance window. 

---

## 3. Constraints and policies

| ID | Constraint | State |
|----|------------|-------|
| C1 | Workload is a single replica for this generation (HA topology out of scope). | Required |
| C2 | `spec.version` may be a **channel** (e.g. `8.4`) or a **pin** (e.g. `8.4.13`). Semantics must be documented. | Required |
| C3 | Prefer not to use unbounded `latest` as the product default. | Required |
| C4 | Plain `rootPassword` on Spec is acceptable for demos only; production Secret references are a later generation. | Required |
| C5 | Image registry reachability (and local Docker/VM runtime for Kind) is an operational prerequisite, not an operator feature. | Required |

---

## 4. Proposed requirements — sustainable upstream image currency

> **State:** `Proposed` — review and adjust before plan/implementation.  
> **Intent:** New pods and Spec-driven rollouts get the current upstream image for the chosen version channel, without depending on a stale node image cache.

### P1 — Pull policy for currency

**Requirement:** The MySQL container MUST use `imagePullPolicy: Always` when relying on floating channel tags (and may use Always generally for this product generation).

**Acceptance:**

- [ ] Workload pod template sets `imagePullPolicy: Always` for the MySQL container.
- [ ] With registry reachable, recreating the pod can resolve a newer digest for the same tag; `SELECT VERSION()` matches the pulled build (or unchanged if upstream tag unchanged).

---

### P2 — Reconcile workload to Spec

**Requirement:** If the live workload image and/or pull policy differ from desired Spec/policy, the operator MUST update the workload so a rollout can occur—without manual image patches or CR deletion.

**Acceptance:**

- [ ] Changing `spec.version` updates the workload image to `mysql:<newVersion>`.
- [ ] Introducing Always pull on an operator upgrade updates existing workloads on reconcile.
- [ ] Automated test covers “workload updated when desired image changes.”

**Out of scope for this proposal:**

- Auto-resize of PVC
- Migration to StatefulSet
- Continuous registry polling / digest watcher without Spec or rollout trigger

---

### P3 — Version semantics documentation

**Requirement:** Document channel vs pin. Optionally default new CRs to an LTS channel (e.g. `8.4`) if product agrees.

| Mode | Spec value | Expected behavior |
|------|------------|-------------------|
| Channel | `"8.4"` | Image `mysql:8.4`; with Always pull → current 8.4.x at pull time |
| Pinned | `"8.4.13"` | Image `mysql:8.4.13` → reproducible |
| Avoid | `"latest"` | Unbounded line/major jumps |

**Acceptance:**

- [ ] API docs / README / this requirements doc agree on channel vs pin.
- [ ] Samples use an intentional channel or pin.
- [ ] (Optional) CRD default for `version` if approved.

---

### P4 — Non-requirements for this proposal

- No requirement to call Docker Hub (or other registry APIs) to discover “latest” tags.
- No requirement for timer-based auto-upgrade without pod recreation / rollout.
- No requirement to wipe or migrate PVC data on minor patch bumps within a line.

---

## 5. Future (out of scope for current generation)

Brief pointer only; detailed Gen2/HA design belongs in a separate requirements doc when prioritized.

- Storage-level replication may change how instance HA is designed versus Classic MySQL replication + Orchestrator.
- Primary/standby, split-brain prevention, and a routing layer (e.g. ProxySQL / MySQL Router) are candidate future epics.
- This generation remains **provisioning + status**, not a failover controller.

---

## 6. How to use this document

1. Agree §1–§3 as the baseline generation contract.
2. Review and lock §4 (P1–P4) before generating an implementation plan.
3. Implement against acceptance checkboxes; record delivery facts and deviations in [`mysql-impl-facts.md`](mysql-impl-facts.md).
4. Add new proposed sections for future goals; do not mix unscoped HA work into §4 without a new requirements section.
