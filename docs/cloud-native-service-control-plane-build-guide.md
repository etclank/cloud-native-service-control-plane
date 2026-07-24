# Cloud-Native Service Control Plane — Build and Learning Guide

> Status: Complete through H8
> Last updated: 24 July 2026
> Target: Hetzner Cloud, Ubuntu 24.04 LTS, single-node K3s
> Purpose: Explain the build, preserve the commands, and provide a reproducible reconstruction path.

## 1. How to Use This Document

This guide records how the public portfolio platform was built and explains what each step does. It serves three purposes:

1. A revision guide for understanding the infrastructure after completing each phase.
2. A reconstruction guide if the VM or cluster must be rebuilt.
3. An evidence trail for explaining the design in a technical interview.

The guide currently covers:

- H1 — Hetzner project and VM provisioning;
- H2 — operating-system hardening;
- H3 — K3s installation and private administration;
- H4 — DNS and TLS foundation;
- H5 — private GHCR and immutable delivery;
- H6 — private Argo CD and tested rollback;
- H7 — operator, control-plane API, managed workload, GitOps, and production TLS;
- H8.1–H8.4 — observability design, telemetry foundations, the validated
  OpenTelemetry Collector boundary, and bounded Prometheus.

The accepted construction roadmap ends at H8. Optional improvements may add
their own implementation records after validation, but they are not required
to complete this baseline. Commands are grouped by where they run.

| Marker | Run the command in |
| --- | --- |
| **Hetzner Console** | The Hetzner Cloud web interface |
| **WSL** | The local Ubuntu environment on the Windows computer |
| **Server** | An SSH shell on the Hetzner Ubuntu VM |
| **Kubernetes** | Submitted with local `kubectl` through the SSH tunnel |
| **Porkbun** | The domain registrar and DNS web interface |

Do not blindly replay commands against an existing environment. Read the explanation and inspect the current state first.

## 2. Security Rules

Never place any of the following in this repository or document:

- private SSH keys;
- kubeconfig contents;
- GitHub or registry tokens;
- Hetzner API tokens;
- domain-provider API tokens;
- Kubernetes Secret values;
- TLS or ACME private keys;
- database passwords;
- unencrypted backups.

Safe items that may be documented include public hostnames, public IP addresses, public keys, resource names, non-secret manifests, versions, and validation output with secret data removed.

The current public IPv4 address is `142.132.178.45`. A rebuilt VM may receive a different address, so DNS and the SSH alias must be updated after reconstruction.

## 3. Implemented Architecture Through H8

```text
                                  Public Internet
                                         |
          test.platform.eoghanclancy.eu and api.platform.eoghanclancy.eu
                                         |
                          A record: 142.132.178.45
                                         |
                              Hetzner Cloud Firewall
                          TCP 80 and 443 from the internet
                          TCP 22 from trusted admin IP only
                                         |
                              portfolio-k3s-01 VM
                              Ubuntu 24.04 LTS
                                         |
                            K3s single-node cluster
                                         |
                                  Traefik
                                         |
                  HTTP redirect / HTTPS termination / rate limit
                                         |
                 +-----------------------+-----------------------+
                 |                                               |
       tls-validation Service                         control-plane API
                                                                 |
                                                       Kubernetes API
                                                                 |
                                                     ManagedService CR
                                                                 |
                                                          operator
                                                                 |
                                               owned Deployment + Service


Local administration:

kubectl in WSL
      |
      | https://127.0.0.1:16443
      v
foreground SSH tunnel over TCP 22
      |
      | remote 127.0.0.1:6443
      v
private K3s Kubernetes API
```

Only ports 80 and 443 are public application endpoints. Port 22 is restricted to the trusted administrative source IP. Kubernetes port 6443 is not public.

## 4. Current Resource Inventory

| Resource | Current value |
| --- | --- |
| Hetzner project | `cloud-native-service-control-plane` |
| Server | `portfolio-k3s-01` |
| Server type | CX23, shared cost-optimized x86 |
| CPU and memory | 2 vCPU, 4 GB RAM |
| Root disk | 40 GB SSD |
| Operating system | Ubuntu 24.04 LTS |
| Administrative user | `eoghan` |
| Local SSH alias | `portfolio-k3s` |
| SSH key name in Hetzner | `wsl-hetzner-portfolio-2026` |
| Local private key | `~/.ssh/hetzner_portfolio_ed25519` |
| Public IPv4 | `142.132.178.45` |
| Kubernetes | K3s `v1.36.2+k3s1` |
| Ingress | Bundled Traefik |
| Storage | Bundled local-path provisioner |
| Certificate controller | cert-manager `v1.21.0` |
| Domain | `eoghanclancy.eu` |
| TLS validation hostname | `test.platform.eoghanclancy.eu` |
| Control-plane API hostname | `api.platform.eoghanclancy.eu` |
| GitOps | Private Argo CD `v3.4.5`, manual sync |
| Operator API | `platform.eoghanclancy.eu/v1alpha1` |
| Platform namespace | `platform-system` |
| Managed workloads | `applications` |
| Local kubeconfig | `~/.kube/portfolio-k3s.yaml` |
| Local tunnel port | `127.0.0.1:16443` |

No extra Hetzner volume, placement group, or highly available storage is
currently used. Automated backup and full disaster-recovery exercises are
optional production-hardening work.

---

# H1 — Hetzner Project and VM Provisioning

## H1.1 Objective

Create the smallest practical public server, give it controlled administrative access, and expose only the ports needed for HTTP and HTTPS.

The CX23 was deliberately selected for the bootstrap because this is a low-traffic portfolio environment. K3s and the early infrastructure fit comfortably, but the complete observability and data stack may require resizing later. Hetzner allows the server resources to be increased without rebuilding the software configuration.

## H1.2 Local SSH Cleanup and Dedicated Key

Earlier Hetzner-specific SSH configuration was removed before starting again. The normal personal key was retained, but the new portfolio server received its own dedicated key.

**WSL:** generate the dedicated key:

```bash
ssh-keygen \
  -t ed25519 \
  -a 100 \
  -f "$HOME/.ssh/hetzner_portfolio_ed25519" \
  -C "wsl-hetzner-portfolio-2026"
```

Explanation:

- `ed25519` is a modern SSH key type;
- `-a 100` increases private-key passphrase derivation work;
- `-f` gives this server a dedicated key filename;
- `-C` adds a non-secret label to the public key.

Protect the local files:

```bash
chmod 700 "$HOME/.ssh"
chmod 600 "$HOME/.ssh/hetzner_portfolio_ed25519"
chmod 644 "$HOME/.ssh/hetzner_portfolio_ed25519.pub"
```

Display only the public key when adding it to Hetzner:

```bash
cat "$HOME/.ssh/hetzner_portfolio_ed25519.pub"
```

Never display or copy the private file without the `.pub` suffix.

## H1.3 Hetzner Project, SSH Key, and Firewall

**Hetzner Console:** create:

```text
Project: cloud-native-service-control-plane
SSH key: wsl-hetzner-portfolio-2026
Firewall: portfolio-edge
```

The initial inbound firewall policy is:

| Protocol | Port | Source | Reason |
| --- | --- | --- | --- |
| TCP | 22 | Trusted public IPv4 with `/32` | SSH administration |
| TCP | 80 | All IPv4 and IPv6 | HTTP and ACME HTTP-01 |
| TCP | 443 | All IPv4 and IPv6 | Public HTTPS applications |

No other unsolicited inbound ports are permitted. Outbound traffic remains allowed so the server can install packages, pull container images, resolve DNS, and contact external services.

Important private ports include:

```text
6443  Kubernetes API
5432  PostgreSQL
6379  Redis
8080  Administrative/application services
9090  Prometheus
3100  Loki
3200  Tempo
4317  OTLP gRPC
4318  OTLP HTTP
```

These services must not be exposed directly through the Hetzner firewall.

## H1.4 VM Creation

**Hetzner Console:** create the server with the following settings:

```text
Name: portfolio-k3s-01
Image: Ubuntu 24.04 LTS
Architecture: x86-64
Type: CX23 cost-optimized shared resources
Resources: 2 vCPU, 4 GB RAM, 40 GB SSD
Networking: public IPv4 and IPv6
SSH key: wsl-hetzner-portfolio-2026
Firewall: portfolio-edge
Additional volume: none
Backups: no automated backup system in the current baseline
Placement group: none for a single-node environment
```

The original region should be read from the Hetzner Console before recreation because it was not recorded in the first revision of this guide.

Why x86 was selected:

- widest container-image compatibility;
- fewer multi-architecture surprises during portfolio development;
- simple migration to larger x86 server types.

Why no extra volume was selected:

- the early workload fits on the root disk;
- K3s local storage remains simple;
- backup and storage expansion decisions are deferred until real data requirements exist.

## H1.5 First Connection and Host Validation

The first connection initially used the root account created by the cloud image:

```bash
ssh -i "$HOME/.ssh/hetzner_portfolio_ed25519" root@142.132.178.45
```

On the server, the initial validation included:

```bash
whoami
hostnamectl
cat /etc/os-release
cloud-init status
free -h
df -h /
nproc
timedatectl
systemctl is-active ssh
ss -lntup
systemctl --failed --no-pager
```

The important results were:

