# H8.3D Collector Deployment and Validation Closeout

**Closeout date:** 2026-07-22

**Slice:** H8.3D — Collector installation, validation, and network isolation

**Result:** Complete

## Purpose and Scope

This document is the canonical, sanitized evidence record for the first live
H8 observability component. It records the GitOps deployment of the
OpenTelemetry Collector, its health and readiness, its restricted network
boundary, and the authoritative Gate 3V-R validation.

H8.3D closes only the Collector foundation. It does not complete H8, enable
workload OTLP export, or add Prometheus, Loki, Tempo, Grafana, durable storage,
or an external exporter. The Collector continued to use only the `nop`
exporter throughout validation, so the synthetic OTLP payloads were neither
persisted nor exported.

## Authoritative Revisions and Reviewed State

| Item | Authoritative value |
| --- | --- |
| Closeout branch and starting HEAD | `h8-observability-foundation` at `a7048b8396c2b083a2873f8d595b6b8eeaedecec` |
| Merged baseline | `origin/main` at `3469f6a0809f508fc81b21abe4fe33b9e565d7ce` |
| Deployed observability revision (Commit A) | `01c2d27b2bc671ce76686ce8d60c6a0c5b70b89d` |
| Argo CD Application | `Application/argocd/observability` |
| Application UID | `5b3b3aa9-f0c5-4a4b-9fad-d95f2e3372cc` |
| Application normalized specification hash | `7b023bc7a91605072671f902520b37d7a3af1a9f578cbc0b4838849f8ae8a01d` |
| Argo CD AppProject | `AppProject/argocd/observability` |
| AppProject specification hash | `b121e160d5f7b55af04e8db9feedbb75b5d3160c33f1be59416eddeed3700a0b` |

The Application targeted and observed exact Commit A and was `Synced` and
`Healthy`, with no conditions, active operation, automation, or pending prune.
Its explicitly reviewed `spec.source.helm.passCredentials: false` field was
present. The restricted AppProject, exact repository, destination, and manual
sync boundary remained unchanged.

The Application generation/resourceVersion advanced from `62/552251` to
`64/552690` during ordinary Argo controller status reconciliation. A
resourceVersion is evidence about an observed object, not an immutability
claim. Generation changes were assessed using the normalized specification
hash, semantic comparison, and the responsible field manager. Those checks
found no specification drift; `h8-observability-bootstrap` remained the only
specification manager and retained ownership of `passCredentials`.

## GitOps Deployment Validation

The staged deployment gates established that:

- the restricted `observability` AppProject admitted only the reviewed source,
  destination, and resource kinds;
- the inert, manual-sync `observability` Application used the reviewed Helm
  package and exact Commit A revision;
- synchronization created the reviewed seven-object inventory without
  automation, pruning, or self-healing;
- the Application remained `Synced` and `Healthy` after the explicit
  `passCredentials: false` field was restored through the reviewed manifest;
- no resource was pending pruning and no live operation remained active.

## Collector Health and Identity

The Collector remained continuously Running and Ready through Gate 3V-R:

| Evidence | Value |
| --- | --- |
| Collector Pod UID | `f7263e53-8ddc-408e-8516-0ee9532a6c08` |
| Collector Pod IP | `10.42.0.43` |
| Collector start time | `2026-07-22T12:49:27Z` |
| Container restarts | `0` |
| DaemonSet readiness | `1/1` |
| Resource request / limit | `25m`/`200m` CPU; `96Mi`/`192Mi` memory |
| Runtime image digest | `sha256:aa4509d8d72195c8576227fb33932788bc368fb0e81f6520f332130139236b91` |
| Collector Service ClusterIP | `10.43.91.255` |
| EndpointSlice UID | `d1fb7d02-ef98-45d6-9649-72806b3e41ef` |
| EndpointSlice revision | resourceVersion `543895`, generation `3` |
| Exposed receiver ports | TCP `4317` and `4318` only |
| Internal health port | TCP `13133`, not admitted from client Pods |
| Exporter | `nop` only |

The Collector Pod was not replaced or restarted. Its Service, EndpointSlice,
ConfigMap, NetworkPolicies, readiness/liveness probes, and all seven reviewed
resource identities remained stable. No Collector warning event or relevant
error was observed.

The seven-object identity set was:

| Resource | UID |
| --- | --- |
| `Namespace/observability` | `ab7af391-fb4c-4806-8424-8a75e3bf2128` |
| Collector ServiceAccount | `975b0ba0-38a0-4edd-95fc-1b404d5bcb33` |
| Collector ConfigMap | `ef5e8333-29d4-4d45-aad1-94a4c00bd935` |
| Collector Service | `71e2d3e4-41bb-4d75-8a89-b71e496f6ae9` |
| Collector DaemonSet | `c0dbb195-b674-4e8b-9dec-e9d8be673d02` |
| Default-deny NetworkPolicy | `7ae9b8d3-c8f7-491c-bd2a-48c1875f9a47` |
| OTLP-ingress NetworkPolicy | `549200e0-28fe-47b0-83d0-c21dc148105a` |

## NetworkPolicy Structural Review

The reviewed boundary permits only selector-authorized Pods in
`platform-system` and `applications` to reach the Collector on TCP 4317 or
4318. It does not authorize the platform operator, TCP 13133, another source
namespace, a public ingress, NodePort, LoadBalancer, host port, or host
networking. The five immutable diagnostic manifests had these SHA-256 hashes:

