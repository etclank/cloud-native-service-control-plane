# SmartEnergy admission record

SmartEnergy is deployed and validated for its portfolio/demo scope. This record captures its immutable application and image revisions, completed Stage 7 checks, and accepted limitations.

| Item | Admitted value |
| --- | --- |
| Deployment revision | `6dee39d597ce87620f4f12b17ea0bdc079d029f0` |
| Application image source revision | `5961af5ce36d3aed2fa81ac85da107627e9b2f35` |
| Application image digest | `sha256:0d13398c342931d23d726b4db1903508272e45fefc27687f9cfe311798a4e363` |
| Repository | `https://github.com/etclank/smartenergy-api.git` |
| Production overlay | `deploy/kubernetes/overlays/production` |
| Namespace / AppProject | `smartenergy` / `smartenergy` |
| Hostname | `energy.platform.eoghanclancy.eu` |
| Sync policy | Manual; automated sync, pruning, and self-healing disabled |
| Current state | Live; final closeout revision prepared for manual synchronization |

Project 1 owns only the Argo Application, namespace policy, AppProject, quota, and platform-side Prometheus configuration. SmartEnergy owns all rendered application workloads and namespaced runtime resources.

## Manual prerequisites

Provision the three runtime Secrets using the [value-free runbook](smartenergy-secret-provisioning.md). No Secret value belongs in Git.

Off-node backup and disaster recovery are optional future improvements and are not part of the current production desired state. PostgreSQL persistence across Pod replacement is validated; node or PVC loss has no recovery path. The deployment does not claim HA or disaster-recovery completeness.

DNS for `energy.platform.eoghanclancy.eu` points to the current Hetzner endpoint and public HTTPS validation passed after the internal workloads became healthy.

The SmartEnergy Certificate uses `ClusterIssuer/letsencrypt-production`, matching Project 1's production issuer.

## Capacity baseline status

Stage 7 began at approximately 61% node memory and completed at approximately 75%, with memory, disk, and PID pressure false. PostgreSQL, Redis, API, worker, Beat, Prometheus, TLS, networking, controlled rollout, and Pod-restart persistence passed on the existing node.

## Stage 7 checkpoints

1. **Baseline:** record node and platform utilization before SmartEnergy exists.
2. **PostgreSQL:** require Ready, Bound 5 GiB PVC, successful `pg_isready`, stable memory, and clean events.
3. **Redis:** require Ready, authentication, AOF enabled, Bound 1 GiB PVC, stable memory, and clean events.
4. **Migration:** require the Job to succeed at Alembic head without schema errors.
5. **API:** require liveness, dependency-aware readiness, internal metrics, stable memory, and healthy platform APIs.
6. **Worker:** require broker connection, concurrency one, one deterministic task, and stable memory.
7. **Beat:** require one instance, only the three approved schedules, and stable memory.
8. **Prometheus:** require the SmartEnergy target Up while the existing six targets remain Up.
9. **Ingress/TLS/DNS:** verify public health, login, dashboard, intended read API, private metrics, and blocked documentation routes.
10. **Controlled restart:** require no OOM, no pressure, and persistence across expected stateful Pod replacement.

At every checkpoint record:

```bash
kubectl top nodes
kubectl top pods -A
kubectl describe node portfolio-k3s-01
kubectl get pods -n smartenergy -o wide
kubectl get pvc -n smartenergy
kubectl get events -n smartenergy --sort-by='.lastTimestamp'
```

Also record node CPU and memory, SmartEnergy Pod memory, restarts, OOM kills, pressure conditions, Pending Pods, scheduler errors, and platform API responsiveness.

## Stop and reassess

Stop the rollout for `MemoryPressure=True`, `DiskPressure=True`, an OOM kill, eviction, resource-related Pending Pod, repeated restart loop, migration failure, PVC mount failure, unexpected public exposure, NetworkPolicy bypass, platform or Argo degradation, failure of an existing Prometheus target, stateful services approaching limits at idle or representative load, or sustained node memory around or above 80–85% without safe rollout margin.

Diagnose first, reduce optional work or retune resources where safe, then reassess. Resize only when evidence requires it.

## Manual final-sync checklist

Before the closeout revision's manual sync, confirm:

- [ ] `targetRevision` is exactly `6dee39d597ce87620f4f12b17ea0bdc079d029f0`.
- [ ] AppProject and destination namespace are exactly `smartenergy`.
- [ ] Application image digest is exactly `sha256:0d13398c342931d23d726b4db1903508272e45fefc27687f9cfe311798a4e363`.
- [ ] PostgreSQL and Redis images retain their reviewed digests.
- [ ] All three required Secret names and keys exist out of Git.
- [ ] Resource totals fit the namespace quota.
- [ ] Exactly two PVCs request 6 GiB total.
- [ ] Ingress host is `energy.platform.eoghanclancy.eu`.
- [ ] Certificate issuer is `letsencrypt-production`.
- [ ] NetworkPolicies retain exact DNS, database, Redis, Traefik, Prometheus, and ACME solver paths.
- [ ] Migration remains a wave `-1` Sync hook.
- [ ] The application render contains no Namespace, ResourceQuota, Secret, ServiceAccount, or RBAC.
- [ ] The Argo diff contains only expected StatefulSets, Services, PVCs, Deployments, migration Job, ConfigMap, NetworkPolicies, Ingress, Certificate, and Middleware resources.
- [ ] The render contains no backup CronJob, backup object-storage egress policy, or `smartenergy-backup` reference.

Review the final Argo diff before the normal manual synchronization. Stop if it contains PVC deletion, unexpected StatefulSet replacement, Secret deletion, RBAC changes, or unrelated drift.
