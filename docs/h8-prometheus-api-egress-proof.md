# H8.4B0 Kubernetes API Egress Compatibility Proof

This record closes only the Kubernetes API egress compatibility prerequisite
for H8.4B. It does not deploy Prometheus, enable kube-state-metrics, or approve
the H8.4 runtime.

## Baseline and scope

The proof ran on 2026-07-23 from branch
`h8-observability-foundation` at
`8eefcd171b65d8ca18733c99ce0913dfa16ebd6d`, subject
`H8.4A Prometheus supply-chain foundation`. The merged baseline remained
`origin/main` at `3469f6a0809f508fc81b21abe4fe33b9e565d7ce`,
and deployed Collector Commit A
`01c2d27b2bc671ce76686ce8d60c6a0c5b70b89d` remained reachable.
The worktree was clean before discovery.

The temporary namespace was
`h8-prometheus-api-egress-proof-8eefcd1-a`. Only temporary namespaced
validation objects were created. No production resource was changed.

## Cluster and network identities

| Item | Observed identity |
| --- | --- |
| Kubernetes server | K3s `v1.36.2+k3s1`, Linux/amd64 |
| Node | `portfolio-k3s-01`; UID `f68ef7c8-16ba-493e-9ce0-88586f2a18d4`; architecture `amd64` |
| Node address relevant to the API | IPv4 InternalIP `142.132.178.45` |
| Kubernetes Service | `default/kubernetes`; UID `6d616b6b-d8d1-4583-a60e-abe34d911576`; ClusterIP `10.43.0.1`; TCP 443 targeting 6443 |
| Kubernetes Service evidence RV | `203`, recorded as volatile evidence only |
| API EndpointSlice | `default/kubernetes`; UID `24a23693-90b8-4c66-b5c8-0a658ddef83a`; generation 1 |
| Ready API backend | IPv4 `142.132.178.45`, TCP 6443, `ready=true` |
| EndpointSlice evidence RV | `206`, recorded as volatile evidence only |
| DNS Service | `kube-system/kube-dns`; UID `569a4b7c-e499-4eae-85bf-f9635c0b9ed6`; ClusterIP `10.43.0.10`; UDP/TCP 53 |
| CoreDNS Pod | `coredns-7fc5cb9848-zz225`; UID `d70a7697-5f5a-4063-aeb3-a58bdc4fcc69`; IP `10.42.0.9`; label `k8s-app=kube-dns` |

The node arguments exposed through the Node object showed Flannel VXLAN,
Service CIDR `10.43.0.0/16`, Pod CIDR `10.42.0.0/16`, and no
`--disable-network-policy` flag. The previously validated H8.3D Gate 3V-D
identified the embedded implementation as K3s kube-router
`v2.6.3-k3s1` and established that denied connections receive a netfilter
REJECT response. Existing repository policies select concrete Pods and ports;
none established whether API Service traffic was checked before or after DNAT.

## Diagnostic identity and reviewed manifests

Every Pod used the previously reviewed Linux/amd64 image:

```text
docker.io/curlimages/curl@sha256:1ab04d023ece37e6ec991bf3306ad04e0ef0084e94a5c6b6563cfcb9563169db
```

The reviewed image version was `8.21.0`; its manifest-list digest was
`sha256:7c12af72ceb38b7432ab85e1a265cff6ae58e06f95539d539b654f2cfa64bb13`.
It was already present on the amd64 node.

All Pods used `automountServiceAccountToken: false`, `restartPolicy: Never`,
a 30-second active deadline, RuntimeDefault seccomp, non-root UID/GID 100,
read-only root filesystems, dropped all capabilities, disabled privilege
escalation and privileged mode, disabled host namespaces, and had explicit
resource requests and limits. No token, Secret, credential ConfigMap,
host mount, projected volume, retry loop, or interactive execution was used.

| Reviewed manifest | SHA-256 |
| --- | --- |
| Namespace | `48f2d429fbfdeadd6de3ee4ea2dc703ab5dddeb0d9b04a44674986977b29646e` |
| Default-deny and CoreDNS policies | `04035b17671bf49433d03012d58b23f2a6a22826f246aa7e32e046b4a3e949b0` |
| Service-IP candidate policy | `f68f21477e5abc2fde88ea3e3abd0df5d76d1155f202701afcfb65321615c0c7` |
| Backend candidate policy | `54bd8e4978b47253ae54420152310b223440d790514e5f887f42f539bb2b42ba` |
| Case 1 DNS Pod | `8185e1cdfdd3e332dfb34795ae4a648e23edc8ed5af11079d75288f63ec684ef` |
| Case 2 denied-control Pod | `b43022084b1484a8a9e2ec27b8704c2dfd621eb61b11282553623d3f05f7b090` |
| Case 3 Service-IP Pod | `13f963f54eebf477de9a9f68a64fd7e5576758763d7fcd2d0813d57c092f0459` |
| Case 4 backend Pod | `0383030ed38c24a9b8a1eacdb0dc8d54e0310d8fb378e5e137a692db5ffb45d9` |

The manifests passed local structural and client-side schema checks before
use. The Namespace, both candidate API policies, and every Pod also passed an
exact server-side dry-run without conflicts. The two base policies were
client-validated and then applied from their already reviewed file.

## Compatibility results

Only one diagnostic Pod existed at a time. Each connection case made one
bounded attempt through `kubernetes.default.svc`; no case was retried.

