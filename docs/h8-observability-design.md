# H8 Observability Architecture and Capacity Design

**Design date:** 2026-07-20

**Repository baseline:** `h8-observability-foundation` at `7fc24d44d1478ed7d52994e561d036dccb38f8aa`

**Scope:** architecture, sizing, retention, security, delivery, and implementation sequencing only

**Cluster changes made while preparing this design:** none

## 1. Decision Summary

H8 should use a deliberately small, single-node observability stack in an
`observability` namespace:

- one OpenTelemetry Collector Kubernetes-distribution DaemonSet, containing
  the required Contrib receivers and processors;
- one standalone Prometheus server, not Mimir and not
  `kube-prometheus-stack`;
- Loki in one-replica monolithic, or single-binary, mode;
- Tempo in one-replica monolithic mode;
- one private Grafana instance;
- one small kube-state-metrics instance with a reduced collector allowlist;
- local-path PVCs with short retention and no high-availability claim;
- a restricted, manual-sync Argo CD Application.

The complete stack has a proposed steady-state budget of **250m CPU and
704Mi memory requested**, with **1300m CPU and 1408Mi memory limited**. This is
within the requested design envelope, but it is not safe to add to the current
4 GB node in full. The live node was already using 2489Mi of 3820Mi allocatable
memory (65%). Adding the proposed requests projects approximately 3193Mi, or
84%, before rollout overhead. Adding the limits projects 3897Mi, which is above
allocatable memory.

The resulting decision is:

- **conditional GO** for a reversible, one-component-at-a-time pilot that
  stops at the thresholds in this document;
- **NO-GO** for declaring the complete H8 stack steady-state on the current
  4 GB node;
- **resize to at least 8 GiB RAM before completing H8, and therefore before
  starting H9**. Four vCPUs would improve compaction and query headroom, but
  memory is the present hard gate.

This is a portfolio and learning environment, not a highly available
production observability platform. A node failure can interrupt every signal
and can lose all local observability history.

## 2. Evidence and Current Baseline

### 2.1 Inspection method

The baseline was captured read-only at approximately 2026-07-20 12:00 CEST.
The inspection included node capacity and utilization, per-container usage,
workload controllers, resource declarations, Services, Ingresses, PVCs,
StorageClasses, Argo CD Applications, and the operator metrics Service. No
Secret object or Secret value was read.

The configured SSH ControlMaster socket existed but was stale: its control
check returned `Connection refused`, and direct SSH could not authenticate
without an interactive key passphrase. Root-disk evidence therefore came from
the read-only kubelet summary endpoint through the private Kubernetes API. It
reports the same root filesystem used by K3s, but it does not replace a future
host-level `du` inspection of `/var/lib/rancher/k3s` and the journal.

### 2.2 Node and filesystem

| Measure | Observed value |
| --- | ---: |
| Node | `portfolio-k3s-01` |
| Kubernetes | K3s `v1.36.2+k3s1` |
| Runtime | containerd `2.3.2-k3s2` |
| Operating system | Ubuntu 24.04.4 LTS |
| CPU capacity / allocatable | 2 / 2 cores |
| Memory capacity / allocatable | 3911572Ki / 3911572Ki, about 3820Mi |
| Current CPU | 121m, 6% |
| Current memory | 2489Mi, 65% |
| Current memory headroom | about 1331Mi |
| Root filesystem | 37.22GiB usable capacity |
| Root used / available | 4.94GiB / 30.70GiB, 13.3% used |
| Runtime image filesystem used | 1.87GiB |
| Root inodes used | 91,608 of 2,456,320, 3.7% |

The rounded sum of running containers reported by `kubectl top pods -A
--containers` was 566Mi, about 1923Mi below node-level usage. K3s, the operating
system, page cache, and usage not attributed by Metrics Server therefore make
pod metrics alone an unsafe sizing basis.

The current 6% CPU and 65% memory readings are consistent with the H7 closeout
record of approximately 6% CPU and 66% memory; they do not show that additional
memory has become safely available.

### 2.3 Existing workloads and reservations

There were 21 running Pods: one managed demo workload; seven Argo CD Pods;
three cert-manager Pods; CoreDNS, local-path-provisioner, Metrics Server,
Traefik, and the two-container ServiceLB Pod; the operator; the control-plane
API; and two validation workloads. All running application Deployments and the
Argo CD controller StatefulSet were Ready. The largest reported container
working sets were Argo CD application controller 141Mi, Dex 99Mi, Argo CD
server 78Mi, and repo server 43Mi.

Only 284Mi of explicit memory requests were present across running containers.
Most Argo CD, cert-manager, Traefik, local-path-provisioner, and ServiceLB
containers had no requests or limits. Requests therefore describe scheduling,
not the cluster's true working set or worst case.

| Workload | CPU request / limit | Memory request / limit | Observed memory |
| --- | ---: | ---: | ---: |
| Operator | 10m / 500m | 64Mi / 128Mi | 13Mi |
| Control-plane API | 10m / 100m | 32Mi / 128Mi | 9Mi |
| Managed `demo-http` | 5m / 50m | 16Mi / 32Mi | 3Mi |
| Each validation workload | 5m / 50m | 16Mi / 32Mi | rounded to 0Mi |
| CoreDNS | 100m / unset | 70Mi / 170Mi | 13Mi |
| Metrics Server | 100m / unset | 70Mi / unset | 22Mi |

### 2.4 Storage, exposure, and GitOps

- No PVC existed. The default StorageClass was `local-path`, with `Delete`
  reclaim policy, `WaitForFirstConsumer`, and no expansion support.
