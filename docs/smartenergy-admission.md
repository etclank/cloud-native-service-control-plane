# SmartEnergy admission record

SmartEnergy admission is prepared but has not been synchronized or deployed.

| Item | Admitted value |
| --- | --- |
| Deployment revision | `280410f47285a29dbe6eb28159fcfcbfd76a3cd6` |
| Application image source revision | `3e39c1e66c15f276aeb88fa5c4d322ec301870e2` |
| Application image digest | `sha256:b9ed2c1be78d707f234df14e08679204a5249def787d0b8e26f398cce41e415f` |
| Repository | `https://github.com/etclank/smartenergy-api.git` |
| Production overlay | `deploy/kubernetes/overlays/production` |
| Namespace / AppProject | `smartenergy` / `smartenergy` |
| Hostname | `energy.platform.eoghanclancy.eu` |
| Sync policy | Manual; automated sync, pruning, and self-healing disabled |
| Current state | Prepared, not synchronized |

Project 1 owns only the Argo Application, namespace policy, AppProject, quota, and platform-side Prometheus configuration. SmartEnergy owns all rendered application workloads and namespaced runtime resources.

## Manual prerequisites

Provision the four runtime Secrets using the [value-free runbook](smartenergy-secret-provisioning.md). No Secret value belongs in Git.

A production backup destination requires an off-node S3-compatible HTTPS endpoint, bucket, AWS Signature V4 compatibility, access credentials, region where required, and an object lifecycle retaining seven daily and four weekly recovery points. No provider has been selected. PostgreSQL may be admitted before this choice, but the backup CronJob is not operationally complete until the destination and successful restore proof exist.

Do not create DNS during admission preparation. Shortly before final public exposure, configure `energy.platform.eoghanclancy.eu` to the current Hetzner public endpoint using the appropriate A, AAAA, or CNAME record. Add or verify DNS only after PostgreSQL, Redis, migration, and the API are healthy internally and capacity remains acceptable.

The SmartEnergy Certificate uses `ClusterIssuer/letsencrypt-production`, matching Project 1's production issuer.

## Capacity baseline status

Stage 6B could not refresh the real platform baseline because no Kubernetes context or administrative tunnel was configured on the validation host. The previously recorded sample is historical and must not be used as the deployment decision. Re-establish read-only cluster access and run the checkpoint commands below immediately before the first Stage 7 sync.

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

## Manual first-sync checklist

Before a later manual sync, confirm:

- [ ] `targetRevision` is exactly `280410f47285a29dbe6eb28159fcfcbfd76a3cd6`.
- [ ] AppProject and destination namespace are exactly `smartenergy`.
- [ ] Application image digest is exactly `sha256:b9ed2c1be78d707f234df14e08679204a5249def787d0b8e26f398cce41e415f`.
- [ ] PostgreSQL, Redis, and backup images retain their reviewed digests.
- [ ] All four Secret names and required keys exist out of Git.
- [ ] Resource totals fit the namespace quota.
- [ ] Exactly two PVCs request 6 GiB total.
- [ ] Ingress host is `energy.platform.eoghanclancy.eu`.
- [ ] Certificate issuer is `letsencrypt-production`.
- [ ] NetworkPolicies retain exact DNS, database, Redis, Traefik, Prometheus, and backup paths.
- [ ] Migration remains a wave `-1` Sync hook.
- [ ] The backup CronJob and its manual destination prerequisite are understood.
- [ ] The application render contains no Namespace, ResourceQuota, Secret, ServiceAccount, or RBAC.
- [ ] The Argo diff contains only expected StatefulSets, Services, PVCs, Deployments, migration Job, CronJob, ConfigMap, NetworkPolicies, Ingress, Certificate, and Middleware resources.

Do not run `argocd app sync smartenergy` during Stage 6B.
