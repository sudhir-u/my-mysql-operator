# Architecture and flows

**Audience:** Engineering walkthroughs, Offering/Support briefings, planning for automation and modularization.  
**Scope:** What exists in `my-mysql-operator` after R6 (channel pins + maintenance windows), plus pictorial “next steps.”  
**Related:** [mysql-version-and-upgrades.md](mysql-version-and-upgrades.md), [myreq/mysql-req.md](../myreq/mysql-req.md)

Diagrams use [Mermaid](https://mermaid.js.org/) (render in GitHub, many IDEs, and Mermaid Live Editor).

---

## 1. System context (today)

What surrounds the operator and how people / systems interact with it.

```mermaid
flowchart TB
  subgraph actors [Actors]
    Dev[Developer]
    Offering[Offering_Release]
    Support[Support]
    Customer[Customer_or_CR_owner]
  end

  subgraph git [GitHub]
    Repo[my-mysql-operator_repo]
    PinFile[config_mysql-channel-pins.yaml]
    GoCode[Go_controller_and_CRD]
  end

  subgraph cluster [Kubernetes_cluster]
    OpNS[my-mysql-operator-system]
    OpDeploy[Operator_Deployment]
    PinCM[ConfigMap_channel_pins]
    MySQLCR[MySQL_CustomResource]
    Workload[StatefulSet_Services_Secret_PVC]
    Registry[Image_registry_mysql_tags]
  end

  Dev -->|code_PRs| GoCode
  Offering -->|pin_PRs_only| PinFile
  PinFile --> Repo
  GoCode --> Repo
  Repo -->|make_deploy| OpDeploy
  Repo -->|make_deploy| PinCM
  Customer -->|kubectl_apply_MySQL| MySQLCR
  Support -->|kubectl_get_describe| MySQLCR
  OpDeploy -->|reconcile| MySQLCR
  OpDeploy -->|read| PinCM
  OpDeploy -->|create_update| Workload
  Workload -->|pull_image| Registry
```

---

## 2. Repository and runtime layout (today)

```mermaid
flowchart LR
  subgraph repo [Repository]
    cmd[cmd_main.go]
    api[api_v1alpha1]
    ctrl[internal_controller]
    pins[config_mysql-channel-pins.yaml]
    kustomize[config_default_kustomize]
    docs[docs]
  end

  subgraph runtime [In_cluster]
    Mgr[controller-runtime_Manager]
    Rec[MySQLReconciler]
    CM[ConfigMap_pins]
    WatchCR[Watch_MySQL]
    WatchCM[Watch_ConfigMap]
    WatchOwn[Own_STS_Svc_Secret]
  end

  cmd --> Mgr
  api --> Rec
  ctrl --> Rec
  pins --> kustomize
  kustomize --> CM
  Mgr --> Rec
  Rec --> WatchCR
  Rec --> WatchCM
  Rec --> WatchOwn
  Rec --> CM
```

| Area | Responsibility today |
|------|----------------------|
| `api/v1alpha1` | MySQL Spec/Status (version, storage, replicas, maintenanceWindow, status fields) |
| `internal/controller` | **Single** reconciler: provision + pins + window + failover labels + status |
| `config/mysql-channel-pins.yaml` | Offering-owned channel → patch map |
| `config/default` | Deploy operator, RBAC, CRDs, pins ConfigMap |

---

## 3. Owned cluster objects (per MySQL CR)

```mermaid
flowchart TB
  CR[MySQL_CR]

  CR -->|owns| Secret[Secret_root_password]
  CR -->|owns| Headless[Service_headless]
  CR -->|owns| STS[StatefulSet]
  CR -->|owns| Svc[Service_primary_selector]
  STS -->|volumeClaimTemplates| PVC[PVC_per_pod]
  STS -->|pods| Pod0[Pod_ordinal_0]
  STS -->|pods| Pod1[Pod_ordinal_1_optional]
  Svc -->|selects_mysql-role_primary| Pod0
  Pins[ConfigMap_channel_pins] -.->|read_by_operator_not_owned| CR
```

Naming: CR name is sanitized (`.` → `-`). Example: `my-mysql-8.4.x` → STS `my-mysql-8-4-x`, Service `my-mysql-8-4-x-service`.

---

## 4. Reconcile flow (today)

End-to-end path when a MySQL CR or pins ConfigMap changes.

```mermaid
flowchart TD
  Start[Reconcile_triggered] --> GetCR[Get_MySQL_CR]
  GetCR -->|not_found| EndOK[Return_nil]
  GetCR --> LoadPins[Load_channel_pins_ConfigMap]
  LoadPins --> Resolve[ResolveImageTag_channel_or_pin]
  Resolve --> Secret[Ensure_Secret]
  Secret --> Headless[Ensure_headless_Service]
  Headless --> STSBranch{StatefulSet_exists?}

  STSBranch -->|no| CreateSTS[Create_STS_with_resolved_image]
  STSBranch -->|yes| PinDriven{Pin_driven_image_change?}

  PinDriven -->|no_or_explicit_Spec_pin| UpdateSTS[reconcileStatefulSet]
  PinDriven -->|yes| Window{In_maintenanceWindow?}
  Window -->|no_or_unset| Defer[Status_deferred_RequeueAfter]
  Window -->|yes| UpdateSTS

  CreateSTS --> PrimSvc[Ensure_primary_Service]
  UpdateSTS --> PrimSvc
  Defer --> EndDefer[Return_RequeueAfter]

  PrimSvc --> Failover[ensurePrimaryAndFailover]
  Failover --> Status[updateMySQLStatus]
  Status --> EndOK2[Return_nil]
```

### Image resolution detail

```mermaid
flowchart LR
  SpecV[spec.version] --> Chan{Is_channel_X.Y?}
  Chan -->|yes| Map{Pin_in_ConfigMap?}
  Chan -->|no| UseSpec[Use_Spec_as_tag]
  Map -->|yes| UsePin[Use_pinned_tag]
  Map -->|no| UseSpec
  UsePin --> Img["mysql:tag"]
  UseSpec --> Img
```

### Pin-driven vs customer-driven upgrade

```mermaid
flowchart TB
  subgraph pinDriven [Pin_driven_Offering]
    A1[ConfigMap_pin_bump] --> A2[Channel_CR_unchanged]
    A2 --> A3[Only_inside_maintenanceWindow]
    A3 --> A4[STS_image_update]
  end

  subgraph customerDriven [Customer_driven]
    B1[Spec.version_set_to_explicit_pin] --> B2[Immediate_STS_update]
    B2 --> B3[No_window_gate]
  end

  A4 --> PVC[PVC_data_retained]
  B3 --> PVC
```

---

## 5. Offering pin lifecycle (today)

```mermaid
sequenceDiagram
  participant Off as Offering_Release
  participant GH as GitHub
  participant Pipe as Deploy_make_deploy
  participant CM as ConfigMap
  participant Op as Operator
  participant STS as StatefulSet

  Off->>Off: Manual_vendor_lookup
  Off->>GH: PR_edit_pin_YAML_only
  GH->>GH: Merge
  Pipe->>CM: Apply_ConfigMap
  CM->>Op: Watch_enqueue_all_MySQL
  Op->>Op: Resolve_and_check_window
  alt Inside_window
    Op->>STS: Update_image
    STS->>STS: Roll_pods
  else Outside_window
    Op->>Op: Status_deferred_requeue
  end
```

---

## 6. Concerns mixed in one reconciler (today — maintainability view)

Useful for explaining **why** to modularize later.

```mermaid
flowchart TB
  Rec[MySQLReconciler]

  Rec --> C1[Provisioning_Secret_STS_Services]
  Rec --> C2[Version_pins_and_image_resolve]
  Rec --> C3[Maintenance_window_gating]
  Rec --> C4[Failover_primary_labels]
  Rec --> C5[Status_aggregation]
  Rec --> C6[ConfigMap_watch_mapping]
```

| Concern | Today | Pain if it grows |
|---------|--------|------------------|
| Provisioning | Same file as everything else | Hard to test STS builders in isolation |
| Version / pins | `version_pins.go` helpers + reconcile gates | Still coupled to STS update path |
| Maintenance window | Inline in Reconcile | Policy changes touch main loop |
| Failover | `ensurePrimaryAndFailover` in same reconciler | HA will dominate file size / RBAC |
| Status | Large `updateMySQLStatus` | Competing status writers / messages |
| Offering config | ConfigMap watch | Fine for now; automation later |

---

## 7. Target modular shape (next — separation of concerns)

Proposed package / controller boundaries (not implemented yet).

```mermaid
flowchart TB
  subgraph api [api_v1alpha1]
    Types[MySQL_types]
  end

  subgraph modules [internal]
    Prov[provisioning_STS_Secret_Svc]
    Ver[version_PinResolver_interface]
    Win[maintenance_WindowPolicy]
    Fail[failover_PrimaryElector]
    Stat[status_Reporter]
  end

  subgraph controllers [optional_split_controllers]
    MainRec[MySQLReconciler_orchestrator]
    PinRec[PinsConfigReconciler_optional]
  end

  Types --> MainRec
  MainRec --> Prov
  MainRec --> Ver
  MainRec --> Win
  MainRec --> Fail
  MainRec --> Stat
  PinRec --> Ver
```

**Suggested interfaces (illustrative):**

```text
PinResolver.Resolve(specVersion) (tag string, pinDriven bool, err)
WindowPolicy.AllowUpgrade(now, window) (ok bool, requeueAfter time.Duration)
Provisioner.Ensure(ctx, mysql, imageTag) error
PrimaryElector.Ensure(ctx, mysql) error
StatusReporter.Update(ctx, mysql, facts) error
```

Benefits: unit-test policies without envtest; Offering automation plugs into `PinResolver` without touching STS code; failover can become its own controller later.

---

## 8. Automation roadmap (pictorial)

From today’s **manual vendor lookup + PR** toward safer automation.

```mermaid
flowchart LR
  subgraph now [Now]
    M1[Human_vendor_lookup]
    M2[PR_pin_YAML]
    M3[Deploy_ConfigMap]
  end

  subgraph near [Near_term]
    N1[CI_job_suggests_new_pins]
    N2[Human_approves_PR]
    N3[Same_deploy_path]
  end

  subgraph later [Later]
    L1[Approved_catalog_service]
    L2[Signed_pin_updates]
    L3[Per_tenant_rollout_waves]
  end

  M1 --> M2 --> M3
  M3 -.-> N1
  N1 --> N2 --> N3
  N3 -.-> L1
  L1 --> L2 --> L3
```

| Stage | What is automated | What stays human |
|-------|-------------------|------------------|
| **Now** | Reconcile, window gate, STS roll | Vendor lookup, pin PR, merge |
| **Near** | Detect newer tags / open draft PR | Approve pin PR, release timing |
| **Later** | Catalog, progressive rollout | Policy / exception approval |

**Constraint:** Do not skip the pin file / PR gate for production-like offerings unless product explicitly accepts fully automatic upgrades.

---

## 9. Walkthrough script (use with diagrams above)

1. **Context** — §1: actors, Git, cluster.  
2. **CR → objects** — §3: what a MySQL instance materializes.  
3. **Happy path create** — §4: reconcile without pin change.  
4. **Offering bump** — §5: edit pins → ConfigMap → window → roll.  
5. **Deferral** — §4: outside window / missing window.  
6. **Limits today** — §6: one reconciler, many concerns.  
7. **Where we’re going** — §7–§8: modules + automation stages.

---

## 10. Quick reference — code map

| Topic | Location |
|-------|----------|
| Entry / manager | `cmd/main.go` |
| API | `api/v1alpha1/mysql_types.go` |
| Reconcile + STS + failover + status | `internal/controller/mysql_controller.go` |
| Pins / window helpers | `internal/controller/version_pins.go` |
| Offering pin source | `config/mysql-channel-pins.yaml` |
| Deployed ConfigMap | `config/manager/mysql_channel_pins.yaml` |
| Version/upgrade narrative | `docs/mysql-version-and-upgrades.md` |

---

## 11. Document history

| Date | Note |
|------|------|
| 2026-11-08 | Initial architecture & flows after R6 merge (PR #1). |