- The only public LoadBalancer was Traefik on ports 80 and 443. Public
  Ingresses were limited to the API and TLS validation hostnames.
- Prometheus, Loki, Tempo, Grafana, and OTLP ports were not public.
- `platform-operator`, `control-plane-api`, and `registry-smoke` Argo CD
  Applications were Synced and Healthy. Their `automated` policy was absent.
- The operator metrics Service was a ClusterIP on HTTPS port 8443.
- The operator ran non-root with a read-only root filesystem, dropped
  capabilities, RuntimeDefault seccomp, and an immutable image digest.

The existing H7 baseline and security boundaries are recorded in
[`h7-platform-deployment-closeout.md`](h7-platform-deployment-closeout.md),
[`infrastructure-context.md`](infrastructure-context.md), and
[`operator-guide.md`](operator-guide.md).

## 3. Current Telemetry and Instrumentation Gaps

| Component | Present today | Exact H8 gap | Required implementation |
| --- | --- | --- | --- |
| Operator | controller-runtime metrics on authenticated HTTPS `:8443`; health and readiness on `:8081`; controller-runtime structured key/value logs | no Prometheus installation or scrape binding; runtime uses an ephemeral self-signed metrics certificate; no trace provider or reconciliation spans; development-mode Zap output; no explicit ManagedService outcome metric | authenticated Prometheus scrape; production JSON Zap configuration; spans around reconciliation, Deployment, Service, and status operations; bounded custom counters/histograms only where controller-runtime metrics are insufficient |
| Control-plane API | `slog` startup/shutdown/fatal events; `/healthz` and Kubernetes-backed `/readyz`; hardened HTTP server; bearer authentication | no access logs, request ID, Prometheus endpoint, HTTP duration/status metrics, OTel provider, trace propagation, Kubernetes-client spans, or log/trace correlation | JSON request middleware that never records credentials or bodies; generated request ID; normalized-route metrics on a separate internal metrics listener; W3C trace propagation; HTTP and Kubernetes-client spans; OTLP export |
| Managed `demo-http` | `slog` lifecycle events; `/healthz` and `/readyz`; hardened HTTP server | no access logs, request ID, metrics endpoint, tracing, propagation, or failure-span evidence | the same safe request middleware and separate internal metrics listener; server spans and OTLP export; exclude probes from normal request logs and high-volume traces |
| Kubernetes and host | Metrics Server point-in-time CPU/memory; container stdout/stderr in K3s CRI files | no historical metrics, searchable logs, root-disk series, retained events, or telemetry correlation | Prometheus Kubernetes discovery and kubelet/cAdvisor scrapes; reduced kube-state-metrics; Collector `filelog` receiver over read-only CRI log paths |
| Observability services | none | no backend self-monitoring | Prometheus scrapes Collector, Prometheus, Loki, Tempo, Grafana, and kube-state-metrics metrics; dashboards and alert rules include ingestion failures, drops, compaction, memory, and storage |

The OpenTelemetry packages currently listed indirectly in `go.mod` do not mean
the project is instrumented. Neither HTTP binary constructs an OTel provider,
and neither wraps its handler with OTel middleware.

Health semantics are also deliberately different today: the operator's
`/healthz` and `/readyz` are controller-runtime ping checks; the API health
check is static but its readiness check lists at most one ManagedService from
the Kubernetes API; and both demo checks are static successful JSON responses.
The API and demo do not currently log requests at all, so they do not leak
headers or bodies through access logging, but they also have no explicit
request-telemetry redaction middleware for H8 to reuse.

Request correlation must use two identifiers with distinct purposes:

1. generate or validate a bounded `X-Request-ID`, return it to the caller, and
   add it to JSON logs and spans;
2. propagate W3C `traceparent`/`tracestate`, and add trace and span IDs to logs
   when a span is active.

Request IDs and trace IDs must never be Prometheus labels or Loki indexed
labels. Route metrics use fixed route templates such as
`/api/v1/managed-services/{name}`, never user-supplied names or paths.

## 4. Target Architecture

### 4.1 Component selection

| Component | Selected mode | Reason for this environment | Explicitly disabled or deferred |
| --- | --- | --- | --- |
| OpenTelemetry Collector Kubernetes distribution | one DaemonSet Pod on the single node | the official Kubernetes distribution contains the required Contrib `filelog` and Kubernetes processors; `filelog` needs node-local CRI files, and the same process accepts OTLP and enriches all signals | no operator, sidecars, host metrics receiver, or persistent queue initially |
| Prometheus | one standalone server | materially smaller and simpler than Mimir or `kube-prometheus-stack`; sufficient for one node and short retention | Mimir, Prometheus Operator, Alertmanager, Pushgateway, federation, remote write |
| Loki | one monolithic/single-binary StatefulSet using filesystem storage | official guidance identifies monolithic mode for a small meta-monitoring stack | gateway, MinIO, caches, canary, distributed read/write/backend replicas, ruler initially |
| Tempo | one monolithic Deployment or StatefulSet using local storage | low trace volume and operational simplicity do not justify microservices or Kafka | distributed mode, Kafka, object storage, metrics generator initially |
| Grafana | one Deployment with SQLite on a PVC | one private UI for all three backends; dashboards and data sources remain declarative | Ingress, anonymous access, plugins, image renderer, HA database |
| kube-state-metrics | one Deployment with an explicit collector allowlist | supplies desired/current object state and restart/PVC series that kubelet metrics do not cover cleanly | collectors unrelated to current dashboards and alerts |
| node exporter | deferred | root filesystem metrics would be useful, but its host mounts and host namespaces widen the security boundary; kubelet/cAdvisor and read-only kubelet summary checks are enough for the first slice | revisit only after a separate Pod Security and host-access review |

