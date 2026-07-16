# Cloud-Native Service Control Plane — Operator Guide

> Status: Working guide, version 0.2  
> Last updated: 16 July 2026  
> Environment: Hetzner Cloud, Ubuntu 24.04 LTS, single-node K3s  
> This guide should be reviewed and finalized after the complete platform is deployed.

## 1. Purpose

This is the day-to-day connection and operating guide for the portfolio environment. It explains how to:

- connect to the Hetzner server;
- start and stop the private Kubernetes API tunnel;
- start and stop private Argo CD access;
- use `kubectl` from WSL;
- inspect and reconcile the GitOps Application;
- validate the server, cluster, DNS, and public ingress;
- recognize common connection failures;
- avoid exposing or committing administrative credentials.

It is an operator runbook, not the full architectural explanation. A separate system explanation and learning guide will be prepared when the platform is complete.

## 2. Current Environment

| Item | Current value |
| --- | --- |
| Project | Cloud-Native Service Control Plane |
| Hetzner server | `portfolio-k3s-01` |
| Operating system | Ubuntu 24.04 LTS |
| Server size | CX23, 2 vCPU, 4 GB RAM, 40 GB SSD |
| Kubernetes | Single-node K3s `v1.36.2+k3s1` |
| Ingress controller | Traefik |
| Certificate controller | cert-manager `v1.21.0` |
| GitOps controller | Argo CD `v3.4.5` |
| SSH user | `eoghan` |
| Local SSH alias | `portfolio-k3s` |
| Public IPv4 | `142.132.178.45` |
| Domain | `eoghanclancy.eu` |
| TLS validation hostname | `test.platform.eoghanclancy.eu` |
| Local kubeconfig | `~/.kube/portfolio-k3s.yaml` |
| Local Kubernetes API endpoint | `https://127.0.0.1:16443` |
| Remote Kubernetes API endpoint | `127.0.0.1:6443`, reached through SSH |
| SSH control socket | `~/.ssh/controlmasters/portfolio-k3s-tunnel.sock` |
| Local Argo CD endpoint | `https://127.0.0.1:18080`, available only while port-forwarding |

The server is intentionally a portfolio and learning environment. It is not a highly available production cluster.

## 3. Suggested Local Documentation Layout

Create the project workspace under `~/projects`:

```text
~/projects/cloud-native-service-control-plane/
├── README.md
├── docs/
│   ├── general-context.md
│   ├── infrastructure-context.md
│   ├── operator-guide.md
│   ├── argocd-sync-rollback-runbook.md
│   ├── system-explanation.md
│   ├── learning-guide.md
│   ├── architecture/
│   ├── decisions/
│   └── runbooks/
├── infrastructure/
├── kubernetes/
└── applications/
```

Recommended initial files:

- `docs/infrastructure-context.md`: the original project and infrastructure specification.
- `docs/operator-guide.md`: this document.
- `docs/argocd-sync-rollback-runbook.md`: exact synchronization, diagnosis, rollback, and Git reconciliation procedure.
- `docs/system-explanation.md`: how traffic, Kubernetes, storage, security, GitOps, and observability work together. Complete later.
- `docs/learning-guide.md`: concepts, commands, interview questions, and troubleshooting exercises. Complete later.
- `docs/decisions/`: short architecture decision records for important choices.
- `docs/runbooks/`: focused operational procedures such as certificate renewal or restoring K3s.

Create the folders from WSL:

```bash
cd ~/projects

mkdir -p cloud-native-service-control-plane/{docs/{architecture,decisions,runbooks},infrastructure,kubernetes,applications}

cd cloud-native-service-control-plane
```

Do not place private SSH keys, kubeconfig files, passwords, API tokens, certificate private keys, or unencrypted backups inside this project tree.

## 4. Administrative Access Model

There are three related administrative connections.

### 4.1 SSH shell access

```text
WSL terminal
    |
    | SSH on TCP 22 using a dedicated private key
    v
Hetzner firewall
    |
    v
Ubuntu server as user eoghan
```

Use this when working directly with Ubuntu or when running the K3s command-line tools on the server.

### 4.2 Local Kubernetes access

```text
Local kubectl
    |
    | https://127.0.0.1:16443
    v
SSH tunnel through portfolio-k3s
    |
    | remote 127.0.0.1:6443
    v
K3s Kubernetes API
```

