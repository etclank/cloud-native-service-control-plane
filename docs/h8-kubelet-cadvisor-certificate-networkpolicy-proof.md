# H8.4C-KR Kubelet Certificate and NetworkPolicy Proof

H8.4C-KR attempted to close the two shared proof properties that still block
the `kubelet` and `cadvisor` Prometheus jobs:

1. exact kubelet serving-certificate identity and trust;
2. kube-router enforcement for ordinary Pod-to-local-node TCP 10250 traffic.

Both properties remain **still unproven**. The batch changed no scrape
configuration, RBAC, NetworkPolicy, Helm value, application code, dependency,
Argo CD object, or live resource.

## Baseline

The repository started at
`0a409518a337bb1a5f1456d52149afaa2b21d3d6` on
`h8-observability-foundation`. The default observability render remained seven
objects with SHA-256
`cb441f08f11f45e914bad39271fa7198e85a21382a598ea37ad4d1e5eebf2900`.
The disabled candidate remained 31 objects with SHA-256
`307f390cedb0f147fdf88c45ae83a184055c4d0d0269e3257591903ea486f0d5`
and exactly the original six H8.4C jobs.

Read-only Kubernetes preflight reconfirmed:

- context `portfolio-k3s` and healthy `/readyz`;
- Node `portfolio-k3s-01`, UID
  `f68ef7c8-16ba-493e-9ce0-88586f2a18d4`;
- K3s `v1.36.2+k3s1`;
- IPv4 InternalIP `142.132.178.45`;
- advertised kubelet TCP 10250;
- Argo CD Synced/Healthy at Collector Commit A with no active operation;
- Collector 1/1 Ready with zero restarts;
- no live Prometheus, kube-state-metrics, Prometheus PVC, node-scrape RBAC, or
  TCP 10250 policy.

No metrics endpoint was queried in H8.4C-KR.

## SSH recovery gate

Only the permitted local metadata was inspected. No key content or broad SSH
configuration was displayed.

The effective alias was unambiguous:

| Field | Value |
| --- | --- |
| Alias | `portfolio-k3s` |
| Host | `142.132.178.45` |
| User | `eoghan` |
| Port | `22` |
| IdentitiesOnly | `yes` |
| Configured key | `~/.ssh/hetzner_portfolio_ed25519` |

The configured private-key file existed with mode 600, and its public-key file
existed with mode 644. Only public-key fingerprints were inspected:

| Identity | ED25519 SHA-256 fingerprint |
| --- | --- |
| Configured `hetzner-portfolio` public key | `SHA256:UZH/Hevwku3TViA+ZhZWLa4MmOvactH0maZ+iTd5n9w` |
| Only identity reported by the SSH agent | `SHA256:r2aP6c1jw5kcvOaJQPikUNb2VDzzIjywLQpICflPCPw` |

The fingerprints did not match. The authorization allowed one controlled SSH
retry only if the agent reported the matching configured key. Therefore no SSH
authentication retry was made, no host session opened, and no host command ran.
The agent, key files, SSH configuration, and server `authorized_keys` were not
changed.

## Certificate decision

Status: **still unproven**.

Because the controlled SSH gate did not open, H8.4C-KR did not inspect the
actual TCP 10250 listener or serving certificate. The following remain unknown:

- subject, issuer, serial and fingerprint;
- not-before and not-after dates;
- key usage and extended key usage;
- DNS and IP SANs;
- whether `142.132.178.45` and `portfolio-k3s-01` are SANs;
- the exact trust anchor available to Prometheus;
- whether `server_name` is required;
- whether certificate rotation preserves the identity and issuer contract.

The earlier metrics-server evidence still shows that strict InternalIP TLS
works for that component. It cannot substitute for the required certificate
metadata or establish the exact Prometheus trust configuration.
`insecure_skip_verify` remains unapproved.

## NetworkPolicy decision

Status: **still unproven**.

No host rule, route, interface, iptables, nftables, or kube-router inspection
ran. Therefore H8.4C-KR cannot establish:

- kube-router's effective backend and relevant chains;
- whether ordinary Pod-to-local-node traffic is policy evaluated;
- the policy-visible destination for TCP 10250;
- whether local host delivery bypasses the intended boundary;
- whether exact `142.132.178.45/32` TCP 10250 is meaningful and enforceable.

No traffic was sent. The earlier TCP 6443 API proof is not reused as proof for
the kubelet host process. No TCP 10250 policy is accepted or implemented.

## Authoritative decision

| Property | Status |
| --- | --- |
| ServiceAccount authentication and `nodes/metrics get` authorization | proven and accepted as a future design input |
| Bounded kubelet and cAdvisor metric-family candidates | proven and accepted as future design inputs |
| Exact kubelet serving certificate and Prometheus trust | still unproven |
| kube-router Pod-to-local-node TCP 10250 enforcement | still unproven |
| `kubelet` job | deferred, not implemented |
| `cadvisor` job | deferred, not implemented |
| H8.4D GitOps registration | blocked |

The concrete recovery path is for the user to restore the configured
`hetzner-portfolio` key fingerprint in the SSH agent outside this batch. The
next dependency-ordered batch is **H8.4C-KR2 SSH-authenticated certificate and
policy inspection follow-up**. It may make one controlled SSH attempt after
reconfirming the matching fingerprint and then run only the previously
reviewed read-only certificate and network-rule commands.

H8.4C-KI is not executable. H8.4D remains blocked while either mandatory node
job is deferred. Prometheus remains disabled, undeployed, and incomplete.