- Ubuntu 24.04 LTS;
- hostname `portfolio-k3s-01`;
- cloud-init complete;
- approximately 3.7 GiB usable RAM;
- approximately 38 GiB root filesystem;
- SSH active on port 22;
- no failed services.

## H1.6 Non-root Administrator

**Server, initially as root:** create the administrator:

```bash
adduser eoghan
usermod -aG sudo eoghan
```

The account has a password because `sudo` requests it locally. SSH password authentication is disabled later, so this password is not accepted for remote login.

Install the existing authorized public key for the new user:

```bash
install -d \
  -m 700 \
  -o eoghan \
  -g eoghan \
  /home/eoghan/.ssh

install \
  -m 600 \
  -o eoghan \
  -g eoghan \
  /root/.ssh/authorized_keys \
  /home/eoghan/.ssh/authorized_keys
```

Validate:

```bash
id eoghan
sudo -l -U eoghan
namei -l /home/eoghan/.ssh/authorized_keys
```

Open a second WSL terminal before closing the root session:

```bash
ssh \
  -i "$HOME/.ssh/hetzner_portfolio_ed25519" \
  eoghan@142.132.178.45
```

Then validate:

```bash
whoami
sudo -v
sudo whoami
```

Expected final result:

```text
eoghan
root
```

This proves that SSH and sudo work before root access is disabled.

## H1.7 Local SSH Alias

**WSL:** add the following to `~/.ssh/config`:

```sshconfig
Host portfolio-k3s
    HostName 142.132.178.45
    User eoghan
    IdentityFile ~/.ssh/hetzner_portfolio_ed25519
    IdentitiesOnly yes
    ServerAliveInterval 60
    ServerAliveCountMax 3
```

Protect and test the configuration:

```bash
chmod 600 "$HOME/.ssh/config"
ssh -G portfolio-k3s | awk '$1 == "hostname" || $1 == "user" || $1 == "identityfile"'
ssh portfolio-k3s
```

The alias prevents repeated typing of the IP, username, and key path. If the VM is rebuilt with a different IP, only `HostName` needs to change.

## H1.8 External Firewall Validation

**WSL:** resolve the configured server IP and scan selected ports:

```bash
SERVER_IP="$(ssh -G portfolio-k3s | awk '$1 == "hostname" {print $2; exit}')"

nmap -Pn \
  -p 22,80,443,6443,5432,6379,8080,9090 \
  "$SERVER_IP"

unset SERVER_IP
```

Before K3s, only SSH was expected to have a listening application. After K3s installed Traefik, ports 80 and 443 became open. Restricted ports remained filtered.

## H1 Exit Criteria

- VM starts successfully.
- Dedicated key authentication works.
- Non-root sudo administration works.
- SSH alias works.
- Firewall is attached.
- Only approved inbound ports are permitted.

---

# H2 — Operating-System Hardening

## H2.1 Objective

Bring Ubuntu fully up to date, remove direct privileged remote access, enable automatic security maintenance, preserve mandatory access controls, and bound host log growth.

## H2.2 Package Updates and Reboot

**Server:**

```bash
sudo apt-get update
sudo apt-get full-upgrade -y
sudo apt-get autoremove --purge -y
```

Inspect the result:

```bash
apt list --upgradable
systemctl --failed --no-pager
test -f /var/run/reboot-required && cat /var/run/reboot-required || echo "No reboot required"
```

A kernel update required a reboot. Reboot without keeping the SSH shell open:

```bash
sudo reboot
```

Reconnect after the VM returns:

```bash
ssh portfolio-k3s
uname -r
```

The validated kernel was:

```text
6.8.0-134-generic
```

## H2.3 SSH Hardening

**Server:** create `/etc/ssh/sshd_config.d/00-portfolio-hardening.conf` with:

```text
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
PermitRootLogin no
AllowUsers eoghan
MaxAuthTries 3
LoginGraceTime 30
X11Forwarding no
```

Use a privileged editor:

```bash
sudoedit /etc/ssh/sshd_config.d/00-portfolio-hardening.conf
```

Validate before reloading:

```bash
sudo sshd -t
sudo systemctl reload ssh
sudo systemctl is-active ssh
```

Open a second terminal and confirm the normal alias still works before ending the existing session:

```bash
ssh portfolio-k3s
```

Confirm direct root access is rejected:

```bash
ssh \
  -i "$HOME/.ssh/hetzner_portfolio_ed25519" \
  root@142.132.178.45
```

Why this matters:

- a stolen or guessed password cannot authenticate over SSH;
- attackers cannot directly target the root login;
- administrative actions are attributed to `eoghan` and elevated through `sudo`;
- `AllowUsers` limits the SSH service to the intended administrator.

## H2.4 Automatic Security Updates

**Server:** install and enable unattended upgrades:

```bash
sudo apt-get install -y unattended-upgrades
sudo dpkg-reconfigure -plow unattended-upgrades
```

Ensure automatic rebooting is disabled so upgrades do not unexpectedly interrupt a demonstration. In a local APT configuration drop-in, use:

```text
Unattended-Upgrade::Automatic-Reboot "false";
```

Validate the configuration and timers:

```bash
sudo unattended-upgrade --dry-run --debug
systemctl is-enabled unattended-upgrades
systemctl is-enabled apt-daily-upgrade.timer
systemctl is-active unattended-upgrades
systemctl is-active apt-daily-upgrade.timer
systemctl list-timers apt-daily.timer apt-daily-upgrade.timer --no-pager
```

The dry-run returned exit code zero and both APT timers were enabled and active.

## H2.5 AppArmor

AppArmor was already available in the Ubuntu image and remains enabled. It provides mandatory access-control profiles in addition to normal Unix permissions.

Validate:

```bash
sudo aa-enabled
systemctl is-enabled apparmor
systemctl is-active apparmor
sudo aa-status
```

Do not disable AppArmor merely because a generic installation guide suggests doing so. Any future incompatibility should be diagnosed and solved narrowly.

## H2.6 Host Firewall Decision

The host UFW firewall remains inactive. This is deliberate:

- the Hetzner Cloud Firewall provides the public perimeter policy;
- K3s, Flannel, ServiceLB, and Traefik manage Linux networking and packet-filter rules;
- independently enabling UFW without a tested Kubernetes rule set can break pod or Service networking.

Current validation:

```bash
sudo ufw status 2>/dev/null || echo "UFW is not installed or inactive"
```

This does not mean the VM is unprotected. The external Hetzner firewall remains authoritative for unsolicited public traffic.

## H2.7 Journal Limits

To prevent logs filling the small root disk, create `/etc/systemd/journald.conf.d/portfolio-limits.conf`:

```ini
[Journal]
SystemMaxUse=500M
RuntimeMaxUse=100M
MaxRetentionSec=7day
MaxFileSec=1day
Compress=yes
```

Apply and inspect:

```bash
sudo systemctl restart systemd-journald
journalctl --disk-usage
sudo systemd-analyze cat-config systemd/journald.conf
```

The limits provide useful recent diagnostic history while bounding disk use.

## H2.8 Host Baseline Validation

```bash
echo "=== AppArmor ==="
sudo aa-enabled
systemctl is-active apparmor

echo
echo "=== Journal usage ==="
journalctl --disk-usage

echo
echo "=== Listening ports ==="
sudo ss -lntup

echo
echo "=== Storage and memory ==="
df -h /
free -h

echo
echo "=== Failed services ==="
systemctl --failed --no-pager
```

At the H2 baseline:

- only SSH and local DNS listeners were present;
- root-disk use was approximately 1.7 GB;
- available memory was approximately 3.3 GB;
- no host services were failed;
- no swap was configured.

## H2 Exit Criteria

- Ubuntu packages and kernel are current.
- SSH uses keys only.
- direct root SSH is disabled.
- `eoghan` sudo access works.
- automatic security updates are active.
- AppArmor remains active.
- journal growth is bounded.
- external firewall behavior is understood.

---

# H3 — K3s Installation and Private Administration

## H3.1 Objective

Install a pinned, reproducible single-node Kubernetes environment with ingress, internal DNS, metrics, local persistent storage, encrypted Kubernetes Secrets at rest, and a private administrative API.

K3s packages the Kubernetes control plane and common supporting components into a lightweight distribution suited to a small server.

## H3.2 Prerequisite Inspection

**Server:**

```bash
uname -m
stat -fc %T /sys/fs/cgroup
mount | grep cgroup

for module in overlay br_netfilter vxlan; do
    if sudo modprobe "$module"; then
        echo "$module: available"
    else
        echo "$module: FAILED"
    fi
done

sysctl net.ipv4.ip_forward
sysctl net.bridge.bridge-nf-call-iptables
sysctl net.bridge.bridge-nf-call-ip6tables
nproc
free -h
df -h /
```

The real Hetzner VM baseline was:

- `x86_64`;
- cgroup v2;
- required modules available;
- 2 vCPU;
- approximately 3.7 GiB RAM;
- approximately 35 GiB available disk;
- IPv4 forwarding initially disabled.

## H3.3 Persistent Kernel Modules and Network Settings

Create `/etc/modules-load.d/k3s.conf`:

```text
overlay
br_netfilter
vxlan
```

Create `/etc/sysctl.d/90-k3s.conf`:

```text
net.ipv4.ip_forward=1
net.bridge.bridge-nf-call-iptables=1
net.bridge.bridge-nf-call-ip6tables=1
```

