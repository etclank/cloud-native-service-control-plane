# H8.4C-KR2 Kubelet Certificate and NetworkPolicy Proof

> Superseded completion decision: the unresolved findings below remain valid,
> but kubelet and cAdvisor were subsequently removed from H8 acceptance by the
> deliberate
> [H8.4 final scope decision](h8-prometheus-scope-decision.md). They are
> optional post-H8 enhancements. H8.4C-KR3 and H8.4C-KI are cancelled, and
> this proof no longer blocks H8.4D.

H8.4C-KR2 retried the two shared proof properties that block the `kubelet`
and `cadvisor` Prometheus jobs:

1. exact kubelet serving-certificate identity and trust;
2. kube-router enforcement for ordinary Pod-to-local-node TCP 10250 traffic.

The exact configured SSH identity authenticated successfully. The session then
stopped at the required privilege boundary because `sudo -n` required a
password. Both properties remain **still unproven**. This batch changed no
scrape configuration, RBAC, NetworkPolicy, Helm value, application code,
dependency, Argo CD object, or live resource.

## Baseline

The repository started at
`a70292d4d19015f60fc3c53531d71ae6a8539758` on
`h8-observability-foundation`, with parent
`0a409518a337bb1a5f1456d52149afaa2b21d3d6` and `origin/main` at
`3469f6a0809f508fc81b21abe4fe33b9e565d7ce`. The default observability
render remained seven objects with SHA-256
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
- all seven observability resource UIDs unchanged;
- Collector Pod UID `f7263e53-8ddc-408e-8516-0ee9532a6c08`, Ready with
  zero restarts;
- no live Prometheus, kube-state-metrics, Prometheus PVC, node-scrape RBAC, or
  TCP 10250 policy.

No metrics endpoint was queried in H8.4C-KR2.

## SSH authentication result

The effective alias remained exact:

| Field | Value |
| --- | --- |
| Alias | `portfolio-k3s` |
| Host | `142.132.178.45` |
| User | `eoghan` |
| Port | `22` |
| IdentitiesOnly | `yes` |
| Configured key | `~/.ssh/hetzner_portfolio_ed25519` |

The configured public key and the loaded agent identity both reported the
expected ED25519 fingerprint
`SHA256:UZH/Hevwku3TViA+ZhZWLa4MmOvactH0maZ+iTd5n9w`. No private-key content
was read or displayed.

Exactly one SSH process was started:

```text
ssh -o BatchMode=yes -o PasswordAuthentication=no \
  -o KbdInteractiveAuthentication=no -o ConnectTimeout=10 \
  portfolio-k3s '<bounded read-only inspection script>'
```

Authentication succeeded. The only remote commands that completed were:

```text
hostname
id -un
sudo -n true
```

They reported host `portfolio-k3s-01`, user `eoghan`, and
`sudo: a password is required`. The script exited with status 90 at that
explicit guard. None of its certificate, listener, process, route, interface,
iptables, nftables, or kube-router inspection commands ran. A password was not
requested or entered, and no second SSH attempt was made.

## Certificate decision

Status: **still unproven**.

The authenticated session establishes the host and user boundary but provides
no certificate evidence. Because the non-interactive sudo guard failed before
inspection, H8.4C-KR2 did not inspect the actual TCP 10250 listener or perform
the permitted node-local TLS handshake. The following remain unknown:

- certificate subject, issuer, serial and SHA-256 fingerprint;
- not-before and not-after dates;
- key usage and extended key usage;
- DNS and IP SANs;
- whether `142.132.178.45` and `portfolio-k3s-01` are stable SANs;
- serving-certificate source and rotation contract;
- the exact trust anchor available through the standard projected
  ServiceAccount CA bundle;
- whether Prometheus requires `server_name`.

The earlier healthy metrics-server evidence continues to show that strict
InternalIP TLS works for that component. It cannot substitute for direct
listener metadata or prove Prometheus's exact trust configuration.
`insecure_skip_verify` remains unapproved.

## NetworkPolicy decision

Status: **still unproven**.

The session ended before host route and netfilter inspection. The previously
recorded K3s `v1.36.2+k3s1` and embedded kube-router `v2.6.3-k3s1` facts were
not re-observed over SSH. H8.4C-KR2 therefore cannot establish:

- the effective iptables or nftables backend and relevant live chains;
- how ordinary Pod egress enters kube-router policy evaluation;
- whether direct local-node delivery traverses or bypasses those chains;
- the policy-visible TCP 10250 destination;
- whether exact `142.132.178.45/32` TCP 10250 is meaningful and enforceable.

No traffic was sent. The earlier TCP 6443 API proof is not reused as proof for
the kubelet host process. No TCP 10250 policy is accepted or implemented.

## Authoritative decision

| Property | Status |
| --- | --- |
| ServiceAccount authentication and `nodes/metrics get` authorization | proven and accepted as a future design input |
| Bounded kubelet and cAdvisor metric-family candidates | proven and accepted as future design inputs |
| SSH alias, exact key identity, and host authentication | proven |
| Exact kubelet serving certificate and Prometheus trust | still unproven |
| kube-router Pod-to-local-node TCP 10250 enforcement | still unproven |
| `kubelet` job | deferred, not implemented |
| `cadvisor` job | deferred, not implemented |
| H8.4D GitOps registration | executable under the superseding six-target scope decision |

The earlier next-owner decision is superseded. H8.4C-KR3 and H8.4C-KI are
cancelled as H8 dependencies. A future optional node-target enhancement would
still need directly observed certificate and enforcement evidence, but no
further node proof is required for H8.4D.

Prometheus remains disabled and undeployed at this repository decision point.
H8 remains incomplete pending deployment, live validation, and closeout.