Port `6443` is not exposed publicly. The local tunnel makes the remote Kubernetes API temporarily available only on WSL loopback port `16443`.

The SSH shell and the Kubernetes tunnel are related but independent. Closing an ordinary SSH shell does not necessarily close a background tunnel, and closing WSL stops the tunnel even though the server and K3s continue running.

### 4.3 Private Argo CD access

```text
Local Argo CD CLI or browser
    |
    | https://127.0.0.1:18080
    v
kubectl port-forward
    |
    v
ClusterIP service/argocd-server
```

The port-forward travels through the Kubernetes API connection described above. It does not expose Argo CD on the VM's public network. The Kubernetes API SSH tunnel must therefore be running before the Argo CD port-forward can work.

## 5. Connect to the Server

From WSL:

```bash
ssh portfolio-k3s
```

Enter the passphrase for `~/.ssh/hetzner_portfolio_ed25519` when prompted.

Confirm the remote identity:

```bash
whoami
hostnamectl --static
```

Expected results:

```text
eoghan
portfolio-k3s-01
```

End the server session:

```bash
exit
```

### Inspect the effective SSH alias

```bash
ssh -G portfolio-k3s | awk '$1 == "hostname" || $1 == "user" || $1 == "identityfile"'
```

The local SSH configuration should contain an entry equivalent to:

```sshconfig
Host portfolio-k3s
    HostName 142.132.178.45
    User eoghan
    IdentityFile ~/.ssh/hetzner_portfolio_ed25519
    IdentitiesOnly yes
    ServerAliveInterval 60
    ServerAliveCountMax 3
```

## 6. Start a Kubernetes Administration Session

### Step 1: configure the current WSL shell

The environment variable is shell-local and must be set again in a new terminal unless it is later added to a shell configuration file.

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
```

Confirm the file is protected:

```bash
stat -c '%a %n' "$KUBECONFIG"
```

Expected mode:

```text
600 /home/clancy/.kube/portfolio-k3s.yaml
```

### Step 2: ensure the control-socket directory exists

```bash
mkdir -p "$HOME/.ssh/controlmasters"
chmod 700 "$HOME/.ssh/controlmasters"
```

### Step 3: check whether the tunnel is already running

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O check \
  portfolio-k3s
```

If it reports that the master is running, do not start a second tunnel.

### Step 4: start the tunnel when it is not running

```bash
ssh \
  -M \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -fNT \
  -o ExitOnForwardFailure=yes \
  -L 127.0.0.1:16443:127.0.0.1:6443 \
  portfolio-k3s
```

Options used:

- `-M`: create an SSH control-master connection.
- `-S`: store its control socket at the documented path.
- `-f`: move the tunnel into the background after authentication.
- `-N`: do not execute a remote command.
- `-T`: do not allocate a terminal.
- `ExitOnForwardFailure=yes`: fail immediately if port forwarding cannot be created.
- `-L`: forward local `127.0.0.1:16443` to the server's `127.0.0.1:6443`.

### Step 5: validate Kubernetes access

```bash
kubectl config current-context
kubectl get nodes
kubectl get pods --all-namespaces
```

Expected context:

```text
portfolio-k3s
```

Expected node state:

```text
portfolio-k3s-01   Ready
```

## 7. Access Argo CD Privately

The matching CLI is installed at `~/.local/bin/argocd`. Configure the shell:

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
export PATH="$HOME/.local/bin:$PATH"
```

After validating Kubernetes access, open a second WSL terminal and keep this process in the foreground:

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"

kubectl port-forward \
  --namespace argocd \
  --address 127.0.0.1 \
  service/argocd-server \
  18080:443
```

The private UI is then available at:

```text
https://127.0.0.1:18080
```

The internal certificate produces a browser warning on this loopback endpoint. No public Argo CD hostname exists.

Validate CLI access from the original terminal:

```bash
argocd account get-user-info
argocd repo list
argocd app get registry-smoke
```

Stop only the Argo CD port-forward with `Ctrl+C` in its terminal. Argo CD and its Applications continue running in the cluster.

The complete synchronization, diagnosis, rollback, Git reconciliation, and credential-rotation procedure is in [`docs/argocd-sync-rollback-runbook.md`](argocd-sync-rollback-runbook.md).

## 8. Stop the Kubernetes Tunnel

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O exit \
  portfolio-k3s