Apply and validate:

```bash
sudo modprobe overlay
sudo modprobe br_netfilter
sudo modprobe vxlan
sudo sysctl --system

sysctl net.ipv4.ip_forward
sysctl net.bridge.bridge-nf-call-iptables
sysctl net.bridge.bridge-nf-call-ip6tables
```

Why these settings exist:

- `overlay` supports efficient container image filesystem layers;
- `br_netfilter` lets Kubernetes networking rules observe bridged traffic;
- `vxlan` supports the default Flannel overlay network;
- IP forwarding allows packets to move between pod, Service, and external networks.

## H3.4 K3s Configuration

Create `/etc/rancher/k3s/config.yaml`:

```yaml
node-name: portfolio-k3s-01
write-kubeconfig-mode: "0600"
secrets-encryption: true
flannel-backend: vxlan
cluster-cidr: 10.42.0.0/16
service-cidr: 10.43.0.0/16
resolv-conf: /etc/rancher/k3s/resolv.conf
node-label:
  - environment=portfolio
  - role=k3s-server
```

Create the dedicated cluster resolver file `/etc/rancher/k3s/resolv.conf`:

```text
nameserver 185.12.64.1
nameserver 185.12.64.2
options timeout:2 attempts:3 rotate
```

Protect the configuration:

```bash
sudo chown root:root /etc/rancher/k3s/config.yaml /etc/rancher/k3s/resolv.conf
sudo chmod 600 /etc/rancher/k3s/config.yaml
sudo chmod 644 /etc/rancher/k3s/resolv.conf
```

The dedicated resolver prevents pods from inheriting too many host nameservers. It removed the persistent CoreDNS `DNSConfigForming` warning.

## H3.5 Pinned K3s Installation

The stable channel resolved to `v1.36.2+k3s1`, and that exact version was pinned instead of allowing an unplanned future version.

Download the official installation script before execution:

```bash
curl -fsSL \
  https://get.k3s.io \
  -o /tmp/install-k3s.sh
```

Install the pinned version:

```bash
sudo env \
  INSTALL_K3S_VERSION="v1.36.2+k3s1" \
  sh /tmp/install-k3s.sh

rm -f /tmp/install-k3s.sh
```

K3s reads `/etc/rancher/k3s/config.yaml` automatically when the service starts.

Validate:

```bash
sudo systemctl is-enabled k3s
sudo systemctl is-active k3s
sudo systemctl status k3s --no-pager
sudo k3s --version
```

## H3.6 What K3s Installed

The bundled system contains:

| Component | Purpose |
| --- | --- |
| Kubernetes API server | Accepts and validates desired cluster state |
| Scheduler | Chooses a node for each pod |
| Controller managers | Reconcile Kubernetes resources toward desired state |
| containerd | Downloads images and runs containers |
| Flannel | Provides pod networking using VXLAN |
| CoreDNS | Resolves Kubernetes Service names |
| Traefik | Routes public HTTP and HTTPS traffic to Services |
| ServiceLB | Connects the VM ports to Traefik's LoadBalancer Service |
| Metrics Server | Supplies resource metrics to `kubectl top` |
| local-path provisioner | Creates persistent volumes on the node's local disk |

The important traffic path is:

```text
Public client
   -> Hetzner firewall TCP 80/443
   -> VM network interface
   -> K3s ServiceLB
   -> Traefik
   -> matching Ingress rule
   -> ClusterIP Service
   -> selected pod
```

## H3.7 Cluster Health Validation

**Server:**

```bash
sudo k3s kubectl get nodes --show-labels
sudo k3s kubectl get pods --all-namespaces -o wide
sudo k3s kubectl get deployments,daemonsets --all-namespaces
sudo k3s kubectl get storageclass
sudo k3s kubectl get ingressclass
sudo k3s kubectl get services --all-namespaces
sudo k3s secrets-encrypt status
sudo k3s kubectl get events --all-namespaces --field-selector type=Warning
```

Validated results:

- node `portfolio-k3s-01` was `Ready`;
- CoreDNS, local-path provisioner, Metrics Server, Traefik, and ServiceLB were healthy;
- `local-path` was the default StorageClass;
- `traefik` was the default IngressClass;
- the Traefik LoadBalancer advertised `142.132.178.45`;
- Kubernetes Secret encryption was enabled with matching server hashes.

Some one-time startup warnings occurred while Flannel and controllers initialized. They were accepted only after all current pods became ready and the warnings did not recur.

## H3.8 Functional Cluster Validation

A temporary `infra-validation` namespace was used to prove behavior rather than relying only on status output.

The validation demonstrated:

1. A pod could start.
2. A ClusterIP Service resolved through CoreDNS.
3. Traefik routed a hostname through Ingress.
4. A PersistentVolumeClaim bound through `local-path`.
5. Data survived deletion and replacement of the application pod.
6. The public route was reachable from WSL.

The persisted value before and after pod replacement matched:

```text
k3s-pvc-created-1784109809
```

The namespace was then deleted:

```bash
sudo k3s kubectl delete namespace infra-validation
```

The deleted hostname returned Traefik `404`, proving that the validation route had been removed.

## H3.9 Local Kubeconfig

The K3s administrator kubeconfig is root-readable on the server. A temporary user-owned copy was created only long enough to transfer it securely.

**Server:**

```bash
sudo install \
  -m 600 \
  -o eoghan \
  -g eoghan \
  /etc/rancher/k3s/k3s.yaml \
  /home/eoghan/k3s.yaml
```

**WSL:**

```bash
mkdir -p "$HOME/.kube"
chmod 700 "$HOME/.kube"

scp \
  portfolio-k3s:~/k3s.yaml \
  "$HOME/.kube/portfolio-k3s.yaml"

chmod 600 "$HOME/.kube/portfolio-k3s.yaml"
```

Remove the temporary server copy:

```bash
ssh portfolio-k3s 'rm -f "$HOME/k3s.yaml"'
```

Configure the local file to use the tunnel endpoint:

```bash
kubectl \
  --kubeconfig "$HOME/.kube/portfolio-k3s.yaml" \
  config set-cluster default \
  --server=https://127.0.0.1:16443

kubectl \
  --kubeconfig "$HOME/.kube/portfolio-k3s.yaml" \
  config rename-context default portfolio-k3s
```

The kubeconfig contains powerful cluster credentials. File mode `600` means only the local owner can read it. It must never be committed.

## H3.10 Private SSH Tunnel

The K3s API listens on server port 6443, but the Hetzner firewall does not expose that port publicly. The SSH tunnel provides temporary private access.

**WSL, dedicated tunnel terminal:**

```bash
ssh \
  -NT \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=60 \
  -o ServerAliveCountMax=3 \
  -L 127.0.0.1:16443:127.0.0.1:6443 \
  portfolio-k3s
```

Leave that terminal open. It is normally quiet because `-N` starts no remote
command and `-T` disables pseudo-terminal allocation. This command does not
open a shell or enter a Pod:

- `kubectl` continues to run in WSL;
- it connects to local port 16443;
- SSH encrypts and forwards that connection;
- the server passes it to its own loopback port 6443;
- K3s authenticates the kubeconfig and handles the request.

**WSL, second terminal:**

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
kubectl config current-context
kubectl get nodes
kubectl get pods --all-namespaces
```

When the session is finished, press `Ctrl+C` in the dedicated tunnel terminal.
Closing that foreground terminal also ends the tunnel. K3s and all workloads
continue running.

For an unusually long session, a background ControlMaster is an optional
advanced workflow. Check for an existing master before starting another one;
a socket file alone does not prove the master is active.

```bash
mkdir -p "$HOME/.ssh/controlmasters"
chmod 700 "$HOME/.ssh/controlmasters"

ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O check \
  portfolio-k3s
```

Only when no active master exists, start it:

```bash
ssh \
  -M \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -fNT \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=60 \
  -o ServerAliveCountMax=3 \
  -L 127.0.0.1:16443:127.0.0.1:6443 \
  portfolio-k3s
```

Check it with the preceding `-O check` command. Stop it explicitly:

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O exit \
  portfolio-k3s
```

A background tunnel can remain active after its original terminal closes. It
normally ends when WSL shuts down, the connection fails, the SSH process
exits, or `-O exit` is used. Both workflows bind only WSL
`127.0.0.1:16443`; neither makes the API public. A later `connection refused`
on that endpoint means the local tunnel is absent, not that Kubernetes
necessarily stopped. The day-to-day and stale-socket procedures are
authoritative in [`operator-guide.md`](operator-guide.md).

## H3.11 External Exposure Validation

After K3s installation, the external scan showed:

```text
22/tcp    open      ssh
80/tcp    open      http
443/tcp   open      https
5432/tcp  filtered  postgresql
6379/tcp  filtered  redis
6443/tcp  filtered  kubernetes-api
8080/tcp  filtered
9090/tcp  filtered
```

This proved that Traefik was public while the Kubernetes API and data services remained private.

## H3.12 Resource Baseline

After K3s stabilized:

- approximately 1.2 GiB memory was used;
- approximately 2.5 GiB remained available;
- the root filesystem used approximately 3.1 GiB;
- `/var/lib/rancher/k3s` used approximately 1.4 GiB;
- no host services were failed.

This showed that the CX23 was sufficient for the bootstrap platform.

## H3 Exit Criteria

