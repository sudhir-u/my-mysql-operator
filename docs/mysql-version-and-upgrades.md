# MySQL versioning and upgrades

**Audience:** Offering, Release, Support, Customer Success, and developers.  
**Scope:** Current operator generation (R6 channel pins + maintenance windows).  
**Architecture diagrams:** [architecture-and-flows.md](architecture-and-flows.md)

---

## 1. What `spec.version` means

| Mode | Example | Behavior |
|------|---------|----------|
| **Channel** | `8.4` | Operator resolves to a concrete patch via Offering-owned **channel pins**, then runs `mysql:<pin>`. |
| **Pin** | `8.4.13` | Uses that exact image tag. Ignores the channel pin map. |

Avoid `latest` (unbounded jumps).

`status.resolvedImage` shows the concrete image the operator is reconciling toward (e.g. `mysql:8.4.13`).

---

## 2. Vendor lookup (R6.1) — manual, not in the operator

There is **no** live call to Oracle or Docker Hub from the operator.

1. Offering/Release looks up the latest approved minor for each channel (vendor release notes / registry).
2. They edit [`config/mysql-channel-pins.yaml`](../config/mysql-channel-pins.yaml) (and keep [`config/manager/mysql_channel_pins.yaml`](../config/manager/mysql_channel_pins.yaml) in sync).
3. They open a PR, merge, and run the normal deploy/release pipeline.

That file is **not** Go/CRD core code; Offering and Release can own the PR.

---

## 3. How the instruction reaches the cluster (R6.1.1)

```text
Edit channel pins (PR) → merge → make deploy / release
        → ConfigMap my-mysql-operator-mysql-channel-pins
        → Operator reads pins on reconcile (and when ConfigMap changes)
```

Primary workflow is **git PR + deploy**, not hand-editing live ConfigMaps in production.

---

## 4. When existing instances upgrade (R6.2 / R6.3)

```text
Pin map changes (e.g. 8.4 → 8.4.14)
    → Operator wants STS image mysql:8.4.14 for CRs with spec.version: "8.4"
    → Only if now is inside spec.maintenanceWindow (UTC)
    → StatefulSet template updates → pods roll
    → PVC / volumeClaimTemplates unchanged → data retained
    → Downtime during rollout is expected (single replica)
```

| Situation | Behavior |
|-----------|----------|
| New provision (STS create) | Uses resolved pin image immediately (not gated by window). |
| Channel CR + pin map bump | Upgrade only **inside** `maintenanceWindow`. Outside: status message “deferred…”, requeue until window. |
| Channel CR + **no** `maintenanceWindow` | Pin-driven upgrades stay deferred until a window is set. |
| Customer sets explicit pin in Spec | Applied **immediately** (customer-initiated). Data still on PVC. |

Long-running pods do **not** self-upgrade only because the vendor published a release. Something must change the pin map (and hit a window) or Spec/pods must be rolled.

---

## 5. Data safety and downtime

| Concern | Behavior |
|---------|----------|
| Data loss on image bump | Not expected: same PVC templates; only container image changes. |
| Downtime | Accepted for single-replica rollout. |
| Informing customers | Use `spec.maintenanceWindow` so upgrades land in the agreed UTC window. |

Always take backups for production-like use.

---

## 6. Guidance by role

### Offering / Release

- Own [`config/mysql-channel-pins.yaml`](../config/mysql-channel-pins.yaml) via PR after vendor lookup.
- Keep the ConfigMap fragment under `config/manager/` aligned with that file.
- Communicate expected downtime and that channel instances need a maintenance window configured.

### Support

- Compare `spec.version`, `status.resolvedImage`, and `SELECT VERSION()` / pod image ID.
- “Deferred until maintenance window” in `status.message` means pin upgrade is waiting for the UTC window (or window unset).
- Explicit Spec pin change forces upgrade outside the pin-map gate.

### Customers

- Prefer channel + maintenance window for controlled patch currency.
- Use an explicit pin when you need a fixed build.
- Expect brief unavailability during StatefulSet rollout.

### Engineering

- No registry API polling for “latest.”
- Pin-driven path vs explicit Spec path as above; do not rewrite Spec.version when applying pins.

---

## 7. FAQ

**Q: When does vendor lookup happen?**  
A: Manually by Offering/Release before they edit the pin file. Not inside the operator.

**Q: When does the StatefulSet get a new DB version?**  
A: On create with resolved pin; on Spec pin change (immediate); on channel pin-map change **during** the instance’s maintenance window.

**Q: Will data be wiped?**  
A: Not by design for image rollouts; data lives on PVC.

**Q: Can customers be informed via a maintenance window?**  
A: Yes — set `spec.maintenanceWindow.start` / `end` (UTC `HH:MM`). Pin-driven upgrades only roll inside that window.
