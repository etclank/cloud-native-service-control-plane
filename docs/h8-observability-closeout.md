# H8 Observability Closeout

> Historical portfolio record: deployment observations apply to the revisions
> and dates recorded below, not current availability. Review environment-specific
> commands before use; see the [documentation index](README.md).

H8 completed on 2026-07-23 on the personal single-node K3s portfolio
environment. The accepted scope is the private OpenTelemetry Collector plus a
bounded standalone Prometheus and reduced kube-state-metrics deployment. Loki,
Tempo, Grafana, workload OTLP export, kubelet, and cAdvisor are optional
post-H8 enhancements.

## Deployment identity

The live environment was verified as:

- node `portfolio-k3s-01`, UID
  `f68ef7c8-16ba-493e-9ce0-88586f2a18d4`, running K3s
  `v1.36.2+k3s1`;
- Argo CD Applications `platform-operator`, `control-plane-api`, and
  `observability`, all manually synchronized, `Synced`, and `Healthy`;
- platform Applications at immutable revision
  `4881195ca84d6236453fcc430c97f7cbc1f1225d`;
- observability Application at immutable revision
  `34420d6c3a24e7a22d8b0a9cbb1e738dca08552c`;
- no automated synchronization, pruning, or self-healing.

The operator runs
`ghcr.io/etclank/cloud-native-service-control-plane-operator@sha256:8f166fe9cdcbab093dee0bbef460e96dc1ec612973c48f9a07de35f16f4d0937`.
The control-plane API and managed demo retained their reviewed immutable H8
digests. All three workloads rolled out successfully.

## Deployment corrections

Two ordinary configuration defects were corrected through the repository and
the same manual GitOps path:

1. the restricted `platform-control-plane` AppProject gained only
   `networking.k8s.io/NetworkPolicy` in its namespaced allowlist;
2. Collector internal metrics moved from the removed
   `service.telemetry.metrics.address` key to the supported Prometheus
   pull-reader schema on the Collector Pod IP and TCP 8888.

No wildcard permission, public metrics route, node scrape permission, Secret,
or mutable image reference was introduced.

## Live inventory and readiness

The observability namespace contains one Ready Prometheus Deployment, one
Ready kube-state-metrics Deployment, and one Ready Collector DaemonSet Pod.
After correction and the persistence exercise, every container had zero
restarts.

The private ClusterIP Services expose only:

- Prometheus: TCP 80 to container TCP 9090;
- kube-state-metrics: TCP 8080;
- Collector: OTLP TCP 4317, OTLP/HTTP TCP 4318, and internal metrics TCP 8888.

The platform API and managed demo expose metrics only on their internal
ClusterIP Services at TCP 9090. The operator retains authenticated HTTPS
metrics on TCP 8443. Prometheus itself has no Ingress or public Service.

## Prometheus acceptance

Prometheus reported Ready and exactly these six active jobs as healthy:

| Job | Live result |
| --- | --- |
| `prometheus-self` | up |
| `kube-state-metrics` | up |
| `platform-operator` | up |
| `opentelemetry-collector` | up |
| `control-plane-api` | up |
| `managed-demo` | up |

No `kubelet` or `cadvisor` job exists. The live Prometheus discovery role
permits `get`, `list`, and `watch` only for Pods, Services, and EndpointSlices.
Authorization checks denied Nodes, `nodes/metrics`, and `nodes/proxy`.

## Storage and resource bounds

The Prometheus PVC is Bound at 3Gi using `local-path`:

- PVC UID: `e3e8993a-ec50-4051-bab2-301b4810deb0`;
- PV: `pvc-e3e8993a-ec50-4051-bab2-301b4810deb0`;
- PV UID: `0788514c-59d2-47f4-a729-b9e1cbf5d0d4`;
- reclaim policy: `Delete`.

Prometheus enforces both `72h` time retention and `2GB` size retention. Its
request/limit is 100m/400m CPU and 256Mi/512Mi memory.
kube-state-metrics is bounded at 25m/100m and 32Mi/64Mi. The Collector remains
bounded at 25m/200m and 96Mi/192Mi.

During closeout, the three observability Pods used approximately 6m CPU and
75Mi memory in total. The node used 125m CPU (6%) and 2235Mi memory (58%),
leaving the required safety margin.

## Persistence and rediscovery

Prometheus was recreated once through its Deployment. The Pod UID changed
from `f2c723da-28e7-48d1-ae29-7d8736db7c20` to
`c825e5f6-12a0-431a-b8c0-ec753b7f906f`; the PVC and PV identities did not
change. The TSDB head `minTime` remained `1784806229702`, historical `up`
samples remained queryable, and all six jobs rediscovered as healthy. The new
Pod became Ready with zero restarts.

## Network boundary

The default-deny policy remains active. Target-specific policies admit only
the exact Prometheus identity and target ports 8080, 8443, 8888, or 9090.
Kubernetes API egress remains the proven node `/32` on TCP 6443; DNS is limited
to TCP/UDP 53. Collector OTLP ingress remains restricted to the two approved
workload identities on TCP 4317 and 4318.

The live policy set contains no TCP 10250 rule, wildcard CIDR, public
Prometheus ingress, NodePort, LoadBalancer, host port, or host network.

## Rollback proof

The prior Collector-only Application from merged commit
`3469f6a0809f508fc81b21abe4fe33b9e565d7ce` passed Kubernetes server-side
dry-run with the existing bootstrap field manager and no conflict. The tested
rollback is:

1. restore the reviewed Collector-only Application revision and `values.yaml`;
2. synchronize manually without prune;
3. remove only reviewed non-storage Prometheus and kube-state-metrics objects
   in a separately authorized cleanup;
4. preserve the PVC unless explicit data-loss authorization is granted.

The live rollback was deliberately not executed because that would
unnecessarily disrupt a healthy deployment and risk retained data. The PVC
reclaim policy makes claim deletion a destructive boundary.

## Validation

The final implementation passed:

- focused observability and GitOps regression tests;
- `make test`;
- `make lint`;
- `make build`;
- deterministic Helm/Kustomize rendering and locked chart checksum checks;
- `git diff --check`.

The Collector remains configured with only the `nop` exporter, and production
workloads still do not export OTLP. H8 and H8.4 are complete. Later
observability work must pass a new resource, security, persistence, and
exposure review.
