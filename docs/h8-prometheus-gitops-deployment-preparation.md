# H8.4D Prometheus GitOps Deployment Preparation

H8.4D prepares the reviewed Prometheus and reduced kube-state-metrics package
for a later manually authorized Argo CD synchronization. It does not deploy or
synchronize a live resource.

## Immutable source and values

The observability Application remains a single-source Helm Application:

| Field | Value |
| --- | --- |
| Repository | `git@github.com:etclank/cloud-native-service-control-plane.git` |
| Revision | `14d483d2169352be2d196b083d963f4ffdbeabf3` |
| Path | `deploy/observability` |
| Release | `observability` |
| Values | `values.yaml`, then `values-h8.4b-candidate.yaml` |
| Destination | `https://kubernetes.default.svc`, namespace `observability` |

The immutable revision is the preceding
`H8.4 scope decision and Prometheus implementation` commit. It contains the
complete H8.4B/C runtime and six-job scrape configuration without depending
on the later bootstrap commit that points to it.

The ordinary `values.yaml` still disables Prometheus and kube-state-metrics.
Only the explicit second value file enables them. This preserves the
seven-object Collector default render while making the reviewed enabled render
available to the manually synchronized Application.

`passCredentials` remains explicitly false. The Application has no automated
sync, prune, self-heal, retry, lifecycle finalizer, hook, or sync wave.

## Restricted AppProject delta

The existing exact repository and `observability` destination remain
unchanged. The AppProject allowlist expands only for kinds in the enabled
render:

- cluster-scoped `rbac.authorization.k8s.io/ClusterRole`;
- cluster-scoped `rbac.authorization.k8s.io/ClusterRoleBinding`;
- namespaced core `PersistentVolumeClaim`;
- namespaced `apps/Deployment`.

Existing permission for `Namespace`, `ConfigMap`, `ServiceAccount`, `Service`,
`DaemonSet`, and `NetworkPolicy` remains. No wildcard group, kind, repository,
server, or namespace is granted.

## Reviewed deployment inventory

The enabled Helm render contains exactly 31 objects:

- the seven existing Collector objects;
- five Prometheus objects, including its 3Gi `local-path` PVC;
- three reduced kube-state-metrics objects;
- two ClusterRoles;
- three ClusterRoleBindings;
- eleven exact NetworkPolicies.

Its only Prometheus jobs are:

1. `prometheus-self`;
2. `kube-state-metrics`;
3. `platform-operator`;
4. `opentelemetry-collector`;
5. `control-plane-api`;
6. `managed-demo`.

Kubelet and cAdvisor are optional post-H8 enhancements under the
[final scope decision](h8-prometheus-scope-decision.md). The enabled render
contains no node scrape, `nodes/metrics`, `nodes/proxy`, or TCP 10250 access.

Prometheus remains a private ClusterIP workload with a 3Gi `local-path`
ReadWriteOnce claim, 72-hour and 2GB retention bounds, one Recreate replica,
and a 300-second termination grace period. No public metrics route is added.

## H8.4E live gate prerequisites

Before any live synchronization:

1. merge or otherwise make the immutable source revision reachable from the
   configured repository;
2. confirm the platform operator and control-plane API Applications have been
   manually synchronized to revisions containing the H8.4C metrics ports and
   ingress policies;
3. confirm the managed demo has reconciled the H8.4C metrics Service port;
4. revalidate the Kubernetes API backend used by the exact
   `142.132.178.45/32` TCP 6443 egress policies;
5. confirm adequate node and `local-path` capacity for the 3Gi claim and the
   bounded CPU/memory budget;
6. record the current observability Application, AppProject, Collector
   resources, Pod UID, readiness, and restart baseline;
7. confirm no Argo operation is active and request an explicit live gate.

H8.4E must apply the reviewed AppProject and Application definitions, then
perform a manual synchronization without prune. It must verify the PVC is
Bound, Pods are Ready, RBAC and policies match the render, exactly six targets
are healthy, the TSDB limits are active, and the existing Collector remains
stable. H8.4F owns the final evidence and documentation closeout.

## Rollback and PVC boundary

Automated pruning and self-healing remain forbidden. The PVC is node-local
operational persistence, not a backup. Its StorageClass reclaim policy can
delete underlying data when the claim is deleted.

Before live synchronization, the operator must prepare an explicit rollback
that:

- records the PVC and PV identities and reclaim policy;
- returns the Application source to the prior Collector-only revision and
  `values.yaml`;
- uses manual synchronization without prune;
- separately removes only reviewed non-storage Prometheus and
  kube-state-metrics objects if required;
- preserves the PVC unless a distinct destructive-data-loss gate authorizes
  its deletion;
- verifies the original seven Collector resources and UIDs remain healthy.

Reverting the Application source without prune does not itself remove objects
that disappear from the desired render. That behavior is intentional: it
prevents an implicit PVC deletion but requires a separately reviewed cleanup
operation during rollback.

Repository-side H8.4B, H8.4C, and H8.4D completed with this preparation.
H8.4E live deployment/validation and H8.4F closeout completed on 2026-07-23;
see [`h8-observability-closeout.md`](h8-observability-closeout.md).
