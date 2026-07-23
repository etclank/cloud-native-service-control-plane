# H8.4B Prometheus Runtime Security Foundation

H8.4B defines the repository-only Prometheus and kube-state-metrics runtime
foundation. H8.4C subsequently replaced its inert scrape placeholder with the
accepted six-job configuration. Neither dependency is enabled in the ordinary
`values.yaml`, and no live resource was deployed by either repository slice.
H8 remains incomplete pending GitOps preparation, live validation, and
closeout.

## Configuration and architecture

The wrapper chart is version 0.5.0 and retains the locked dependencies:

- `prometheus-community/prometheus` 29.18.0, Prometheus v3.13.1;
- direct `kube-state-metrics` 7.8.1, kube-state-metrics v2.19.1;
- the existing OpenTelemetry Collector 0.165.0.

Both new dependencies remain disabled by default. The
`values-h8.4b-candidate.yaml` fixture sets only their two `enabled` flags and
is used for deterministic local review and the intended H8.4D GitOps overlay.
The Prometheus chart's bundled kube-state-metrics, Alertmanager,
node-exporter, Pushgateway, config reloader, and test hooks remain disabled.
The direct kube-state-metrics chart has no RBAC proxy, ServiceMonitor,
self-monitor Service port, test workload, persistence, or auxiliary
container.

Prometheus is one standalone Deployment using the chart's Recreate strategy.
It is neither highly available nor an Operator-managed StatefulSet. There is
no federation, remote read, remote write, or public endpoint. H8.4C's rendered
ConfigMap contains exactly the six jobs accepted by the
[final target scope](h8-prometheus-scope-decision.md). The chart's lifecycle
reload flag is disabled because no config-reloader sidecar is present.

## Storage, retention, and resources

| Component | Replicas | CPU request / limit | Memory request / limit | Storage |
| --- | ---: | --- | --- | --- |
| Prometheus | 1 | 100m / 400m | 256Mi / 512Mi | 3Gi `local-path`, ReadWriteOnce |
| kube-state-metrics | 1 | 25m / 100m | 32Mi / 64Mi | none |
| H8.4 candidate total | 2 | 125m / 500m | 288Mi / 576Mi | 3Gi requested |

Prometheus passes both
`--storage.tsdb.retention.time=72h` and
`--storage.tsdb.retention.size=2GB`; whichever bound is reached first limits
retention. The 2GB TSDB cap leaves about 1Gi of claim headroom for the WAL,
compaction overlap, filesystem metadata, and operational variance. The
`local-path` claim is node-local operational persistence, not a backup,
replica, independent failure domain, or guaranteed filesystem quota. Node,
PV, or claim loss can remove all history; WAL recovery covers only data that
survives on that volume.

If the claim or shared node filesystem fills, Prometheus may fail WAL or block
writes, compaction, ingestion, and readiness. Do not increase the claim or
retention automatically. Stop ingestion, preserve evidence, and review node
space, the claim, and recovery separately. Deleting the Deployment leaves the
PVC object only when pruning does not include it. An explicit Argo prune or
PVC deletion can trigger the `local-path` StorageClass `Delete` reclaim
boundary and permanent data loss. H8.4D must preserve this consequence in its
rollback plan; automated prune remains forbidden.

Prometheus receives a 300-second termination grace period so its normal signal
handling can flush and close the TSDB/WAL. It has no preStop hook or second
replica, and Recreate prevents a stateful rollout surge. kube-state-metrics is
stateless and also uses Recreate to keep the candidate budget to one replica;
the approved upstream chart exposes no termination-grace override, so the Pod
uses Kubernetes' 30-second default and requires no custom shutdown hook.

## Identities and RBAC

The upstream charts own two dedicated ServiceAccounts:

- `prometheus-server`, with token automount explicitly true because the
  accepted H8.4C design uses Kubernetes discovery and authenticated operator
  metrics;
- `kube-state-metrics`, with token automount explicitly true because it must
  watch its reduced Kubernetes object inventory.

Neither identity uses the namespace default ServiceAccount, shares an
identity, references a manually managed token Secret, or has an image-pull
Secret. H8.4B does not inspect token contents.

Upstream RBAC generation is disabled. Repository-owned rules are:

| Identity | API group | Resources | Verbs | Reason |
| --- | --- | --- | --- | --- |
| Prometheus | core | `pods`, `services` | `get`, `list`, `watch` | exact fixed-namespace EndpointSlice target validation |
| Prometheus | `discovery.k8s.io` | `endpointslices` | `get`, `list`, `watch` | EndpointSlice discovery |
| Prometheus | existing `platform-operator-metrics-reader` ClusterRole | non-resource `/metrics` | `get` | authenticated controller-runtime metrics |
| kube-state-metrics | core | `namespaces`, `nodes`, `persistentvolumeclaims`, `pods` | `list`, `watch` | exact enabled collectors |
| kube-state-metrics | `apps` | `daemonsets`, `deployments`, `replicasets`, `statefulsets` | `list`, `watch` | exact enabled collectors |

