# Cloud-Native Service Control Plane — Operator Guide

> Status: H7 operating guide, version 1.0
> Last updated: 20 July 2026
> Environment: Hetzner Cloud, Ubuntu 24.04 LTS, single-node K3s
> H7 is complete; observability, SmartEnergy, backup, and later roadmap phases remain future work.

## 1. Purpose

This is the day-to-day connection and operating guide for the portfolio environment. It explains how to:

- connect to the Hetzner server;
- start and stop the private Kubernetes API tunnel;
- start and stop private Argo CD access;
- use `kubectl` from WSL;
- inspect and reconcile the GitOps Application;
- operate the authenticated control-plane API without exposing its token;
- inspect `ManagedService` status and generated resources;
- rotate API and GHCR credentials safely;
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
| Control-plane API | `https://api.platform.eoghanclancy.eu` |
| Platform namespace | `platform-system` |
| Managed workload namespace | `applications` |
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
argocd app get platform-operator
argocd app get control-plane-api
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
dig +short A api.platform.eoghanclancy.eu
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

For the H7 API, verify permanent redirection without sending credentials:

```bash
curl -sS \
  -o /dev/null \
  -w 'HTTP status: %{http_code}\n' \
  http://api.platform.eoghanclancy.eu/healthz
```

Expected status: `308`.

Verify the production HTTPS health endpoint:

```bash
curl -fsS https://api.platform.eoghanclancy.eu/healthz
```

Expected body: `{"status":"healthy"}`.

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

## 10. Operate the H7 Platform

### 10.1 Check GitOps and workload health

Start the Kubernetes tunnel, set `KUBECONFIG`, and open the private Argo CD port-forward using Sections 6 and 7. Then check both platform Applications:

```bash
argocd app get platform-operator
argocd app get control-plane-api
```

Expected state for both is `Synced` and `Healthy`.

Inspect platform workloads without requesting Secret contents:

```bash
kubectl get deployment,pod,service,ingress,certificate \
  --namespace platform-system

kubectl get managedservices,deployments,pods,services \
  --namespace applications

kubectl get events \
  --all-namespaces \
  --field-selector type=Warning \
  --sort-by='.lastTimestamp'
```

The operator and API Pods should be Ready. `portfolio-demo` should remain active in `applications`, with its owned Deployment and Service ready.

### 10.2 Create a protected temporary API header

The API bearer token must not appear in shell history, process arguments, terminal output, documentation, or chat. Build a temporary curl header file directly from the Kubernetes Secret:

```bash
umask 077

API_SESSION_DIR="$(mktemp -d)"
API_HEADER_FILE="$API_SESSION_DIR/api-header"

install -m 600 /dev/null "$API_HEADER_FILE"
printf '%s: %s ' 'Authorization' 'Bearer' > "$API_HEADER_FILE"
kubectl get secret control-plane-api-token \
  --namespace platform-system \
  --output jsonpath='{.data.token}' \
  | base64 --decode >> "$API_HEADER_FILE"
printf '\n' >> "$API_HEADER_FILE"
chmod 600 "$API_HEADER_FILE"
```

Do not run `cat`, `head`, `tail`, `less`, `echo`, or tracing commands against this file. Do not enable `set -x` in this shell.

When finished, remove it explicitly:

```bash
rm -f "$API_HEADER_FILE"
rmdir "$API_SESSION_DIR"
unset API_HEADER_FILE API_SESSION_DIR
```

### 10.3 Call the API safely

Health and readiness are public:

```bash
curl --fail-with-body --silent --show-error \
  https://api.platform.eoghanclancy.eu/healthz

curl --fail-with-body --silent --show-error \
  https://api.platform.eoghanclancy.eu/readyz
```

An unauthenticated lifecycle request should return `401`:

```bash
curl --silent --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  https://api.platform.eoghanclancy.eu/api/v1/managed-services
```

Use the protected header file for authenticated calls. Create a temporary managed service:

```bash
curl --fail-with-body --silent --show-error \
  --request POST \
  --header @"$API_HEADER_FILE" \
  --header 'Content-Type: application/json' \
  --data '{"name":"guide-check","replicas":1,"message":"Hello from the operator guide"}' \
  https://api.platform.eoghanclancy.eu/api/v1/managed-services
```

List and get managed services:

```bash
curl --fail-with-body --silent --show-error \
  --header @"$API_HEADER_FILE" \
  https://api.platform.eoghanclancy.eu/api/v1/managed-services

curl --fail-with-body --silent --show-error \
  --header @"$API_HEADER_FILE" \
  https://api.platform.eoghanclancy.eu/api/v1/managed-services/guide-check
```

Delete the temporary service:

```bash
curl --fail-with-body --silent --show-error \
  --request DELETE \
  --header @"$API_HEADER_FILE" \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  https://api.platform.eoghanclancy.eu/api/v1/managed-services/guide-check
```

Expected deletion status: `204`.

Never copy command output and paste it back into the shell. Output is evidence to read, not a command to execute. In particular, never use command substitution around Secret, API, Argo CD, or diagnostic output unless a documented procedure explicitly requires it.

### 10.4 Check status and generated resources

Inspect the stable live demo:

```bash
kubectl get managedservice portfolio-demo \
  --namespace applications \
  --output wide

kubectl get managedservice portfolio-demo \
  --namespace applications \
  --output jsonpath='{.status.readyReplicas}{" ready replicas\n"}{range .status.conditions[*]}{.type}{"="}{.status}{" reason="}{.reason}{"\n"}{end}'

kubectl get deployment,service,pod \
  --namespace applications \
  --selector app.kubernetes.io/instance=portfolio-demo
```

Expected status includes `Available=True` and `readyReplicas=1`. If label selection does not return the children, inspect the named resources and owner references:

```bash
kubectl get deployment portfolio-demo \
  --namespace applications \
  --output jsonpath='{.metadata.ownerReferences}{"\n"}'

kubectl get service portfolio-demo \
  --namespace applications \
  --output jsonpath='{.metadata.ownerReferences}{"\n"}'
```

### 10.5 Synchronize platform Applications manually

Use a clean checkout and review the exact revision before synchronization:

```bash
cd ~/projects/cloud-native-service-control-plane
git status --short --branch
git pull --ff-only origin main

SYNC_REVISION="$(git rev-parse HEAD)"
printf 'Reviewed revision: %s\n' "$SYNC_REVISION"
```

Refresh and review differences:

```bash
argocd app get platform-operator --hard-refresh
argocd app diff platform-operator

argocd app get control-plane-api --hard-refresh
argocd app diff control-plane-api
```

Synchronize only the reviewed Application and revision:

```bash
argocd app sync platform-operator --revision "$SYNC_REVISION"
argocd app wait platform-operator --sync --health --timeout 180

argocd app sync control-plane-api --revision "$SYNC_REVISION"
argocd app wait control-plane-api --sync --health --timeout 180
```

Manual synchronization is intentional. Do not add `--prune`, enable automatic synchronization, or synchronize an unexplained diff for convenience.

### 10.6 Rotate the API token without displaying it

The API reads the token at process startup, so rotation requires an API rollout after replacing the Secret.

```bash
umask 077

TOKEN_WORK_DIR="$(mktemp -d)"
TOKEN_FILE="$TOKEN_WORK_DIR/token"

openssl rand -hex 32 | tr -d '\n' > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"

kubectl create secret generic control-plane-api-token \
  --namespace platform-system \
  --from-file=token="$TOKEN_FILE" \
  --dry-run=client \
  --output yaml \
  | kubectl apply -f -

kubectl rollout restart deployment/control-plane-api \
  --namespace platform-system

kubectl rollout status deployment/control-plane-api \
  --namespace platform-system \
  --timeout=120s

rm -f "$TOKEN_FILE"
rmdir "$TOKEN_WORK_DIR"
unset TOKEN_FILE TOKEN_WORK_DIR
```

Recreate the temporary API header using Section 10.2 and confirm that an authenticated request succeeds. Old header files must be deleted. Never print either token for comparison.

### 10.7 Copy or rotate namespace-scoped GHCR pull Secrets

`ghcr-pull` is namespace-scoped. `platform-system/ghcr-pull` cannot be used by Pods in `applications`.

To copy the currently approved Docker configuration without displaying it:

```bash
umask 077

PULL_WORK_DIR="$(mktemp -d)"
DOCKER_CONFIG_FILE="$PULL_WORK_DIR/config.json"

kubectl get secret ghcr-pull \
  --namespace platform-system \
  --output jsonpath='{.data.\.dockerconfigjson}' \
  | base64 --decode > "$DOCKER_CONFIG_FILE"

chmod 600 "$DOCKER_CONFIG_FILE"

kubectl create secret generic ghcr-pull \
  --namespace applications \
  --type kubernetes.io/dockerconfigjson \
  --from-file=.dockerconfigjson="$DOCKER_CONFIG_FILE" \
  --dry-run=client \
  --output yaml \
  | kubectl apply -f -

rm -f "$DOCKER_CONFIG_FILE"
rmdir "$PULL_WORK_DIR"
unset DOCKER_CONFIG_FILE PULL_WORK_DIR
```

For a complete rotation, create a new GitHub token with read-only package access outside the repository. Enter it silently and let Docker create a protected temporary configuration:

```bash
umask 077

PULL_WORK_DIR="$(mktemp -d)"
chmod 700 "$PULL_WORK_DIR"

read -r -s -p 'New read-only GHCR token: ' GHCR_READ_TOKEN
printf '\n'
printf '%s' "$GHCR_READ_TOKEN" \
  | docker --config "$PULL_WORK_DIR" login ghcr.io \
      --username YOUR_GITHUB_USERNAME \
      --password-stdin
unset GHCR_READ_TOKEN

for namespace in platform-system applications; do
  kubectl create secret generic ghcr-pull \
    --namespace "$namespace" \
    --type kubernetes.io/dockerconfigjson \
    --from-file=.dockerconfigjson="$PULL_WORK_DIR/config.json" \
    --dry-run=client \
    --output yaml \
    | kubectl apply -f -
done

rm -f "$PULL_WORK_DIR/config.json"
rmdir "$PULL_WORK_DIR"
unset PULL_WORK_DIR
```

Revoke the superseded registry token only after both namespace Secrets are updated and a controlled image-pull validation succeeds. Do not paste a token into a `kubectl --docker-password=...` argument because command arguments can be exposed through history or process inspection.

### 10.8 Diagnose the production API certificate

Start at the high-level Certificate and move down the ACME chain:

```bash
kubectl get certificate control-plane-api \
  --namespace platform-system

kubectl describe certificate control-plane-api \
  --namespace platform-system

kubectl get certificaterequest,order,challenge \
  --namespace platform-system

kubectl describe ingress control-plane-api \
  --namespace platform-system

kubectl get events \
  --namespace platform-system \
  --sort-by='.lastTimestamp'
```

Confirm DNS and public routing without requesting Secret data:

```bash
dig +short A api.platform.eoghanclancy.eu
curl -sSI http://api.platform.eoghanclancy.eu/healthz
curl -fsS https://api.platform.eoghanclancy.eu/healthz
```

The Certificate must use `letsencrypt-production`, exactly `api.platform.eoghanclancy.eu`, and Secret name `control-plane-api-tls`. Do not retrieve or print the TLS Secret.

## 11. Common Problems

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

Update the Hetzner firewall's SSH source to the new address with a `/32` suffix. Do not open SSH to the entire internet as a permanent workaround. If a temporary `/32` source is added while changing networks, remove the old or temporary source immediately after access is restored and verified.

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

## 12. Security Rules

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

## 13. Current Platform Status

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
- tested operational rollback and Git reconciliation;
- Kubebuilder v4.15.0 operator and `ManagedService` CRD;
- digest-pinned operator, API, and `demo-http` images;
- restricted `platform-control-plane` AppProject;
- manually synchronized `platform-operator` and `control-plane-api` Applications;
- authenticated lifecycle API at `https://api.platform.eoghanclancy.eu`;
- production certificate, permanent HTTPS redirect, and Traefik rate limit;
- live `portfolio-demo` with available status and tested drift correction;
- tested API creation and deletion with owner-driven garbage collection;
- restricted, manual-sync OpenTelemetry Collector deployment in
  `observability`;
- validated OTLP/HTTP access from both authorized workload identities and
  isolation of unauthorized identities and TCP 13133;
- Collector configured with only the `nop` exporter and no production workload
  OTLP export.

H7 platform deployment and H8.3 Collector foundation are complete. H8 remains
in progress; H8.4 Prometheus is next, while workload OTLP enablement remains a
separately reviewed later slice. The authoritative Collector validation record
is [`h8-collector-deployment-closeout.md`](h8-collector-deployment-closeout.md).
Later work will add the remaining observability backends, persistent data
services, backup procedures, and roadmap workloads.

## 14. Planned Final Documentation

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