- pinned K3s service is enabled and active;
- node is `Ready`;
- bundled system pods are healthy;
- Service DNS works;
- Ingress routing works;
- PVC persistence works across pod replacement;
- Kubernetes Secrets encryption is enabled;
- local kubeconfig is protected;
- API access works through SSH and is not public.

---

# H4 — DNS and TLS Foundation

## H4.1 Objective

Give the cluster a real public domain, automate certificates with cert-manager, validate safely against Let's Encrypt staging, issue a trusted production certificate, and redirect HTTP users to HTTPS.

## H4.2 Domain Purchase and Account Protection

The domain was purchased through Porkbun:

```text
eoghanclancy.eu
```

Registrar protections enabled:

- application-based two-factor authentication using Authy;
- domain lock;
- auto-renewal;
- protected account recovery information.

The `.eu` WHOIS notice stated that the registry receives registration contact information but generally redacts personal information. DNSSEC was deferred until the initial routing and certificate flow was stable.

## H4.3 DNS Record

**Porkbun:** create:

```text
Type: A
Host: test.platform
Value: 142.132.178.45
```

No `AAAA` record was created because IPv6 application routing had not yet been validated. Publishing an untested IPv6 record could cause IPv6-capable clients to choose a broken path.

**WSL:** validate using multiple resolvers:

```bash
dig +short A test.platform.eoghanclancy.eu
dig +short AAAA test.platform.eoghanclancy.eu
dig +short A test.platform.eoghanclancy.eu @1.1.1.1
dig +short A test.platform.eoghanclancy.eu @8.8.8.8
```

The A queries all returned `142.132.178.45`, while the AAAA query returned nothing.

Before creating an Ingress route:

```bash
curl -sS \
  -o /dev/null \
  -w 'HTTP status: %{http_code}\n' \
  http://test.platform.eoghanclancy.eu/
```

Traefik returned `404`. That result was useful: DNS and port 80 reached Traefik, but no application route existed yet.

## H4.4 Install cert-manager

cert-manager watches Kubernetes certificate resources, communicates with certificate authorities, solves ownership challenges, creates TLS Secrets, and renews certificates before expiry.

The main components are:

| Component | Responsibility |
| --- | --- |
| cert-manager controller | Reconciles Issuers, Certificates, Requests, Orders, and Challenges |
| webhook | Validates and defaults cert-manager API resources |
| cainjector | Injects CA bundles into supported resources |

**WSL with the SSH tunnel active:**

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
kubectl get nodes
helm version --short
```

Helm was installed locally in WSL. Helm runs locally, renders the chart, and submits resources to the Kubernetes API through the same SSH tunnel as `kubectl`.

The installation pinned cert-manager `v1.21.0` and verified the official chart signature:

```bash
CERT_MANAGER_VERSION="v1.21.0"
KEYRING="/tmp/cert-manager-keyring.gpg"

curl -fsSL \
  -o "$KEYRING" \
  "https://cert-manager.io/public-keys/cert-manager-keyring-2021-09-20-1020CF3C033D4F35BAE1C19E1226061C665DF13E.gpg"

gpg --show-keys "$KEYRING"

helm install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --version "$CERT_MANAGER_VERSION" \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --verify \
  --keyring "$KEYRING"

rm -f "$KEYRING"
unset CERT_MANAGER_VERSION KEYRING
```

The CRDs extend the Kubernetes API with:

```text
Certificate
CertificateRequest
Issuer
ClusterIssuer
Order
Challenge
```

Validate:

```bash
kubectl wait \
  --namespace cert-manager \
  --for=condition=Available \
  deployment/cert-manager \
  deployment/cert-manager-cainjector \
  deployment/cert-manager-webhook \
  --timeout=180s

helm status cert-manager -n cert-manager
kubectl get pods -n cert-manager -o wide
kubectl get deployments -n cert-manager
kubectl get crds -o name | grep cert-manager.io
kubectl get events -n cert-manager --field-selector type=Warning
```

All three deployments became available with zero restarts and no warnings.

## H4.5 Let's Encrypt Staging Issuer

Staging is used first because repeated test failures should not consume production issuance limits. Staging certificates deliberately fail normal browser trust checks.

File: `kubernetes/bootstrap/cert-manager/clusterissuer-letsencrypt-staging.yaml`

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    email: eoghanclancy@live.com
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-staging-account-key
    solvers:
      - http01:
          ingress:
            ingressClassName: traefik
```

Apply and validate:

```bash
kubectl apply -f \
  kubernetes/bootstrap/cert-manager/clusterissuer-letsencrypt-staging.yaml

kubectl wait \
  --for=condition=Ready \
  clusterissuer/letsencrypt-staging \
  --timeout=120s

kubectl get clusterissuer letsencrypt-staging
kubectl describe clusterissuer letsencrypt-staging
kubectl get secret -n cert-manager letsencrypt-staging-account-key
```

The issuer registered its ACME account and reached `Ready=True`. The referenced Secret contains the generated ACME account private key and must never be printed or committed.

## H4.6 How HTTP-01 Works

For a requested hostname, the flow is:

```text
Certificate resource
      |
      v
CertificateRequest
      |
      v
ACME Order
      |
      v
HTTP-01 Challenge
      |
      v
temporary cert-manager solver pod, Service, and Ingress
      |
      v
Let's Encrypt requests:
http://HOST/.well-known/acme-challenge/TOKEN
      |
      v
DNS -> public IP -> port 80 -> Traefik -> solver
      |
      v
domain ownership validated
      |
      v
certificate and private key stored in a Kubernetes TLS Secret
```

This is why DNS and public TCP port 80 had to work before requesting a certificate.

## H4.7 Staging Certificate Validation

A temporary `tls-validation` namespace, BusyBox HTTP server, Service, Certificate, and Ingress were created for `test.platform.eoghanclancy.eu`.

The staging certificate used:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: test-platform-staging
  namespace: tls-validation
spec:
  secretName: test-platform-staging-tls
  dnsNames:
    - test.platform.eoghanclancy.eu
  issuerRef:
    name: letsencrypt-staging
    kind: ClusterIssuer
    group: cert-manager.io
```

It reached `Ready=True`, the ACME Order became `valid`, and the Secret type was `kubernetes.io/tls` with two data entries. The completed temporary Challenge resources were cleaned up automatically.

Validate the staging certificate while deliberately bypassing public trust:

```bash
curl -kfsS https://test.platform.eoghanclancy.eu/

openssl s_client \
  -connect test.platform.eoghanclancy.eu:443 \
  -servername test.platform.eoghanclancy.eu \
  </dev/null 2>/dev/null |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -dates \
  -ext subjectAltName
```

The issuer was identified as a Let's Encrypt staging intermediate, and the SAN matched the hostname.

## H4.8 Let's Encrypt Production Issuer

File: `kubernetes/bootstrap/cert-manager/clusterissuer-letsencrypt-production.yaml`

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-production
spec:
  acme:
    email: eoghanclancy@live.com
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-production-account-key
    solvers:
      - http01:
          ingress:
            ingressClassName: traefik
```

Apply and validate:

```bash
kubectl apply -f \
  kubernetes/bootstrap/cert-manager/clusterissuer-letsencrypt-production.yaml

kubectl wait \
  --for=condition=Ready \
  clusterissuer/letsencrypt-production \
  --timeout=120s

kubectl get clusterissuer letsencrypt-production
kubectl describe clusterissuer letsencrypt-production
kubectl get secret -n cert-manager letsencrypt-production-account-key
```

If `kubectl` unexpectedly tries `http://localhost:8080`, the current shell is not using the portfolio kubeconfig. Fix it instead of using `--validate=false`:

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
kubectl config current-context
kubectl get nodes
```

## H4.9 Production Certificate

File: `kubernetes/validation/tls/production-certificate.yaml`

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: test-platform-production
  namespace: tls-validation
spec:
  secretName: test-platform-production-tls
  dnsNames:
    - test.platform.eoghanclancy.eu
  issuerRef:
    name: letsencrypt-production
    kind: ClusterIssuer
    group: cert-manager.io
```

Apply and wait:

```bash
kubectl apply -f \
  kubernetes/validation/tls/production-certificate.yaml

kubectl wait \
  --namespace tls-validation \
  --for=condition=Ready \
  certificate/test-platform-production \
  --timeout=180s
```

The production CertificateRequest became ready, its Order became `valid`, and the production Secret was created. The Ingress was switched to `test-platform-production-tls` only after the new certificate was ready, avoiding a certificate gap.

Normal verification then worked without `curl -k`:

```bash
curl -fsS https://test.platform.eoghanclancy.eu/
```

The production certificate had:

```text
subject CN: test.platform.eoghanclancy.eu
issuer: Let's Encrypt YR2
SAN: DNS:test.platform.eoghanclancy.eu
notBefore: 15 July 2026
notAfter: 13 October 2026
renewal time: 13 September 2026
```

cert-manager will attempt renewal before expiry and update the same Kubernetes TLS Secret. Traefik watches the Secret and begins serving the replacement certificate without a manual file installation.

The obsolete staging Certificate and TLS Secret were deleted, while the staging ClusterIssuer was retained for future safe tests:

```bash
kubectl delete certificate \
  -n tls-validation \
  test-platform-staging

kubectl delete secret \
  -n tls-validation \
  test-platform-staging-tls \
  --ignore-not-found
```