The required components are Collector, Prometheus, Loki, Tempo, and Grafana.
kube-state-metrics is **optional but selected** within the initial budget because
it supplies object-state metrics needed by the dashboards and alerts. Node
exporter is **deferred** for the security reason above.

### 4.2 Signal flows

```text
operator HTTPS /metrics -- bearer token + RBAC ----+
API internal /metrics -----------------------------+--> Prometheus --> Grafana
demo internal /metrics ----------------------------+
kubelet/cAdvisor + kube-state-metrics --------------+
Collector and backend self-metrics -----------------+

/var/log/pods/*/*/*.log
  -- read-only hostPath --> OTel filelog
  --> Kubernetes metadata + redaction/limits
  --> OTLP/HTTP --> Loki monolithic --> Grafana

API and demo W3C spans
  --> OTLP/gRPC Collector
  --> memory limiter + tail sampling + batching
  --> OTLP/gRPC Tempo monolithic --> Grafana

Grafana is reached only with kubectl port-forward over the private
Kubernetes API SSH tunnel. No observability Service has an Ingress,
NodePort, LoadBalancer, hostPort, or public DNS record.
```

The Collector is the ingestion and enrichment layer, not the durable store.
Prometheus owns metrics, Loki owns logs, Tempo owns traces, and Grafana owns
only its small UI database and declarative presentation configuration.

### 4.3 Namespace and workload shape

All components live in `observability`. Single replicas are intentional. PDBs
are disabled because they cannot create availability on one node and can block
maintenance. Stateful backends use `Recreate` or a non-surging StatefulSet
update; the Collector DaemonSet uses `maxUnavailable: 1`. Short telemetry
downtime during a reviewed rollout is preferable to memory pressure from a
second backend Pod.

The namespace cannot enforce the Restricted Pod Security Standard while the
Collector mounts `/var/log/pods` through `hostPath`, because Restricted forbids
hostPath volumes. Use `pod-security.kubernetes.io/enforce=baseline` and
Restricted `audit`/`warn` labels, then harden every Pod individually. The only
planned Baseline exception is the Collector's read-only log hostPath. No
component needs privilege, host PID, host IPC, host network, added capability,
or a writable host root.

## 5. Collection and Correlation Design

### 5.1 Metrics

Prometheus uses Kubernetes service discovery but an explicit keep-list of
targets and labels. Initial scrape intervals are 30 seconds for applications
and backends and 60 seconds for kubelet/cAdvisor. Health probes are not turned
into labels. Drop unneeded Go runtime collectors only after measuring their
value, and reject labels containing request IDs, trace IDs, ManagedService
names, messages, URLs, bearer identities, or pod UIDs.

The operator scrape preserves its existing secure endpoint:

1. the Prometheus ServiceAccount receives a short-lived projected token;
2. a dedicated ClusterRoleBinding grants that identity the existing rendered
   `platform-operator-metrics-reader` non-resource permission for `/metrics`;
3. Prometheus sends the token to the ClusterIP Service over HTTPS;
4. controller-runtime performs TokenReview authentication and
   SubjectAccessReview authorization through the operator's existing
   `metrics-auth-role` permissions.

The current runtime-generated serving certificate is self-signed and has no CA
bundle that a Pod in another namespace can mount. The existing Kubebuilder
ServiceMonitor example therefore uses `insecureSkipVerify: true`. The first
H8 slice may use that exact TLS setting with the authenticated endpoint and
NetworkPolicy, but it must be recorded honestly: traffic is encrypted and the
client is authenticated, while Prometheus does **not** verify server identity.
A later cert-manager or trust-manager design must provide a verifiable CA
bundle before this pattern is reused in a multi-node or less trusted network.
Disabling endpoint authentication or switching the operator to plaintext is
not an acceptable workaround.

The API and demo metrics listener must bind on all Pod interfaces to a separate
internal port, such as 9090, but only a metrics ClusterIP Service and
NetworkPolicy expose it. The API's public port-80 Service and Ingress must not
route `/metrics`.

### 5.2 Logs

The Collector Kubernetes-distribution DaemonSet uses the `filelog` receiver
and mounts only:

```text
host /var/log/pods -> container /var/log/pods, read-only
```

The include pattern is `/var/log/pods/*/*/*.log`. Exclude the Collector's own
container logs to prevent an export-error feedback loop. Start at the end on
first installation to avoid ingesting the full historical CRI backlog. A
restart without persistent receiver offsets can produce a small gap or
duplicate window; accepting that limitation avoids a writable hostPath in the
first release.

The `k8sattributes` processor needs `get`, `list`, and `watch` on Pods,
Namespaces, and Nodes. Add ReplicaSet and Deployment read access only if the
chosen workload-name extraction requires it. It needs no Secret, ConfigMap,
write, status, exec, log, or impersonation permission.

Loki indexed labels are limited to stable low-cardinality values:
`cluster`, `namespace`, `workload`, `container`, `app`, and severity. Pod name,
file path, trace ID, and request ID remain structured metadata or parsed JSON
fields. The Collector removes authorization/cookie headers, API token fields,
and obvious credential keys before export. Applications must never log an
Authorization header, token-file content, environment dump, request body, or
ManagedService message.

### 5.3 Traces

The API and demo use server spans with normalized route, method, status code,
service name, and error classification. The API adds child spans around the
Kubernetes lifecycle calls, but not request bodies, bearer credentials, or
ManagedService messages. The operator adds spans for reconciliation and its
Deployment, Service, and status phases. Service names are fixed:
`platform-operator`, `control-plane-api`, and `demo-http`.