| Case | Pod lifecycle (UTC) | Policies and DNS | Result |
| --- | --- | --- | --- |
| 1 — DNS control | Created `07:15:24Z`; finished `07:15:26Z` | Default deny plus exact CoreDNS Pod selector on UDP/TCP 53 | Resolved `kubernetes.default.svc` to `10.43.0.1`; `DNS_EXIT=0`; no API request |
| 2 — denied API control | Created `07:16:03Z`; finished `07:16:05Z` | DNS policy only; no API allowance | DNS succeeded; TCP 443 was explicitly refused; no HTTP response; `HTTP_CODE=000`; curl exit 7 |
| 3 — Service-IP candidate | Policy created `07:16:44Z`; Pod created `07:16:46Z`; finished `07:16:48Z` | Added only `10.43.0.1/32` TCP 443 | DNS succeeded, but TCP 443 was still explicitly refused; no HTTP response; `HTTP_CODE=000`; curl exit 7 |
| 4 — backend fallback | Policy created `07:17:46Z`; Pod created `07:17:47Z`; finished `07:17:50Z` | Service-IP policy was absent; added only `142.132.178.45/32` TCP 6443 | DNS still resolved the Service IP; TLS 1.3 completed; unauthenticated API response HTTP 401; `REMOTE_IP=10.43.0.1`; `REMOTE_PORT=443`; curl exit 0 |
| 5 — port exclusion | After Case 4 Pod deletion | Structural inspection of the live selected policy | Exactly one egress peer, `142.132.178.45/32`; exactly one port, TCP 6443; no `except`, range, second port, or second API policy |

The Pod API timestamps above are exact. Each completed Pod was immediately
deleted and confirmed absent before the next case. The deletion commands did
not independently print timestamps; their bounded windows were respectively
before the next creation at `07:16:03Z`, `07:16:44Z`, `07:17:46Z`, and the
cleanup start at `07:18:40Z`. This timestamp limitation does not affect the
connectivity classification or the one-Pod-at-a-time invariant.

## Decision

The preferred Service-IP rule failed while the exact ready backend rule passed.
For this K3s kube-router dataplane, egress policy is therefore evaluated
against the post-DNAT destination:

```yaml
ipBlock:
  cidr: 142.132.178.45/32
ports:
  - protocol: TCP
    port: 6443
```

This is the narrowest proven rule. It permits one address and one port and does
not require `10.43.0.0/16`, `10.42.0.0/16`, a node CIDR,
`0.0.0.0/0`, multiple destinations, or unrestricted egress. DNS remains a
separate CoreDNS Pod-selector rule on UDP/TCP 53.

The value is stable only while the single ready API EndpointSlice address and
port remain `142.132.178.45:6443`. Rediscover and review it after:

- node replacement or IPv4 readdressing;
- an API EndpointSlice address, readiness, or port change;
- adding or removing K3s server nodes;
- changing the Service CIDR, API bind/advertise address, embedded proxy, CNI,
  or NetworkPolicy implementation;
- a K3s upgrade that changes Service DNAT or kube-router enforcement.

## Cleanup and production stability

The temporary policies were deleted beginning at `07:18:40Z`. The namespace
was deleted and confirmed absent at `07:18:58Z`. No labeled proof Pod, Job,
ReplicaSet, Deployment, Service, ServiceAccount, ConfigMap, or NetworkPolicy
remained. Namespace deletion establishes that no namespaced temporary object
can remain without listing Secrets.

Before/after normalized hashes were byte-identical:

| Inventory | SHA-256 |
| --- | --- |
| `platform-system` workloads | `345a7ba6fac0ddbc5598683deb6cb6084b79eb24735deb5c90272c1d4d683dd9` |
| `applications` workloads | `5e43dbb14d0fc9e31e9bb738a6afae19c0b07639f2d1a570b99ff2daf4587806` |
| `Namespace/observability` | `429baff0c83a651b8f8c464606a29bff5bf393decdecd6db2b3d31f338714bf6` |
| Six namespaced objects completing the seven-object observability inventory | `ecb91e74c845e2f1a21c42511e88c37a0f0182320b413040dbd8489a19d1c72a` |

The Collector Pod retained UID
`f7263e53-8ddc-408e-8516-0ee9532a6c08`, remained ready, and retained zero
restarts. All seven reviewed resources retained their recorded UIDs and
normalized specifications. The Kubernetes Service and EndpointSlice retained
their UIDs, resourceVersions, generation, address, port, and readiness.

The observability Application remained Synced/Healthy at exact Commit A with
no active operation or conditions. Its generation advanced from 362 to 367
during ordinary application-controller reconciliation, but the normalized
specification remained the authoritative hash
`7b023bc7a91605072671f902520b37d7a3af1a9f578cbc0b4838849f8ae8a01d`,
including explicit `passCredentials: false`. No Application field or revision
changed during the proof.

## Warnings and explicit non-actions

The first read-only attempt to hash the six observability resources used
invalid comma-separated `resource/name` syntax and produced no object output;
the corrected explicit query produced the recorded hash. A later read-only
`jq` query expected suppressed `managedFields` and failed; the corrected query
used `--show-managed-fields` and produced the authoritative specification
hash. Neither command mutated state.

The initial resumed session also found the documented SSH control socket stale.
A run-specific tunnel attempt could not authenticate, created no listener, and
was not retried. The user then restored the standard private API tunnel
manually before discovery resumed.

No Secret was listed, mounted, retrieved, or decoded. No ServiceAccount token
was mounted. No interactive Pod execution, port-forward, Argo CD refresh or
synchronization, prune, force operation, production rollout, workflow action,
image publication, push, merge, external telemetry export, H8.4B runtime
implementation, or later H8 work occurred.

H8.4B0 proves only the exact Kubernetes API egress input. H8.4B and H8.4 remain
incomplete.