| Case manifest | SHA-256 |
| --- | --- |
| Authorized `platform-system` client | `0104f6a1363b3daeb0f093d978254f56922bbf836f435a71d3c30232cd1d93ea` |
| Unauthorized `platform-system` client | `0c2eb49003f9bda8a2b1215b3a28634edce6ac08873fed3dd14467b6b942b411` |
| Authorized `applications` client | `927924721bfb5f6a315b7bfd3164930f896f336f88f87279f7f699eba0d7fc32` |
| Unauthorized `applications` client | `6a4c47491738aebbd075008caefb58ddcea723ce759b6facf52910faa3ad9615` |
| Authorized identity against excluded TCP 13133 | `623f371160bc9f3db609cb86b5d90e5462e106cb81c8c293b5bc6ca1aab778fa` |

## Gate 3V Procedural History

The original Gate 3V execution remains **procedurally failed**. It is not
retroactively classified as passing. Its denied cases returned curl exit 7
instead of the originally expected timeout, leaving the procedure unable to
classify the result even though the connections did not succeed.

Gate 3V-D was a separate, read-only diagnosis. It established that the pinned
K3s kube-router NetworkPolicy dataplane uses a REJECT response for traffic that
is not admitted. A rejected TCP connection therefore returns an immediate
connection refusal and curl exit 7, rather than waiting for curl exit 28. This
is valid isolation evidence only when DNS resolves the intended target, the
same target is demonstrably healthy for an authorized control, the denied
client receives no HTTP response, and the production target remains healthy.

Gate 3V-R was a new, coherent execution using the unchanged reviewed manifests
and revised acceptance criteria: a denied case passed on either curl exit 28
(timeout) or curl exit 7 (explicit refusal), subject to the paired healthy
control and invariant checks. Gate 3V-R is the authoritative successful
execution.

## Gate 3V-R Evidence

All lifecycle timestamps below are UTC on 2026-07-22.

| Case | Pod and lifecycle | DNS and target | Result |
| --- | --- | --- | --- |
| Authorized platform client | `platform-system/h8-gate3v-platform-allow-01c2d27b`; created `15:04:24Z`, deleted `15:04:30Z` | Resolved Collector Service to `10.43.91.255`; TCP 4318 connected | OTLP/HTTP `200`; response `{"partialSuccess":{}}`; curl exit 0 |
| Unauthorized platform client | `platform-system/h8-gate3v-platform-deny-01c2d27b`; created `15:05:30Z`, deleted `15:05:36Z` | Resolved the same Service correctly; TCP 4318 was explicitly refused | No HTTP response; curl exit 7; pass under revised criteria |
| Authorized applications client | `applications/h8-gate3v-applications-allow-01c2d27b`; created `15:06:26Z`, deleted `15:06:32Z` | Resolved Collector Service to `10.43.91.255`; TCP 4318 connected | OTLP/HTTP `200`; expected success response; curl exit 0 |
| Unauthorized applications client | `applications/h8-gate3v-applications-deny-01c2d27b`; created `15:07:28Z`, deleted `15:07:33Z` | Resolved the same Service correctly; TCP 4318 was explicitly refused | No HTTP response; curl exit 7; pass under revised criteria |
| Excluded health port | `platform-system/h8-gate3v-platform-port-deny-01c2d27b`; created `15:08:33Z`, deleted `15:08:38Z` | Direct target `10.42.0.43:13133`; connection explicitly refused | No HTTP response; curl exit 7; pass under revised criteria |

Only one diagnostic Pod existed at a time, and no case was retried. The two
authorized controls prove the Collector Service and OTLP/HTTP receiver were
healthy from each permitted namespace. The two identity-denied cases prove
that the authorization label is required. The final case proves that an
authorized identity does not gain access to the policy-excluded health port.

## Cleanup and Production Invariants

All five diagnostic Pods were deleted and confirmed absent. No residual
`h8-gate3v-*` Pod, Job, ReplicaSet, Deployment, Service, ConfigMap, or
ServiceAccount remained.

Production workload inventory hashes were unchanged:

| Namespace | Inventory hash |
| --- | --- |
| `platform-system` | `beadd5559c1a6ec8681c2bfe5d70182d1274f852bfff56dabf945c76aa347ee9` |
| `applications` | `22bea96629a242ce6238743dcaefa3e9978d3d6fc14572ca8f4324197ac357f8` |

The Collector and all seven reviewed resources retained their UIDs. The
EndpointSlice retained its UID, resourceVersion, generation, and ready
`10.42.0.43` endpoint. No synchronization, pruning, forced conflict, Collector
rollout, production workload mutation, or Secret access occurred during Gate
3V-R.

## Warnings and Harmless Failures

- The local kubectl client and K3s server had the known minor-version skew.
  Admission and resource lifecycle operations completed normally, so this was
  recorded as non-blocking.
- An initial client-side manifest inspection omitted the explicit kubeconfig,
  failed harmlessly against localhost, and created no object. The inspection
  was repeated correctly with the explicit kubeconfig.

## Completion and Handoff

H8.3D is complete: the GitOps-managed Collector is healthy, its two authorized
OTLP/HTTP paths work, its two unauthorized identities are blocked, and TCP
13133 is blocked from client Pods. The Collector remains a bounded ingress
foundation with no durable or external exporter.

The exact next planned slice is **H8.4 — Prometheus**. Before any implementation
or live change, that slice requires its own bounded design and review of
scrape authentication, reduced kube-state-metrics scope, TSDB retention and
storage, resource headroom, immutable supply chain, restricted GitOps change,
and rollback. Workload OTLP enablement remains separately reviewed later work
and was not begun by this closeout.