Applications send OTLP/gRPC only to the Collector ClusterIP. The Collector
uses a memory limiter, bounded queues, batch processing, Kubernetes metadata,
and tail sampling. The initial policy retains all error traces and 10% of other
non-probe traces. A five-second decision window is sufficient at the current
rate-limited API traffic. Health and readiness spans are dropped. If tail
sampling pushes Collector memory above its limit or drops data, replace it
temporarily with 10% parent-based head sampling rather than increasing memory
on the 4 GB node.

## 6. Resource Budget and Capacity Gates

### 6.1 Proposed requests and limits

These are implementation starting points, not promises about actual usage.
Every component must be observed during a 30-minute idle soak and a bounded
demo exercise before the next component is synchronized.

| Component | Replicas | CPU request | CPU limit | Memory request | Memory limit |
| --- | ---: | ---: | ---: | ---: | ---: |
| OTel Collector Kubernetes distribution | 1 DaemonSet Pod | 25m | 200m | 96Mi | 192Mi |
| Prometheus server | 1 | 100m | 400m | 256Mi | 512Mi |
| Loki monolithic | 1 | 50m | 250m | 128Mi | 256Mi |
| Tempo monolithic | 1 | 25m | 200m | 96Mi | 192Mi |
| Grafana | 1 | 25m | 150m | 96Mi | 192Mi |
| kube-state-metrics | 1 | 25m | 100m | 32Mi | 64Mi |
| **Total** | **6 Pods** | **250m** | **1300m** | **704Mi** | **1408Mi** |

Arithmetic:

```text
memory requests = 96 + 256 + 128 + 96 + 96 + 32 = 704Mi
memory limits   = 192 + 512 + 256 + 192 + 192 + 64 = 1408Mi
CPU requests    = 25 + 100 + 50 + 25 + 25 + 25 = 250m
CPU limits      = 200 + 400 + 250 + 200 + 150 + 100 = 1300m

current memory + requests = 2489 + 704 = 3193Mi
3193 / 3820 = 83.6%

current memory + limits = 2489 + 1408 = 3897Mi
3897 / 3820 = 102.0%
```

The request total is within 650–750Mi and the limit total is 1.375GiB, within
1.25–1.5GiB. The node-level arithmetic still fails the safety reserve. A
request is a scheduler guarantee, not a cap, and many current system Pods are
unbounded.

### 6.2 Stop/go thresholds

| Signal | GO for next slice | Stop and roll back | Emergency action |
| --- | --- | --- | --- |
| Node memory | below 75% for 30 minutes and at least 768Mi available | 80% or more for 5 minutes, less than 640Mi available, sustained reclaim, or any eviction | above 90%, OOM kill, or node `MemoryPressure`: revert latest slice immediately |
| Component memory | working set below 75% of limit and no upward idle trend | above 85% of limit for 10 minutes or repeated GC/queue growth | OOMKilled or restart loop |
| Node CPU | below 60% sustained; short peaks below 85% | above 70% for 15 minutes or API/operator latency degradation | above 90% for 5 minutes with failed probes or reconciliation backlog |
| Root disk | below 60% and forecast below 70% through the retention window | 70% used, below 10GiB free, or retention/compaction not reducing old data | 80% used or below 6GiB free: stop ingestion before data corruption |
| PVC | below 70% after one full retention window | 75% used or unexpected monotonic growth | 85% used: stop the owning receiver and preserve evidence |
| Telemetry health | no drops, failed exports, or scrape gaps during bounded test | sustained dropped spans/logs, failed exports, corrupt blocks, or target down | disable the failing pipeline rather than raising all limits |

The current 65% memory baseline passes only the preflight check. It does not
authorize syncing the whole stack. Stateful backends must not use a rolling
surge. Before each manual sync, reserve at least the incoming component's full
memory limit plus 256Mi operational headroom.

## 7. Storage, Retention, and Failure Boundaries

| Owner | PVC | Retention and cap | Compaction / cleanup | Data-loss boundary |
| --- | ---: | --- | --- | --- |
| Prometheus | 3Gi `local-path` | 72h and `--storage.tsdb.retention.size=2GB`; whichever is reached first | Prometheus TSDB block compaction and retention | node/PV loss removes history; WAL recovery is local only |
| Loki | 3Gi `local-path` | 72h; target no more than about 2.5Gi of retained chunks/index | retention enabled in the compactor; delete store on filesystem; strict ingestion and stream limits | node/PV loss removes all logs; retention is asynchronous |
| Tempo | 2Gi `local-path` | 48h; target no more than about 1.5Gi of blocks | monolithic compaction with 48h block retention | node/PV loss removes all traces; in-flight spans may be lost on restart |
| Grafana | 1Gi `local-path` | no telemetry retention; SQLite, users, and preferences only | no compactor; dashboards and data sources provisioned from Git | UI state can be lost; declarative dashboards and data sources are reproducible |
| **Total** | **9Gi requested** | **about 6.25Gi target payload plus filesystem/compaction headroom** | one owner per data type | all storage is node-local and non-HA |

K3s local-path PVC size is a requested capacity, not a reliable filesystem
quota on the shared root disk. Backend retention, ingestion limits, and
node-disk alerts are the actual controls. The current 30.70GiB free space makes
the 9Gi claim plan disk-feasible: even fully consumed it would leave about
21.7GiB if other usage were static. That does not solve the memory gate.