## H4.10 HTTP-to-HTTPS Redirect

File: `kubernetes/validation/tls/redirect-https.yaml`

```yaml
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: redirect-https
  namespace: tls-validation
spec:
  redirectScheme:
    scheme: https
    permanent: true
```

The Ingress annotation attaches this namespace-scoped middleware:

```yaml
metadata:
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: tls-validation-redirect-https@kubernetescrd
```

The current Ingress TLS section is:

```yaml
spec:
  ingressClassName: traefik
  tls:
    - hosts:
        - test.platform.eoghanclancy.eu
      secretName: test-platform-production-tls
```

The redirect was validated externally:

```text
HTTP/1.1 308 Permanent Redirect
Location: https://test.platform.eoghanclancy.eu/
```

The cert-manager solver creates its own temporary Ingress for future HTTP-01 challenges, so the application redirect does not prevent challenge handling.

## H4.11 Canonical Validation Resources

The final local directory is:

```text
kubernetes/
├── bootstrap/
│   └── cert-manager/
│       ├── clusterissuer-letsencrypt-staging.yaml
│       └── clusterissuer-letsencrypt-production.yaml
└── validation/
    └── tls/
        ├── kustomization.yaml
        ├── tls-validation.yaml
        ├── production-certificate.yaml
        └── redirect-https.yaml
```

The Kustomize entry point is:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - tls-validation.yaml
  - production-certificate.yaml
  - redirect-https.yaml
```

Validate and reconcile this group with:

```bash
kubectl apply \
  --server-side \
  --dry-run=server \
  -k kubernetes/validation/tls/

kubectl apply -k kubernetes/validation/tls/
```

The public validation page returns:

```text
tls-production-validation-ok
```

This lightweight endpoint remains deployed temporarily as a continuous smoke test for DNS, Traefik, HTTPS, and certificate renewal.

## H4.12 Final Validation

```bash
echo "=== HTTPS response ==="
curl -fsS https://test.platform.eoghanclancy.eu/
echo

echo
echo "=== HTTP redirect ==="
curl -sSI http://test.platform.eoghanclancy.eu/ |
  sed -n '1p;/^[Ll]ocation:/p'

echo
echo "=== Issuers ==="
kubectl get clusterissuer

echo
echo "=== Production certificate ==="
kubectl get certificate -n tls-validation

echo
echo "=== TLS resources ==="
kubectl get pods,service,ingress,middleware -n tls-validation

echo
echo "=== Warnings ==="
kubectl get events -n cert-manager --field-selector type=Warning
kubectl get events -n tls-validation --field-selector type=Warning
```

Validated results:

- HTTPS returned `tls-production-validation-ok`;
- HTTP returned a permanent redirect to HTTPS;
- staging and production ClusterIssuers were ready;
- the production Certificate was ready;
- workload, Service, Ingress, and middleware were healthy;
- no cert-manager or validation warnings existed.

## H4 Exit Criteria

- public DNS resolves to the correct server;
- port 80 reaches Traefik;
- HTTP-01 validation works;
- staging issuance works;
- production issuance works;
- standard clients trust the production certificate;
- HTTP redirects to HTTPS;
- renewal timing is visible and understood;
- certificate and ACME private keys remain in Kubernetes Secrets;
- administrative services remain private.

---

# H5 — GitHub Container Registry and CI Access

## H5.1 Objective

Create a private source repository and container registry workflow, publish a traceable validation image, authenticate K3s to private GHCR, deploy the image by immutable digest, and prove both successful and failed image-pull diagnosis.

The final H5 flow is:

```text
Local Git commit
      |
      v
Private GitHub repository
      |
      | push to main
      v
GitHub Actions workflow
      |
      | temporary GITHUB_TOKEN with packages:write
      v
Private GHCR package
      |
      | dedicated PAT classic with read:packages only
      v
Encrypted Kubernetes dockerconfig Secret
      |
      v
K3s pulls an exact sha256 image digest
      |
      v
Running pod reports the source Git commit
```

## H5.2 Repository Initialization

The existing local project folder was initialized with `main` as its default branch:

```bash
cd ~/projects/cloud-native-service-control-plane
git init -b main
```

Before staging, `.gitignore` excluded credentials, local administrative files, environment files, private key formats, kubeconfigs, build output, and Terraform state. `.terraform.lock.hcl` was intentionally left trackable because dependency lock files should normally be committed for reproducibility.

The repository was scanned for credential-like filenames and content. The scan returned no credential patterns:

```bash
rg -l \
  --hidden \
  -g '!.git/**' \
  -g '!*.md' \
  '(-----BEGIN ([O]PENSSH|RSA|EC|DSA|PRIVATE) PRIVATE KEY-----|[g]ithub_pat_[A-Za-z0-9_]+|[g]h[pousr]_[A-Za-z0-9_]+|[c]lient-key-data:|HCLOUD[_]TOKEN=)' \
  . \
  || echo "No credential patterns detected"
```

The repository uses a GitHub-provided no-reply address for commit attribution so the personal email address is not exposed in Git commit metadata:

```bash
GH_LOGIN="$(gh api user --jq '.login')"
GH_ID="$(gh api user --jq '.id')"

git config user.name "Eoghan Clancy"
git config user.email "${GH_ID}+${GH_LOGIN}@users.noreply.github.com"

unset GH_LOGIN GH_ID
```

The initial commit contained only public documentation and non-secret Kubernetes manifests:

```bash
git add .
git diff --cached --check
git commit -m "Bootstrap Hetzner K3s infrastructure foundation"
```

GitHub CLI created and pushed the repository:

```bash
gh repo create cloud-native-service-control-plane \
  --public \
  --source=. \
  --remote=origin \
  --push \
  --description "Cloud-native portfolio platform using Go, Kubernetes operators, K3s, GitOps, and OpenTelemetry"
```

The repository was subsequently changed to private while implementation is in progress. Current repository identity:

```text
Repository: etclank/cloud-native-service-control-plane
Default branch: main
Visibility: private
Remote: https://github.com/etclank/cloud-native-service-control-plane.git
```

Changing repository visibility does not itself expose or revoke Kubernetes credentials. It changes repository access and influences the default visibility of newly published linked packages.

## H5.3 Immutable GitHub Action Pins

Workflow actions were pinned to full commit SHAs rather than mutable major-version tags:

| Action | Release | Commit SHA |
| --- | --- | --- |
| `actions/checkout` | v7.0.0 | `9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0` |
| `docker/setup-buildx-action` | v4.2.0 | `bb05f3f5519dd87d3ba754cc423b652a5edd6d2c` |
| `docker/login-action` | v4.4.0 | `af1e73f918a031802d376d3c8bbc3fe56130a9b0` |
| `docker/metadata-action` | v6.2.0 | `dc802804100637a589fabce1cb79ff13a1411302` |
| `docker/build-push-action` | v7.3.0 | `53b7df96c91f9c12dcc8a07bcb9ccacbed38856a` |

They were resolved through the authenticated GitHub API:

```bash
for action in \
  "actions/checkout:v7.0.0" \
  "docker/setup-buildx-action:v4.2.0" \
  "docker/login-action:v4.4.0" \
  "docker/metadata-action:v6.2.0" \
  "docker/build-push-action:v7.3.0"
do
    repository="${action%%:*}"
    version="${action##*:}"
    sha="$(gh api "repos/${repository}/commits/${version}" --jq '.sha')"
    printf '%-35s %-8s %s\n' "$repository" "$version" "$sha"
done
```

Pinning a commit protects the workflow from an action tag being moved to different code. Updates must be reviewed and deliberately pinned to a new SHA.

## H5.4 Registry Smoke Image

The H5 image is an infrastructure validation tool, not a product application.

Files:

```text
images/registry-smoke/
├── Dockerfile
└── README.md
```

The Dockerfile:

```dockerfile
FROM busybox:1.37.0

ARG BUILD_DATE=unknown
ARG GIT_SHA=unknown
ARG SOURCE_URL=unknown

LABEL org.opencontainers.image.title="Registry smoke image"
LABEL org.opencontainers.image.description="GHCR and Kubernetes image-pull validation image"
LABEL org.opencontainers.image.source="${SOURCE_URL}"
LABEL org.opencontainers.image.revision="${GIT_SHA}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"

RUN mkdir -p /www \
    && printf \
      '{"service":"registry-smoke","revision":"%s","built_at":"%s"}\n' \
      "${GIT_SHA}" \
      "${BUILD_DATE}" \
      > /www/index.html \
    && chmod 0444 /www/index.html

USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["httpd", "-f", "-p", "8080", "-h", "/www"]
```

Design properties:

- pinned BusyBox version;
- unprivileged UID/GID 65534;
- non-privileged port 8080;
- source revision baked into the response;
- OCI source, revision, creation, title, and description labels;
- no shell or Kubernetes credentials embedded in the image.

Docker was not installed in the local WSL distribution. That did not block H5 because GitHub-hosted runners built and validated the image. Local Docker Desktop WSL integration remains optional.

## H5.5 GitHub Actions GHCR Workflow

File:

```text
.github/workflows/registry-smoke.yml
```

The workflow:

```yaml
name: Registry smoke image