The operator metrics grant is referenced through a separate ClusterRoleBinding
rather than duplicated. No new rule contains a wildcard, mutation or
privilege-escalation verb, Secret, ConfigMap, Ingress, event, status,
exec/attach/port-forward resource, service proxy, non-resource URL, or
resource-name exception. Prometheus and kube-state-metrics have separate
ClusterRoles and bindings.

## Runtime and network security

Both candidate Pods run as non-root UID/GID 65534 with RuntimeDefault seccomp,
no privilege escalation, `privileged: false`, all capabilities dropped, and a
read-only root filesystem. They have no host network, PID or IPC namespace,
host port, hostPath, device, init container, sidecar, or external credential.
Only Prometheus receives `fsGroup: 65534` with
`fsGroupChangePolicy: OnRootMismatch`, because only it writes the mounted
`/data` PVC. kube-state-metrics has no `fsGroup` or writable volume.

The existing namespace-wide ingress/egress default deny remains authoritative.
Five candidate policies add only:

| Policy | Selected Pod | Allowance |
| --- | --- | --- |
| `prometheus-dns-egress` | exact Prometheus chart labels | CoreDNS Pods selected by `k8s-app=kube-dns` in `kube-system`, UDP/TCP 53 |
| `prometheus-kubernetes-api-egress` | exact Prometheus chart labels | `142.132.178.45/32`, TCP 6443 |
| `kube-state-metrics-dns-egress` | exact kube-state-metrics chart labels | CoreDNS Pods selected by `k8s-app=kube-dns` in `kube-system`, UDP/TCP 53 |
| `kube-state-metrics-kubernetes-api-egress` | exact kube-state-metrics chart labels | `142.132.178.45/32`, TCP 6443 |
| `kube-state-metrics-metrics-ingress` | exact kube-state-metrics chart labels | exact Prometheus Pod labels, TCP 8080 |

The API rule is the post-DNAT backend proven by
[`h8-prometheus-api-egress-proof.md`](h8-prometheus-api-egress-proof.md).
The ineffective `10.43.0.1/32` TCP 443 candidate, Service/Pod/node CIDRs,
metadata endpoints, `0.0.0.0/0`, port ranges, and unrestricted egress are
absent. Rediscover the endpoint after node replacement or readdressing,
EndpointSlice or API-port change, server topology change, K3s upgrade, or
CNI/NetworkPolicy reconfiguration.

Prometheus TCP 9090 has no ingress allowance because it has no approved
consumer. H8.4C adds exact egress and target ingress for each of its six jobs
without broadening the API rule. The complete policy mapping is recorded in
[`h8-prometheus-scrape-foundation.md`](h8-prometheus-scrape-foundation.md).

## Candidate object inventory and ownership

The default render remains the original seven Collector objects. The final
explicit H8 candidate render contains exactly 31 objects:

- seven unchanged Collector-era objects: Namespace, Collector ServiceAccount,
  ConfigMap, Service and DaemonSet, default deny, and OTLP ingress;
- five Prometheus upstream objects: ServiceAccount, ConfigMap, PVC, ClusterIP
  Service, and Deployment;
- three direct kube-state-metrics upstream objects: ServiceAccount, ClusterIP
  Service, and Deployment;
- sixteen repository-owned objects: two ClusterRoles, three
  ClusterRoleBindings, and eleven NetworkPolicies.

There is no Secret, CRD, Operator, Ingress, NodePort, LoadBalancer,
StatefulSet, extra DaemonSet, test Pod or Job, node scrape, alert rule, or
recording rule.

H8.4D derives and reviews the exact AppProject permission delta for
Deployment, PersistentVolumeClaim, ClusterRole, and ClusterRoleBinding before
the GitOps bootstrap changes. It also owns PVC prune and rollback behavior.
No Application or AppProject change was part of H8.4B/C.

## Deferred work and completion state

H8.4C completed all six real scrape jobs, Kubernetes service-discovery
configuration, relabeling/cardinality controls, target-specific Prometheus
egress, operator metrics ingress, Collector metrics exposure, and workload
metrics Services. H8.4D owns restricted GitOps registration changes.
H8.4E owns live synchronization, API/RBAC/NetworkPolicy checks, PVC binding,
target health, TSDB cap observation, resource soak, and rollback evidence.

No live deployment, synthetic scrape, token inspection, Secret access, Argo
operation, public exposure, or later H8 implementation occurred in H8.4B.
H8.4B/C are complete as a disabled repository candidate. Prometheus is not
deployed, and H8 remains incomplete until H8.4D preparation, H8.4E live
validation, and H8.4F closeout finish.