```

Confirm that the local API port is no longer listening:

```bash
ss -lnt | grep ':16443 ' || echo 'Kubernetes tunnel stopped'
```

Stopping the tunnel does not stop K3s or any workload on the server. It only ends the private administrative path from the current WSL environment.

## 9. Routine Validation Commands

### 9.1 Local cluster checks through the tunnel

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"

kubectl get nodes -o wide
kubectl get pods --all-namespaces
kubectl get deployments --all-namespaces
kubectl get ingress --all-namespaces
kubectl get services --all-namespaces
kubectl get pvc --all-namespaces
kubectl get events --all-namespaces --field-selector type=Warning --sort-by='.lastTimestamp'
```

Resource use, once Metrics Server has collected data:

```bash
kubectl top nodes
kubectl top pods --all-namespaces --sort-by=memory
```

### 9.2 Server and K3s checks over SSH

```bash
ssh portfolio-k3s
```

Then on the server:

```bash
sudo systemctl is-active k3s
sudo systemctl --failed --no-pager
sudo k3s kubectl get nodes
sudo k3s kubectl get pods --all-namespaces
free -h
df -h /
sudo du -sh /var/lib/rancher/k3s
```

Recent K3s logs:

```bash
sudo journalctl -u k3s -n 100 --no-pager
```

Follow K3s logs temporarily:

```bash
sudo journalctl -u k3s -f
```

Press `Ctrl+C` to stop following logs. Do not restart K3s merely because an old warning appears in the journal; first inspect current pod and node health.

### 9.3 DNS validation

```bash
dig +short A test.platform.eoghanclancy.eu
dig +short AAAA test.platform.eoghanclancy.eu
dig +short A test.platform.eoghanclancy.eu @1.1.1.1
dig +short A test.platform.eoghanclancy.eu @8.8.8.8
```

Current expected result:

- the `A` queries return `142.132.178.45`;
- the `AAAA` query returns nothing because no IPv6 DNS record has been created.

### 9.4 Public ingress validation

```bash
curl -sS \
  -o /dev/null \
  -w 'HTTP status: %{http_code}\n' \
  http://test.platform.eoghanclancy.eu/
```

At the present stage, `404` is expected. It proves DNS resolves to the VM and Traefik receives the request, but no Ingress route exists for that hostname yet.

After TLS is configured, use:

```bash
curl -vI https://test.platform.eoghanclancy.eu/
```

### 9.5 Public exposure validation

From WSL:

```bash
SERVER_IP="$(ssh -G portfolio-k3s | awk '$1 == "hostname" {print $2; exit}')"

nmap -Pn \
  -p 22,80,443,6443,5432,6379,8080,9090 \
  "$SERVER_IP"

unset SERVER_IP
```

Intended public exposure:

| Port | Expected state | Purpose |
| --- | --- | --- |
| 22/TCP | Open only from the trusted administrative IP | SSH |
| 80/TCP | Open | HTTP, redirect, and ACME HTTP-01 |
| 443/TCP | Open | Public HTTPS ingress |
| 6443/TCP | Filtered | Kubernetes API must not be public |
| 5432/TCP | Filtered | PostgreSQL must not be public |
| 6379/TCP | Filtered | Redis must not be public |
| 8080/TCP | Filtered | Administrative/application service |
| 9090/TCP | Filtered | Prometheus |

## 10. Common Problems

### `kubectl` reports connection refused on `127.0.0.1:16443`

Example:

```text
The connection to the server 127.0.0.1:16443 was refused
```

Cause: the kubeconfig points to the local end of the SSH tunnel, but the tunnel is not running.

Fix:

1. Set `KUBECONFIG`.
2. Start the SSH tunnel using Section 6.
3. Run `kubectl get nodes` again.

### The SSH control socket already exists

Check it first:

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O check \
  portfolio-k3s