on:
  push:
    branches:
      - main
    paths:
      - images/registry-smoke/**
      - .github/workflows/registry-smoke.yml
  pull_request:
    paths:
      - images/registry-smoke/**
      - .github/workflows/registry-smoke.yml
  workflow_dispatch:

concurrency:
  group: registry-smoke-${{ github.ref }}
  cancel-in-progress: true

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}-registry-smoke

jobs:
  build:
    name: Build and optionally publish
    runs-on: ubuntu-latest

    permissions:
      contents: read
      packages: write

    steps:
      - name: Check out repository
        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@bb05f3f5519dd87d3ba754cc423b652a5edd6d2c # v4.2.0

      - name: Log in to GHCR
        if: github.event_name != 'pull_request'
        uses: docker/login-action@af1e73f918a031802d376d3c8bbc3fe56130a9b0 # v4.4.0
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Generate image metadata
        id: metadata
        uses: docker/metadata-action@dc802804100637a589fabce1cb79ff13a1411302 # v6.2.0
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=sha,format=long,prefix=sha-

      - name: Record build time
        id: build
        shell: bash
        run: |
          echo "created=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" >> "$GITHUB_OUTPUT"

      - name: Build and optionally publish image
        id: image
        uses: docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a # v7.3.0
        with:
          context: ./images/registry-smoke
          file: ./images/registry-smoke/Dockerfile
          platforms: linux/amd64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.metadata.outputs.tags }}
          labels: ${{ steps.metadata.outputs.labels }}
          build-args: |
            BUILD_DATE=${{ steps.build.outputs.created }}
            GIT_SHA=${{ github.sha }}
            SOURCE_URL=${{ github.server_url }}/${{ github.repository }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Publish immutable image identity
        if: github.event_name != 'pull_request'
        shell: bash
        run: |
          echo "Image: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:sha-${{ github.sha }}"
          echo "Digest: ${{ steps.image.outputs.digest }}"
```

Important behavior:

- pushes to `main` build and publish;
- pull requests build but do not authenticate or publish;
- manual dispatches are supported;
- `GITHUB_TOKEN` is temporary and scoped to `contents:read` and `packages:write`;
- no personal token is stored in repository Actions secrets;
- deployment tags contain the full Git commit;
- the workflow reports the immutable registry digest;
- cache data is stored in GitHub Actions cache.

## H5.6 Workflow and Package Evidence

The image workflow completed successfully:

```text
Workflow run: 29479198410
Source commit: 90e0703a4f6f9af60179328e1f0662ab59b28408
Image tag: sha-90e0703a4f6f9af60179328e1f0662ab59b28408
Image digest: sha256:53019aa1ef2810c7d5e9085c31a0518bafd9bb48f1be61433883f0bdac206ca9
Package: cloud-native-service-control-plane-registry-smoke
Package repository: etclank/cloud-native-service-control-plane
Package visibility: private
```

The local GitHub CLI initially returned `403` when reading package metadata because its OAuth token did not include `read:packages`. This did not affect the workflow, which used a separate `GITHUB_TOKEN`.

The local CLI permission was refreshed:

```bash
gh auth refresh \
  --hostname github.com \
  --scopes read:packages
```

The GHCR API then confirmed the package link, visibility, tag, and digest. Additional untagged OCI digests were left intact because they are auxiliary image-index, platform, or provenance-related manifests generated by BuildKit.

## H5.7 Private Pull Credential

Because the repository and GHCR package are private, K3s cannot pull the image anonymously. A dedicated Personal Access Token (classic) was created with:

```text
Name: portfolio-k3s-ghcr-pull
Expiration: 90 days
Scope: read:packages only
```

It did not receive `write:packages`, `delete:packages`, `repo`, `workflow`, or administrative scopes.

The token was entered into a shell variable without writing its literal value into shell history:

```bash
read -rsp "GHCR read-only token: " GHCR_PULL_TOKEN
echo
```

The Kubernetes namespace and registry Secret were created:

```bash
kubectl create namespace registry-validation \
  --dry-run=client \
  -o yaml |
kubectl apply -f -

kubectl create secret docker-registry ghcr-pull \
  --namespace registry-validation \
  --docker-server=ghcr.io \
  --docker-username=etclank \
  --docker-password="$GHCR_PULL_TOKEN" \
  --dry-run=client \
  -o yaml |
kubectl apply -f -

unset GHCR_PULL_TOKEN
```

Validated metadata:

```text
Secret: ghcr-pull
Type: kubernetes.io/dockerconfigjson
Data entries: 1
Namespace: registry-validation
```

The Secret value was never printed or committed. K3s stores it within its encrypted Secrets datastore.

The token must be rotated before its expiry unless the package is intentionally made public and the pull Secret is removed.

## H5.8 Digest-pinned Kubernetes Deployment

Files:

```text
kubernetes/validation/registry/
├── kustomization.yaml
└── registry-smoke.yaml
```

The stable manifest contains:

- the `registry-validation` Namespace;
- a `registry-smoke` ServiceAccount referencing `ghcr-pull`;
- disabled ServiceAccount API-token automounting;
- a one-replica Deployment;
- an image reference pinned to the exact digest;
- non-root and read-only container security controls;
- readiness and liveness probes;
- small CPU and memory requests and limits;
- an internal ClusterIP Service.

The deployed image reference is:

```text
ghcr.io/etclank/cloud-native-service-control-plane-registry-smoke@sha256:53019aa1ef2810c7d5e9085c31a0518bafd9bb48f1be61433883f0bdac206ca9
```

The registry Secret is intentionally absent from Kustomize:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - registry-smoke.yaml
```

Validate and apply:

```bash
kubectl apply \
  --server-side \
  --dry-run=server \
  -k kubernetes/validation/registry/

kubectl apply -k kubernetes/validation/registry/

kubectl rollout status \
  deployment/registry-smoke \
  -n registry-validation \
  --timeout=180s
```

The declared image and runtime `imageID` both matched the same GHCR digest.

## H5.9 Runtime Traceability Validation

An ephemeral public BusyBox client queried the private image through Kubernetes Service DNS:

```bash
kubectl run registry-smoke-client \
  -n registry-validation \
  --image=busybox:1.37.0 \
  --restart=Never \
  --rm \
  -i \
  -- \
  wget -qO- \
  http://registry-smoke.registry-validation.svc.cluster.local/
```

The running private image returned:

```json
{"service":"registry-smoke","revision":"90e0703a4f6f9af60179328e1f0662ab59b28408","built_at":"2026-07-16T07:13:45Z"}
```

This joined the complete evidence chain:

```text
Git commit
  = workflow head SHA
  = immutable GHCR tag
  = image OCI revision
  = runtime HTTP revision

GHCR digest
  = Kubernetes desired image digest
  = container runtime imageID
```

## H5.10 Controlled Failure Diagnosis

A temporary invalid `dockerconfigjson` Secret and pod were created to prove the private registry failure path. They were not added to Git.

The pod used the correct private tag with deliberately invalid credentials and `imagePullPolicy: Always`. Kubernetes events showed:

```text
failed to authorize
403 Forbidden
ErrImagePull
ImagePullBackOff
```

This proves how to distinguish registry authentication failure from application startup failure. The container never started, so application logs would not be the correct diagnostic source. The relevant evidence was in `kubectl describe pod` and Warning events.

The invalid pod and Secret were deleted immediately:

```bash
kubectl delete pod \
  -n registry-validation \
  registry-pull-failure

kubectl delete secret \
  -n registry-validation \
  ghcr-invalid
```

The real digest-pinned Deployment remained fully rolled out with one `1/1 Running` pod and zero restarts. Only `ghcr-pull` remained.

## H5 Security Decisions

- The repository and package are currently private.
- GitHub Actions publishes with the repository-scoped `GITHUB_TOKEN`.
- No PAT is stored in GitHub Actions or Git.
- K3s receives a separate read-only token rather than the developer's GitHub CLI credential.
- The token has an expiry and must be rotated.
- The Kubernetes Secret is namespace-scoped and referenced through a dedicated ServiceAccount.
- The pod does not receive a Kubernetes API token.
- Deployment uses an immutable digest rather than `latest`.
- Runtime identity is traceable to source control.
- Deliberately invalid credentials were removed after testing.

## H5 Exit Criteria

- GitHub repository exists and is connected to the local project;
- CI builds the validation image on GitHub-hosted infrastructure;
- workflow dependencies are pinned to full commit SHAs;
- GHCR publication uses `GITHUB_TOKEN` rather than a stored PAT;
- package is linked to the correct repository;
- image has a full commit-based tag and immutable digest;
- cluster can authenticate and pull the private image;
- registry credential is absent from Git;
- running image revision matches the source commit;
- desired image digest matches the runtime image ID;
- controlled invalid credentials produce diagnosable pull failures;
- stable Deployment remains healthy after failure cleanup.

---

# H7 — Platform Operator, API, Managed Workload, and Production TLS

## H7.1 Objective and Result

H7 turns the infrastructure foundation into a working service control plane. The completed slice provides:

- a Kubebuilder v4.15.0 operator foundation;
- the `platform.eoghanclancy.eu/v1alpha1` `ManagedService` API;
- one approved workload template, `demo-http`;
- an authenticated Go control-plane API;
- immutable operator, API, and workload images;
- restricted Kubernetes RBAC;
- manually synchronized Argo CD Applications;
- production TLS for `api.platform.eoghanclancy.eu`;
- a complete create, observe, drift-correct, and delete lifecycle.

The live H7 validation was completed on 20 July 2026. The platform is a single-node portfolio environment, not a highly available production service.

## H7.2 Why the Platform Is Split into Layers

The H7 components deliberately do not collapse into one manifest or one binary.

| Layer | Responsibility | Why it is separate |
| --- | --- | --- |
| CRD | Defines the Kubernetes `ManagedService` contract | API schema and validation must exist before custom resources can be stored |
| Operator | Reconciles desired service state into child resources | Long-running convergence is different from handling an HTTP request |
| Control-plane API | Provides a constrained authenticated user interface | Public clients should not receive raw Kubernetes credentials or arbitrary manifest access |
| RBAC | Defines what each ServiceAccount may do | Authorization remains independently reviewable and least privilege |
| GitOps | Selects reviewed configuration from Git | Deployment approval and rollback are separate from application behavior |
| Registry | Stores immutable OCI images | Source, build output, and runtime identity remain traceable |
| DNS | Maps the public API name to the ingress address | Naming is external to Kubernetes workload logic |
| TLS | Proves the public identity and encrypts transport | Certificate lifecycle belongs to cert-manager and ACME, not the API process |

This separation creates useful failure boundaries. For example, a healthy API image does not prove that its RBAC is correct, a valid Certificate does not prove that the Ingress backend is ready, and an Argo `Synced` state does not replace application-level validation.

## H7.3 ManagedService Contract

The custom resource is:

```text
apiVersion: platform.eoghanclancy.eu/v1alpha1
kind: ManagedService
```

The current contract intentionally exposes only a small supported surface:

| Field | Rule |
| --- | --- |
| `spec.template` | Required and fixed to `demo-http` |
| `spec.replicas` | Defaults to `1`; allowed range `1`–`3` |
| `spec.message` | Optional; maximum 120 characters |

The operator creates:

- one Deployment using the approved immutable `demo-http` image;
- one ClusterIP Service on the internal Kubernetes network;
- owner references from both children to the `ManagedService`.

The status surface includes:

- `observedGeneration`;
- `readyReplicas`;
- the internal endpoint;
- `Available`, `Progressing`, and `Degraded` conditions.

Every condition records the generation it describes. A client can therefore distinguish current status from a condition that belongs to an older desired-state generation.

## H7.4 Desired State, Actual State, and Status

These three concepts are related but not interchangeable.

**Desired state** is the `ManagedService.spec` submitted to the Kubernetes API. It says what the user wants: template, replica count, and message.

**Actual state** is the current Deployment, Service, Pods, and endpoints observed in the cluster. Actual state can temporarily differ because a rollout is progressing, a Pod is failing, or someone changed a managed child manually.

**Status** is the operator's report about the relationship between desired and actual state. It is not the source of truth for desired configuration.

The status transition for a normal rollout is:

```text
ManagedService accepted
  -> Progressing=True
  -> child Deployment reports ready replicas
  -> Available=True
  -> Progressing=False
  -> Degraded=False
```

Invalid operator template configuration or a failed child reconciliation produces `Degraded=True` with a bounded reason and message.

## H7.5 Request-to-Reconciliation Flow

```text
Authenticated client
  -> HTTPS at api.platform.eoghanclancy.eu
  -> Traefik redirect/rate-limit/TLS handling
  -> control-plane API
  -> create ManagedService in applications
  -> Kubernetes API validates the CRD
  -> operator watch receives the event
  -> operator validates its approved image configuration
  -> CreateOrUpdate owned Deployment
  -> CreateOrUpdate owned ClusterIP Service
  -> Pods become ready
  -> operator updates ManagedService status
  -> API GET returns selected desired state and status
```

The control-plane API never accepts an arbitrary namespace, template, image, Kubernetes metadata object, label set, annotation set, or raw manifest. It always writes to `applications` and always selects `demo-http`.

Public endpoints are:

```text
GET /healthz
GET /readyz
```

Authenticated lifecycle endpoints are:

```text
POST   /api/v1/managed-services
GET    /api/v1/managed-services
GET    /api/v1/managed-services/{name}
DELETE /api/v1/managed-services/{name}
```

The bearer token is loaded from a mounted Secret file. Its value is not part of the Deployment manifest, image, documentation, or command examples.

## H7.6 Owner References, Garbage Collection, and Drift Correction

The operator sets controller owner references on each generated Deployment and Service. Kubernetes garbage collection then removes those children when their owning `ManagedService` is deleted. This avoids a separate API delete sequence that could leave orphaned resources.

Reconciliation is idempotent and uses `CreateOrUpdate`. Re-running it with the same desired state does not create duplicate resources or unnecessary status writes.

Drift correction follows the same path as initial creation:

```text
someone changes an owned Deployment from replicas 1 to replicas 2
  -> Deployment watch triggers reconciliation
  -> operator compares actual state with ManagedService.spec
  -> Deployment is restored to replicas 1
```

This is a central operator property: convergence is continuous, not a one-time templating operation.

## H7.7 Immutable Image Identities

The runtime references are exact SHA-256 digests:

```text
Operator
ghcr.io/etclank/cloud-native-service-control-plane-operator@sha256:377af6a1fb4df40c52d6be37d4e948ca1990fe4c0f840789da68fc3e449ffc75

Demo workload
ghcr.io/etclank/cloud-native-service-control-plane-demo-http@sha256:2d1fc30e0cf75ba9fbe96af176f770524377ee5349acbee9ed94ae13f1143b2f

Control-plane API
ghcr.io/etclank/cloud-native-service-control-plane-api@sha256:22ffdf07c24af219a1fe493095c3293c480da4f3cdc04ccc167e65ae81d631ad
```

The image workflows publish `sha-<full commit SHA>` tags for traceability, but Kubernetes runs the digest-qualified references. A tag is a registry pointer that can theoretically move; a digest identifies the selected image content.

The multi-stage builds use pinned builder and distroless runtime bases, static Go binaries, non-root users, SBOM generation, and maximum provenance in GitHub Actions.

## H7.8 Namespace-Scoped Secrets

Kubernetes Secrets are namespaced. A Pod can only reference a Secret from its own namespace, even if an identically named Secret exists elsewhere.

H7 uses these Secret names:

| Secret | Purpose |
| --- | --- |
| `platform-system/ghcr-pull` | Pull operator and API images |
| `applications/ghcr-pull` | Pull managed `demo-http` images |
| `platform-system/control-plane-api-token` | Mount the API bearer-token file |
| `platform-system/control-plane-api-tls` | cert-manager-generated TLS key pair |

No Secret value is committed to Git. The TLS Secret is generated by cert-manager. Registry and API-token Secrets are created or rotated through controlled operational procedures.

Duplicating `ghcr-pull` across namespaces is not redundant configuration: it is required by the Kubernetes security boundary. Rotation must update each permitted namespace deliberately.

## H7.9 ServiceAccounts and RBAC

The operator and API have separate identities.

The operator owns the permissions needed to reconcile `ManagedService` resources and their Deployment and Service children. The API has a Role only in `applications`, with exactly:

```text
apiGroup: platform.eoghanclancy.eu
resource: managedservices
verbs: create, get, list, delete
```

The API cannot:

- read or write Secrets;
- create Deployments or Services directly;
- access another workload namespace;
- update or patch a `ManagedService`;
- modify status or finalizers;
- use cluster-wide RBAC.

Both workloads run non-root with dropped Linux capabilities, RuntimeDefault seccomp, read-only root filesystems, resource requests and limits, and bounded health probes.

## H7.10 GitOps and Public TLS

The restricted `platform-control-plane` AppProject permits only the private repository, the exact `platform-system` and `applications` destinations, and enumerated resource kinds.

Two manually synchronized Applications deploy the platform:

| Application | Source path | Destination |
| --- | --- | --- |
| `platform-operator` | `config/default` | `platform-system` |
| `control-plane-api` | `kubernetes/platform/control-plane-api` | `platform-system` and explicitly namespaced resources |

Argo CD remains private and is accessed through a local port-forward over the Kubernetes SSH tunnel. Automatic synchronization, pruning, and self-healing are intentionally disabled.

The API Certificate uses the production `letsencrypt-production` ClusterIssuer for exactly `api.platform.eoghanclancy.eu`. Traefik:

- accepts public HTTP and HTTPS through the existing ports 80 and 443;
- permanently redirects HTTP to HTTPS;
- terminates TLS using `control-plane-api-tls`;
- applies an average rate of 5 requests per second with a burst of 10;
- routes only to the internal `control-plane-api` ClusterIP Service.

Argo CD, Kubernetes port 6443, databases, and internal workload Services are not public.

## H7.11 Recreate in Dependency Order

Recreation should proceed from contracts and credentials toward public routing:

1. Restore the hardened host, firewall, K3s, private kubeconfig, and SSH tunnel.
2. Install cert-manager and the production ClusterIssuer.
3. Install private Argo CD and register the read-only private repository.
4. Create `platform-system/ghcr-pull` and `applications/ghcr-pull` without writing values to Git.
5. Synchronize `platform-operator` so the CRD, RBAC, operator Deployment, and manager configuration exist.
6. Confirm the CRD is established and the operator Pod is Ready.
7. Create `platform-system/control-plane-api-token` securely.
8. Synchronize `control-plane-api` so the API, restricted RBAC, Service, Certificate, Middleware, and Ingress exist.
9. Confirm API readiness and Certificate readiness.
10. Submit a `ManagedService` through the authenticated API.
11. Confirm its owned Deployment, Service, Pods, endpoint, and status.
12. Test drift correction and deletion/garbage collection.

Do not synchronize both Applications blindly after a partial restore. Stop at each checkpoint and diagnose the first failing dependency.

## H7.12 Validation Checkpoints and Live Evidence

The closeout validation established:

| Checkpoint | Observed result |
| --- | --- |
| GitOps | Operator and API Applications `Synced` and `Healthy` |
| API extension | `ManagedService` CRD installed |
| Workloads | Operator, API, and demo Pods Ready with zero restarts |
| Public routing | HTTP returned `308`; HTTPS health succeeded |
| Authentication | Unauthenticated API returned `401`; authenticated API succeeded |
| Creation | `portfolio-demo` created through the API |
| Reconciliation | Child Deployment and Service became Ready |
| Workload | Internal response returned the configured message |
| Status | `Available=True`, `readyReplicas=1` |
| Drift | Deployment replicas changed from `1` to `2` and restored to `1` |
| Deletion | `lifecycle-check` returned `204`; owner and children disappeared |
| Retained demo | `portfolio-demo` remained active |
| Events | No warning events |
| Node baseline | Approximately 6% CPU and 66% memory |
| Exposure | 22, 80, 443 open; 6443, 5432, 6379, 8080, 9090 filtered |
| Host health | Zero failed systemd services |
| Certificate | Valid production certificate for `api.platform.eoghanclancy.eu` |

The memory observation is important for H8. A 4 GB single node has limited headroom for Prometheus, Loki, Tempo, Grafana, and an OpenTelemetry Collector. H8 must begin with conservative requests, limits, and retention rather than deploying an unbounded reference stack.

## H7 Exit Criteria

- [x] `ManagedService` CRD installed and validated.
- [x] Operator ready and reconciling owned child resources.
- [x] Control-plane API ready with constrained lifecycle endpoints.
- [x] Public API protected by bearer authentication and production HTTPS.
- [x] Immutable runtime digests configured.
- [x] Least-privilege ServiceAccounts and RBAC verified.
- [x] Manual restricted GitOps delivery verified.
- [x] Status, internal endpoint, owner references, drift correction, and deletion verified.
- [x] No Secret values committed or included in documentation.

H7 is complete. At its closeout, the next phase was H8 observability deployment.

---

# H8.1–H8.3 — Observability Foundation and Collector

## H8.1–H8.2 Design and Instrumentation

H8 began with a resource and retention design for the constrained single-node
environment, followed by shared Go telemetry foundations for the operator,
control-plane API, and demo service. Runtime image pins were advanced through
the reviewed immutable publication process. Workload OTLP endpoints remained
unset, so instrumentation did not imply live export.

## H8.3 Collector Delivery and Network Boundary

The OpenTelemetry Collector is packaged through a locked Helm dependency and
delivered by a restricted, manually synchronized Argo CD Application. It runs
in `observability` with bounded resources, private ClusterIP receivers, and
NetworkPolicies that admit only reviewed client Pod identities from
`platform-system` and `applications` on TCP 4317 and 4318.

Gate 3V-R proved both authorized OTLP/HTTP paths returned HTTP 200. The
unlabelled identity in each client namespace was rejected, and an authorized
identity was rejected on the excluded TCP 13133 health port. On the pinned K3s
kube-router dataplane, denied traffic produces an explicit refusal and curl
exit 7; the paired successful controls distinguish policy enforcement from an
unhealthy target. The Collector remained Ready with zero restarts, cleanup was
complete, and production workload inventories were unchanged.

The canonical sanitized evidence, including immutable revisions, hashes,
resource identities, procedural history, and exact case timestamps, is in
[`h8-collector-deployment-closeout.md`](h8-collector-deployment-closeout.md).
The original Gate 3V remains procedurally failed; Gate 3V-D diagnosed the
dataplane behavior, and Gate 3V-R is the authoritative passing execution.

H8.3 and H8.4 are complete. The restricted Collector and bounded Prometheus
stack are live through manual-sync GitOps; all six accepted scrape jobs are
healthy, PVC-backed recovery passed, and rollback was dry-run without deleting
data. Only the `nop` Collector exporter is live, and no production workload
exports OTLP. Kubelet, cAdvisor, Loki, Tempo, Grafana, and other additional
backends are optional post-H8 enhancements. See
[`h8-observability-closeout.md`](h8-observability-closeout.md).

# Operational Concepts Learned Through H1–H8

## Desired State and Reconciliation

Kubernetes resources describe desired state. For example, a Deployment says one validation pod should exist. Kubernetes controllers continually compare that request with reality and create or replace pods until it is true.

cert-manager uses the same pattern. A Certificate says which DNS name and Secret are desired. cert-manager creates requests, Orders, and Challenges until the certificate is ready, then later repeats the process for renewal.

## Pods, Services, and Ingresses

- A **pod** runs one or more containers and has an internal, replaceable IP.
- A **Service** provides a stable internal address and selects pods by labels.
- An **Ingress** maps an HTTP hostname and path to a Service.
- **Traefik** implements those Ingress rules and terminates TLS.

Clients never need to know the pod IP. When a Deployment replaces a pod, the Service automatically selects the new pod through its labels.

## Secrets

Kubernetes Secrets hold sensitive runtime data such as TLS private keys. K3s encrypts Secret values in its datastore, but RBAC and kubeconfig protection are still essential because authorized administrators can request decrypted values through the API.

## Helm and Kustomize

- **Helm** installs parameterized third-party application packages, such as cert-manager.
- **Kustomize** composes and applies plain Kubernetes manifests without templating. `kubectl` includes Kustomize support through `kubectl apply -k`.

Helm and Kustomize both submit resources to the Kubernetes API; neither runs the deployed application locally in WSL.

## Public Versus Administrative Traffic

Public traffic enters through ports 80 and 443 and is handled by Traefik. Administrative Kubernetes traffic stays private and crosses an authenticated SSH tunnel. This separation reduces attack surface without preventing convenient local administration.

## Single-node Limitations

The environment does not provide:

- node redundancy;
- control-plane high availability;
- multi-zone resilience;
- highly available persistent storage;
- automatic disaster recovery.

The design is appropriate for a portfolio demonstration and learning laboratory. It must not be described as a highly available production platform.

---

# Common Recovery Checks

## SSH works but kubectl uses localhost:8080

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
kubectl config current-context
```

## kubectl reports 127.0.0.1:16443 refused

The kubeconfig is correct but the SSH tunnel is not running. Start the default
foreground tunnel from H3.10 and leave its dedicated terminal open. If the
optional background workflow was deliberately used, run its `-O check`
command instead.

## SSH times out

The local public IP may have changed. Determine the new address and update the Hetzner firewall's TCP 22 source `/32` rule. Do not permanently open SSH to all addresses.

## DNS resolves but the site returns 404

The request probably reached Traefik but no matching Ingress exists. Inspect:

```bash
kubectl get ingress --all-namespaces
kubectl describe ingress -n NAMESPACE INGRESS_NAME
```

## Certificate does not become ready

Inspect from the highest-level resource downward:

```bash
kubectl describe certificate -n NAMESPACE CERTIFICATE
kubectl get certificaterequest,order,challenge -n NAMESPACE
kubectl describe certificaterequest -n NAMESPACE REQUEST
kubectl describe order -n NAMESPACE ORDER
kubectl describe challenge -n NAMESPACE CHALLENGE
kubectl get events -n NAMESPACE --sort-by='.lastTimestamp'
```

Also confirm that public DNS points to the VM and public TCP port 80 remains reachable.

## A pod is not ready

```bash
kubectl get pod -n NAMESPACE POD -o wide
kubectl describe pod -n NAMESPACE POD
kubectl logs -n NAMESPACE POD --all-containers
kubectl logs -n NAMESPACE POD --all-containers --previous
kubectl get events -n NAMESPACE --sort-by='.lastTimestamp'
```

---

# Phase Status

| Phase | Status | Main result |
| --- | --- | --- |
| H1 | Complete | Hetzner VM, firewall, dedicated SSH access, non-root administrator |
| H2 | Complete | Updated and hardened Ubuntu host |
| H3 | Complete | Healthy pinned K3s cluster with private local administration |
| H4 | Complete | Public DNS, cert-manager, trusted TLS, renewal, HTTPS redirect |
| H5 | Complete | Private GHCR publication, read-only cluster authentication, immutable digest deployment |
| H6 | Complete | Private Argo CD bootstrap, restricted GitOps, and tested rollback |
| H7 | Complete | Operator, authenticated API, managed workload, immutable images, TLS, and lifecycle validation |
| H8 | Complete | Restricted Collector and bounded six-job Prometheus deployment validated |

---

# Recording an Optional Improvement

The baseline is complete through H8. If an
[optional improvement](optional-improvements.md) is later implemented, its
record should add:

```text
Phase objective
Current decisions and versions
Architecture or traffic flow
Permanent files created
Commands grouped by WSL/server/Kubernetes/external provider
Why each major step is required
Validation commands and observed evidence
Common failures and diagnosis
Security implications
Resource impact
Exit criteria
Final phase status
```

The document should record the tested path, not merely paste generic installation instructions.