Use `Retain` semantics during early implementation where the chart permits it,
or explicitly protect PVC deletion in Argo CD. Automatic pruning remains off.
Deleting a backend or PVC is a separate reviewed operation. H11 must add
backup/restore decisions; until then, observability data is disposable and no
durability claim is made.

Additional controls:

- Loki: filesystem TSDB schema, one tenant, 1MiB/s ingestion rate, 2MiB burst,
  bounded query parallelism, bounded streams, and no untrusted high-cardinality
  labels;
- Tempo: bounded receivers, search concurrency, block duration, and query
  concurrency; metrics generator disabled;
- Prometheus: 30–60 second scrapes, 72h/2GB dual retention, no remote write;
- Collector: memory limiter at about 144Mi with a 32Mi spike allowance,
  bounded sending queues, and no disk queue initially;
- host: keep K3s/containerd image garbage collection and the system journal in
  the same root-disk forecast. Verify `/var/lib/rancher/k3s` with SSH after the
  ControlMaster is restored.

## 8. Helm and Image Supply Chain

### 8.1 Version selection as of 2026-07-20

The following versions were the first, therefore newest, entries in the
maintainers' official Helm indexes when checked on 2026-07-20. The chart
artifact SHA-256 values came from those same index entries.

| Component | Official chart and repository | Chart version | App version | Chart artifact SHA-256 |
| --- | --- | ---: | ---: | --- |
| OTel Collector | `open-telemetry/opentelemetry-collector` from the [OpenTelemetry index](https://open-telemetry.github.io/opentelemetry-helm-charts/index.yaml) | `0.165.0` | `0.156.0` | `b592ea064d9b906930cac2d22b88eeb1bc82f12d5ed07fd20792de2c051ca3c5` |
| Prometheus | `prometheus-community/prometheus` from the [Prometheus Community index](https://prometheus-community.github.io/helm-charts/index.yaml) | `29.18.0` | `v3.13.1` | `24f5f056dd5cb00e98ffb905c9c2779e810153f1b5a6306bf2cc2c5a4f02a0b9` |
| Loki | `grafana-community/loki` from the [Grafana Community index](https://grafana-community.github.io/helm-charts/index.yaml) | `18.5.1` | `3.7.3` | `f6b938d251bce2d70c21e81c601f4a1b6690ab6792de6e5ca1c347d85323b6f0` |
| Tempo | `grafana-community/tempo` from the [Grafana Community index](https://grafana-community.github.io/helm-charts/index.yaml) | `2.2.3` | `2.10.7` | `14c52efe5d0cad5456ffa5a8be1e5107be47d184be3e209f748be51a7b0316fe` |
| Grafana | `grafana-community/grafana` from the [Grafana Community index](https://grafana-community.github.io/helm-charts/index.yaml) | `12.7.2` | `13.1.0` | `5254a977708ba74697ad1129154235c54fb2fd575e585d1d0306fdd5ddb76751` |

Grafana's documentation records the March 2026 Loki chart move to the
Grafana Community repository. The community repository states that OCI is the
preferred distribution and that its charts are signed. Use the community
versions above, not the older `grafana/helm-charts` index entries that remained
available during inspection.

### 8.2 Reproducible delivery model

| Model | Assessment | Decision |
| --- | --- | --- |
| Pinned upstream charts with repository-owned values | preserves upstream maintenance, exposes values and dependencies for review, and can be locked by version, `Chart.lock`, chart artifact digest, and rendered tests | **selected**, using a repository-owned wrapper chart |
| Vendored rendered manifests | maximally inspectable at sync time but creates a large generated diff and obscures how to reproduce or upgrade upstream templates | retain as a fallback only if a selected chart cannot render an immutable image or compliant security context |
| Handwritten component manifests | smallest possible render but transfers every upstream configuration, upgrade, and security fix to this repository | reject for the backends; use narrow handwritten integration resources such as NetworkPolicies, RBAC bindings, dashboards, and the Argo definitions |

Create a repository-owned wrapper chart and values under a future
`deploy/observability` path. Pin every dependency version exactly, commit
`Chart.lock`, verify downloaded chart signatures where supported, verify the
artifact digests above, and commit the values, dashboards, rules, and Collector
configuration. CI must run `helm dependency build`, `helm lint`, `helm template`,
schema validation, policy checks, and a rendered resource-budget sum.

One restricted `observability` AppProject should permit only:

- this private Git repository and the exact official chart repositories needed
  by the locked dependencies;
- the `observability` destination on
  `https://kubernetes.default.svc`;
- exact namespaced and cluster-scoped kinds derived from rendered output;
- no wildcard repository, namespace, API group, or kind.

One Argo CD Application targets `main` and the repository-owned wrapper path.
Its `syncPolicy.automated` field is absent: no automatic sync, prune, or
self-heal. Synchronization and rollback select a reviewed Git revision
manually, consistent with the existing platform Applications.

### 8.3 Intended image inventory and digest support

The exact chart archives were inspected read-only in a temporary directory.
With every optional helper named below disabled, the intended render introduces
only these six runtime images. The versions shown are mutable tag identities
from the pinned charts, not approved runtime references; implementation must
resolve and substitute immutable platform-specific digests.

| Chart | Intended image before digest resolution | Supported immutable override | Required handling |
| --- | --- | --- | --- |
| OTel Collector `0.165.0` | `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-k8s:0.156.0` | `image.digest` renders `repository@digest` | set the repository explicitly, set `command.name=otelcol-k8s`, and verify the distribution contains every configured component |
| Prometheus `29.18.0` | `quay.io/prometheus/prometheus:v3.13.1` | `server.image.digest` renders `repository@digest` | disable the config-reloader sidecar and set the server digest |
| Prometheus dependency kube-state-metrics `7.8.1` | `registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.19.1` | `image.sha` renders `repository:tag@sha256:digest` | set the SHA-256 and keep kube-rbac-proxy disabled |
| Loki `18.5.1` | `docker.io/grafana/loki:3.7.3` | `loki.image.digest` renders `repository@digest` | set a full `sha256:` digest and render only the monolithic workload |
| Tempo `2.2.3` | `docker.io/grafana/tempo:2.10.7` | no separate digest field; `tempo.tag` is inserted after `:` and supports a valid `2.10.7@sha256:digest` reference | use tag-plus-digest, assert the rendered digest, and keep `tempoQuery.enabled=false` |
| Grafana `12.7.2` | `docker.io/grafana/grafana:13.1.0` | `image.sha` renders `repository:tag@sha256:digest` | set the SHA-256 and disable test, init-chown, sidecar, renderer, and download containers |

The Prometheus parent chart locks Alertmanager `1.40.3`, node-exporter
`4.56.1`, Pushgateway `3.7.0`, and kube-state-metrics `7.8.1`; only the last is
enabled. Its default config reloader image is
`quay.io/prometheus-operator/prometheus-config-reloader:v0.92.1`, and it must be
disabled. The Loki chart locks MinIO `5.4.0` and rollout-operator `0.50.1` and
also carries defaults or templates for a Loki canary, gateway, Memcached,
Memcached exporter, rules sidecar, and a `loki-helm-test:latest` image. Disable
all of them. Grafana defaults include `bats/bats:1.13.0` for chart tests and
`busybox:1.38.0` for init-chown; disable both. These transitive defaults are why
the final rendered-image assertion, not the intended list alone, is the gate.

Chart versions do not make container images immutable. Charts can reference
mutable app tags, helper images, test Pods, init containers, sidecars, and
subchart images. Digest override fields differ between charts, and setting a
repository to `name@sha256:...` can produce a malformed reference if the chart
still appends a tag. Before any deployment:

1. render the exact locked chart set with all optional subcharts disabled;
2. enumerate every `containers[*].image` and `initContainers[*].image`;
3. resolve each approved upstream image to a platform-specific immutable
   digest and use the chart's supported digest field;
4. fail CI for `:latest`, branch tags, unqualified image names, unexpected
   registries, or any rendered reference without `@sha256:`;
5. record source chart version, chart artifact digest, image digest, and
   rendered-manifest hash.

Do not enable chart test Pods, download plugins at startup, or inherit default
MinIO, Memcached, gateway, Alertmanager, Pushgateway, node-exporter, or canary
images. A chart upgrade is a separately reviewed supply-chain change.

## 9. Security Model

### 9.1 ServiceAccounts and RBAC

Use one ServiceAccount per component and disable token automount unless it is
required:

- **Collector:** token required; read-only `get/list/watch` for Pods,
  Namespaces, and Nodes, plus apps workload kinds only if metadata enrichment
  needs them.
- **Prometheus:** token required; read-only discovery of Nodes, Pods,
  Services, EndpointSlices, and Namespaces; `get` on node metrics/proxy paths;
  the explicit `/metrics` non-resource grant for the operator.
- **kube-state-metrics:** token required; read-only access only to enabled
  collectors such as Nodes, Namespaces, Pods, Deployments, ReplicaSets,
  StatefulSets, DaemonSets, and PVCs.
- **Loki, Tempo, and Grafana:** no Kubernetes API access and
  `automountServiceAccountToken: false`.

No component receives Secret-listing permission. Grafana admin credentials and
any future notification credentials come from named Secret references created
outside Git; values are never placed in Helm values, ConfigMaps, dashboards,
logs, or this repository.

### 9.2 Runtime hardening

Override chart defaults to require non-root execution, RuntimeDefault seccomp,
dropped capabilities, no privilege escalation, a read-only root filesystem,
bounded writable `emptyDir` mounts, and explicit requests/limits. Use `fsGroup`
only for the local-path volumes that require it. Render and validate every
Pod; do not assume a chart's advertised defaults satisfy Pod Security.

The Collector receives the only hostPath exception, mounted read-only. Loki,
Tempo, Prometheus, and Grafana never mount the host filesystem. No component
uses a Docker socket, containerd socket, host process namespace, or Kubernetes
Secret volume beyond its own explicitly named credential.

### 9.3 Network policy and exposure

First verify that K3s was not started with `--disable-network-policy`; K3s
normally includes its kube-router-based NetworkPolicy controller. Introduce
policies with connectivity tests in the same slice, because a mistaken default
deny can blind the platform.

The intended policy is default-deny ingress and egress in `observability`, then
allow only:

- DNS to CoreDNS;
- Kubernetes API access from Collector, Prometheus, and kube-state-metrics;
- application OTLP to Collector ports 4317/4318;
- Collector egress to Loki and Tempo;
- Prometheus egress to approved metrics Services, Pods, and kubelet endpoints;
- Grafana egress to Prometheus, Loki, and Tempo;
- backend self-metrics from Prometheus;
- no Internet ingress and no public Service type.

Services are ClusterIP. Grafana is private and reached by
`kubectl port-forward service/<grafana-service>` over the existing SSH tunnel.
Anonymous Grafana access is disabled. The initial administrator credential is
rotated without displaying it; day-to-day users receive Viewer access. A
future public dashboard requires its own threat model and review and is not
part of H8.

No new Hetzner firewall rule is needed. Ports 3000, 9090, 3100, 3200, 4317,
and 4318 remain non-public. If Grafana is ever exposed later, it requires an
exact reviewed hostname, cert-manager production TLS, permanent HTTPS
redirection, Traefik access controls, authenticated Viewer access, and a fresh
log/data disclosure review. Plain HTTP or anonymous public access is forbidden.

### 9.4 Sensitive telemetry and cardinality

Telemetry is treated as potentially sensitive even though the current demo is
small. Required deny rules include Authorization, Cookie, Set-Cookie, API
token, Secret content, token-file paths plus contents, request/response bodies,
environment dumps, registry credentials, Kubernetes Secret data, and user
messages. Query strings are omitted. Errors are classified and logged without
dumping raw upstream responses.

Metrics labels and Loki indexed labels must come from allowlists. Dynamic
resource names belong in logs and trace attributes only where needed. Dashboards
must not expose cluster bearer tokens, internal Secret names beyond already
documented resource identities, or raw authentication failures.

## 10. Dashboards and Alert Rules

Provision data sources, folders, dashboards, and Prometheus rule files from
Git. Do not make manual UI edits the source of truth.

### 10.1 Required dashboards

1. **Cluster and node overview:** node and namespace CPU/memory, pod readiness
   and restarts, root/PVC capacity where available, scrape health, and top
   consumers.
2. **Operator reconciliation:** reconcile rate, errors, duration percentiles,
   work queue depth, Deployment/Service failures, and ManagedService
   Available/Progressing/Degraded outcomes.
3. **Control-plane API:** readiness, request rate by normalized route/method/status,
   p50/p95/p99 latency, 401/429/5xx rates, in-flight requests, Kubernetes-client
   failures, and links from logs to traces.
4. **Managed workload:** desired and ready replicas, request rate/status/latency,
   probe state, restarts, logs by workload, and representative request traces.
5. **Logs-to-traces investigation:** correlated request ID, namespace,
   workload, severity, trace ID, and a direct Loki-to-Tempo link for one
   successful and one failed request, without indexed high-cardinality IDs.
6. **Observability self-health:** Collector accepted/refused/dropped telemetry
   and export failures; Prometheus targets and TSDB size; Loki ingestion,
   streams, compaction, and disk; Tempo received spans, compaction, and disk;
   Grafana health.

### 10.2 Initial rule groups

- node memory above 80% for 10 minutes and above 90% for 5 minutes;
- root disk above 70% warning and 80% critical, and PVC above 75%/85%;
- Pod OOM kill, restart loop, unavailable Deployment, or failed readiness;
- Prometheus target down for five minutes;
- operator reconciliation error rate or p95 duration above the measured
  baseline;
- API readiness failure, 5xx ratio, or p95 latency regression;
- Collector refused/dropped data or sustained exporter failures;
- Loki or Tempo ingestion/compaction failure and unexpected retention growth;
- observability component memory above 85% of its limit.

Prometheus evaluates the initial metric rules. Alertmanager is deferred to save
resources; firing state is visible in Prometheus and Grafana. A future external
notification contact requires a Secret-backed credential and a separate
review. During validation, trigger a safe synthetic rule and prove it returns
to normal without exhausting resources.

## 11. Implementation Slices

Each slice begins with node/PVC/Pod usage capture, a local render and security
review, and a reviewed manual Argo sync. Each ends with at least a 30-minute
soak, stop-threshold evaluation, and evidence capture. Never sync two new
backends together.

| Slice | Expected files or areas | Live mutation | Validation gate and rollback point | Added request / limit |
| --- | --- | --- | --- | ---: |
| H8.1 design and budget | `docs/h8-observability-design.md` only | forbidden | review evidence, arithmetic, and resize gate; rollback is document revert | 0 |
| H8.2 instrumentation foundation | `cmd/main.go`, `cmd/control-plane-api`, `cmd/demo-http`, `internal/controller`, `internal/controlplaneapi`, `internal/demohttp`, telemetry helpers and tests, plus later immutable image pins | forbidden in the implementation PR; publish and pin through the existing reviewed image process later | unit/envtest coverage for safe logs, request IDs, normalized metrics, propagation, redaction, and shutdown; rollback is source/image-pin revert | 0 until rollout |
| H8.3 Collector | future `deploy/observability` wrapper/values, Collector config, namespace/RBAC/NetworkPolicy, restricted AppProject/Application bootstrap | allowed only by reviewed manual Argo sync; no direct apply | Collector Ready, bounded memory, OTLP reachable only in-cluster, no backend exporter enabled yet; revert Git revision and manual sync | +25m / +96Mi request; +200m / +192Mi limit |
| H8.4 Prometheus | wrapper values, scrape configuration, rules directory, operator metrics binding, reduced kube-state-metrics values | manual Argo sync only | authenticated operator scrape 200, Collector/self and Kubernetes targets up, TSDB cap active, no forbidden labels; revert Prometheus values/revision and preserve PVC | +125m / +288Mi request; +500m / +576Mi limit |
| H8.5 Loki and logs | Loki values, 3Gi PVC configuration, Collector log pipeline, redaction and label allowlists | manual Argo sync only | operator/API/demo logs searchable, Collector self-log excluded, credentials absent, compactor/72h settings visible; disable Collector export then revert Loki revision, preserving PVC | +50m / +128Mi request; +250m / +256Mi limit |
| H8.6 Tempo and traces | Tempo values, 2Gi PVC configuration, Collector sampling/export pipeline, application/operator trace configuration | manual Argo sync only | one success and one representative failure trace, no sensitive attributes, zero drops; disable trace exporters then revert Tempo/instrumentation revision | +25m / +96Mi request; +200m / +192Mi limit |
| H8.7 Grafana and data sources | Grafana values, 1Gi PVC configuration, declarative Prometheus/Loki/Tempo data sources | manual Argo sync only | private port-forward works; no Ingress/public Service; Viewer/admin boundaries verified; revert Grafana revision and preserve PVC | +25m / +96Mi request; +150m / +192Mi limit |
| H8.8 dashboards and alert | versioned dashboard JSON and Prometheus rule files | manual Argo sync only; no new Pod | six dashboards load; a safe actionable rule fires and resolves; revert configuration revision | 0 |
| H8.9 end-to-end telemetry | test/evidence scripts or runbook additions only | bounded test traffic allowed after review; no direct resource mutation | request -> API log/metric/trace -> Kubernetes/operator reconciliation -> demo log/metric/trace is investigable; rollback to each prior known-good revision | 0 |
| H8.10 retention/resource/security validation | policy tests, rendered-budget checks, validation notes | read-only inspection over at least 72h; configuration corrections only through Git/manual sync | expiry, compaction, disk slope, memory/CPU, immutable images, RBAC, NetworkPolicy, Pod Security, and private exposure all pass; disable newest/noisiest pipeline on failure | 0 |
| H8.11 documentation closeout | README, infrastructure context, build guide, operator guide, H8 closeout/runbook | forbidden except separately reviewed GitOps changes already validated | all H8 exit criteria linked to sanitized evidence; docs revert has no runtime effect | 0 |

The order keeps Grafana last, so backend APIs and Prometheus can validate
signals without spending Grafana memory early. The Collector starts with no
durable exporters; Prometheus then observes it, Loki enables the log pipeline,
and Tempo enables traces. If the VM is not resized, stop after any slice that
reaches the memory gate. Do not trade away the 20% node reserve merely to
finish the diagram.

## 12. Go/No-Go Decision and H9 Gate

### Current 4 GB node

- CPU: **GO** for staged measurement; 250m requested is 12.5% of allocatable
  CPU and the live baseline is 6%.
- Disk: **GO** for the bounded 9Gi plan; current root usage is only 13.3%, but
  local-path has no independent failure domain or reliable quota.
- Memory: **NO-GO for the complete stack**. Requests project 83.6% node use,
  limits project 102%, existing system components are largely unbounded, and a
  rollout/debugging reserve would be lost.
- Security and delivery: **conditional GO** after rendered-chart, image-digest,
  RBAC, Pod Security, NetworkPolicy, and manual-Argo validation.

### Resize gate

Resize to at least 8GiB RAM before the complete six-Pod stack is enabled. After
resize, recapture the same baseline; do not assume memory use stays constant.
Proceed only if the existing platform plus the full 704Mi request budget leaves
at least 20% allocatable memory free and one 512Mi Prometheus-limit rollout
fits without surge.

H9 adds PostgreSQL, Redis, an API, and a dashboard. It must not begin on the
current 4 GB node, even if a partial H8 pilot appears idle. H9 needs a new
capacity and local-storage review after H8 has completed a retention-window
soak.

## 13. Validation Plan and Authoritative References

Implementation pull requests must validate:

- Helm dependency lock and artifact hashes;
- exact rendered image digests and registry allowlist;
- summed requests, limits, and PVC sizes;
- schema and Kubernetes API validity;
- no wildcard AppProject, RBAC, or NetworkPolicy permissions;
- manual Argo sync with no automated, prune, or self-heal policy;
- no Ingress, LoadBalancer, NodePort, hostPort, or hostNetwork;
- no Secret manifests or credential-like values;
- Pod Security settings and the single documented read-only hostPath exception;
- metric-label and Loki-label cardinality allowlists;
- retention and compaction settings;
- dashboards, data sources, and alert rules as code;
- live logs, metrics, successful and failure traces, secure operator scrape,
  disk bounds, and protected Grafana access.

Primary references consulted for this decision:

- [OpenTelemetry Helm charts](https://open-telemetry.github.io/opentelemetry-helm-charts/)
  and its [official index](https://open-telemetry.github.io/opentelemetry-helm-charts/index.yaml);
- [OpenTelemetry Collector Kubernetes components](https://opentelemetry.io/docs/platforms/kubernetes/collector/components/),
  including the preferred DaemonSet `filelog` pattern and Kubernetes metadata
  requirements;
- [Prometheus Community chart index](https://prometheus-community.github.io/helm-charts/index.yaml)
  and [Prometheus storage/retention](https://prometheus.io/docs/prometheus/latest/storage/);
- [Grafana Community Helm charts](https://github.com/grafana-community/helm-charts)
  and its [official index](https://grafana-community.github.io/helm-charts/index.yaml);
- [Loki Helm deployment guidance](https://grafana.com/docs/loki/latest/setup/install/helm/)
  and [monolithic mode](https://grafana.com/docs/loki/latest/setup/install/helm/install-monolithic/);
- [Tempo deployment modes](https://grafana.com/docs/tempo/latest/reference-tempo-architecture/deployment-modes/);
- [Grafana authentication guidance](https://grafana.com/docs/grafana/latest/setup-grafana/configure-access/configure-authentication/)
  and [roles and permissions](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/);
- [K3s local storage](https://docs.k3s.io/add-ons/storage) and
  [K3s NetworkPolicy controller](https://docs.k3s.io/networking/networking-services);
- [Argo CD automated sync semantics](https://argo-cd.readthedocs.io/en/stable/user-guide/auto_sync/),
  used here to keep automation, pruning, and self-heal absent.

This document resolves the H8 architecture and capacity decision. It does not
authorize a cluster synchronization, VM resize, Secret creation, chart
installation, or public exposure change.