```

If the check succeeds, reuse the existing tunnel. If the check fails and no process is listening on local port `16443`, remove only the stale socket and start the tunnel again:

```bash
ss -lnt | grep ':16443 ' || true
rm -f "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock"
```

### SSH times out after the local public IP changes

Port 22 is restricted by the Hetzner Cloud Firewall. Determine the current WSL-visible public IP:

```bash
curl -4 https://ifconfig.me
```

Update the Hetzner firewall's SSH source to the new address with a `/32` suffix. Do not open SSH to the entire internet as a permanent workaround.

### SSH reports `Permission denied (publickey)`

Inspect the alias and key permissions:

```bash
ssh -G portfolio-k3s | awk '$1 == "hostname" || $1 == "user" || $1 == "identityfile"'
stat -c '%a %n' "$HOME/.ssh/hetzner_portfolio_ed25519"
```

The private key should normally have mode `600`.

For diagnostic output:

```bash
ssh -vv portfolio-k3s
```

Do not paste private-key material or complete authentication logs into public issues.

### DNS works but HTTP returns `404`

This normally means the request reached Traefik but no Ingress rule matched the hostname and path. Inspect:

```bash
kubectl get ingress --all-namespaces
kubectl describe ingress -n NAMESPACE INGRESS_NAME
```

### A pod is not becoming Ready

```bash
kubectl get pod -n NAMESPACE POD_NAME -o wide
kubectl describe pod -n NAMESPACE POD_NAME
kubectl logs -n NAMESPACE POD_NAME --all-containers
kubectl get events -n NAMESPACE --sort-by='.lastTimestamp'
```

For a container that restarted:

```bash
kubectl logs -n NAMESPACE POD_NAME --all-containers --previous
```

Replace the uppercase placeholders; do not run them literally.

## 11. Security Rules

- Never commit `~/.ssh/hetzner_portfolio_ed25519` or any private key.
- Never commit `~/.kube/portfolio-k3s.yaml`.
- Never paste kubeconfig contents into documentation, chat, or an issue.
- Never expose Kubernetes port `6443` publicly merely to avoid using the tunnel.
- Never expose PostgreSQL, Redis, Grafana, Prometheus, Loki, Tempo, or Argo CD directly without an explicit secured design.
- Do not store registrar, Hetzner, GitHub, ACME, or registry credentials in Markdown files.
- Use Kubernetes Secrets or a later secrets-management solution for runtime credentials.
- Keep the Porkbun account protected with application-based two-factor authentication, recovery information, domain lock, and auto-renewal.
- Treat server and Kubernetes changes as controlled operations: inspect, change one slice, validate, and document the result.

Suggested `.gitignore` entries:

```gitignore
# Administrative credentials
*.kubeconfig
kubeconfig
.kube/
*.pem
*.key
*.p12
*.pfx

# Environment files and local secrets
.env
.env.*
!.env.example
secrets/
*.secret.yaml

# Local tooling
.terraform/
*.tfstate
*.tfstate.*
```

Do not rely on `.gitignore` as permission to place secrets in the repository. Keep them outside the project tree whenever possible.

## 12. Current Platform Status

Completed:

- Hetzner project, server, SSH key, and cloud firewall;
- non-root administrator and hardened SSH configuration;
- Ubuntu updates and automatic security upgrades;
- AppArmor and journal limits;
- pinned K3s installation with secrets encryption;
- Traefik, CoreDNS, Metrics Server, and local-path storage validation;
- internal service DNS and PVC persistence test;
- private local `kubectl` access through SSH tunnelling;
- domain purchase and registrar account two-factor authentication;
- public DNS for `test.platform.eoghanclancy.eu`;
- cert-manager with Let's Encrypt staging and production issuers;
- trusted public TLS and permanent HTTP-to-HTTPS redirection;
- private GHCR publication and digest-pinned cluster deployment;
- Argo CD `v3.4.5` with private administrative access;
- read-only private repository registration;
- restricted AppProject and manually synchronized Application;
- tested operational rollback and Git reconciliation.

Later work will add the platform workloads, observability, persistent data services, backup procedures, monitoring, and final documentation.

## 13. Planned Final Documentation

At project completion, update this guide with:

- final hostnames and service ownership;
- final namespaces and deployed components;
- routine start, stop, upgrade, and maintenance procedures;
- certificate renewal checks;
- Argo CD access and reconciliation procedures;
- observability access and alert diagnosis;
- backup and restoration procedures;
- incident and resource-pressure runbooks;
- server resize procedure and post-resize validation;
- exact recovery steps for rebuilding the VM and K3s cluster;
- links to architecture decisions and application documentation.

The final `system-explanation.md` should explain the complete request, deployment, storage, telemetry, and GitOps flows. The final `learning-guide.md` should translate the implementation into concepts, exercises, likely interview questions, and concise explanations of design trade-offs.