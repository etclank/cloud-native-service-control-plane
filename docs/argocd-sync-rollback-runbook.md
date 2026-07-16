# Argo CD Synchronization and Rollback Runbook

This runbook operates the Argo CD installation for the Cloud-Native Service Control Plane portfolio cluster.

## Scope

Current GitOps resources:

| Item | Value |
| --- | --- |
| Argo CD namespace | `argocd` |
| AppProject | `portfolio-validation` |
| Application | `registry-smoke` |
| Git repository | `git@github.com:etclank/cloud-native-service-control-plane.git` |
| Tracked branch | `main` |
| Source path | `kubernetes/validation/registry` |
| Destination | `registry-validation` |
| Sync policy | Manual |

The Argo CD server is private. Do not create a public Ingress, LoadBalancer, NodePort, or firewall exception for administrative convenience.

## 1. Start an Administrative Session

Set the local tools:

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"
export PATH="$HOME/.local/bin:$PATH"
```

Check the Kubernetes API SSH tunnel:

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O check \
  portfolio-k3s
```

If it is not running:

```bash
mkdir -p "$HOME/.ssh/controlmasters"
chmod 700 "$HOME/.ssh/controlmasters"

ssh \
  -M \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -fNT \
  -o ExitOnForwardFailure=yes \
  -L 127.0.0.1:16443:127.0.0.1:6443 \
  portfolio-k3s
```

Validate Kubernetes:

```bash
kubectl get nodes
```

In a second WSL terminal, keep this foreground process running:

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"

kubectl port-forward \
  --namespace argocd \
  --address 127.0.0.1 \
  service/argocd-server \
  18080:443
```

Validate the existing Argo CD session in the original terminal:

```bash
argocd account get-user-info
```

If the session has expired, log in interactively without placing a password in shell history:

```bash
argocd login 127.0.0.1:18080 \
  --username admin \
  --insecure
```

## 2. Routine Health Check

```bash
kubectl get deployments,statefulsets,pods \
  --namespace argocd

argocd repo get \
  git@github.com:etclank/cloud-native-service-control-plane.git

argocd app get registry-smoke

kubectl get deployment,pods \
  --namespace registry-validation
```

Expected state:

- all Argo CD workloads ready;
- repository connection `Successful`;
- Application `Synced` and `Healthy`;
- registry smoke Deployment ready;
- immutable image digest present.

## 3. Review a Pending Git Change

Argo CD uses manual synchronization. A Git change should become `OutOfSync` without changing the cluster.

Refresh comparison state:

```bash
argocd app get registry-smoke \
  --hard-refresh
```

Review the difference:

```bash
argocd app diff registry-smoke
```

Confirm the live workload before approving deployment:

```bash
kubectl get deployment,pods \
  --namespace registry-validation
```

Do not synchronize until the Git commit, rendered manifests, destination, image identity, and expected rollout impact have been reviewed.

## 4. Synchronize an Exact Revision

From a clean local clone:

```bash
cd ~/projects/cloud-native-service-control-plane

git status --short --branch
git pull --ff-only origin main

SYNC_REVISION="$(git rev-parse HEAD)"
printf 'Synchronizing revision: %s\n' "$SYNC_REVISION"
```

Apply that exact revision:

```bash
argocd app sync registry-smoke \
  --revision "$SYNC_REVISION"

argocd app wait registry-smoke \
  --sync \
  --health \
  --timeout 180
```

Validate the rollout:

```bash
kubectl rollout status \
  deployment/registry-smoke \
  --namespace registry-validation \
  --timeout=120s

argocd app get registry-smoke
argocd app history registry-smoke

kubectl get deployment,pods \
  --namespace registry-validation
```

## 5. Diagnose a Failed Synchronization

Inspect from GitOps state down to Kubernetes state:

```bash
argocd app get registry-smoke
argocd app diff registry-smoke
argocd app history registry-smoke
argocd app logs registry-smoke
```

```bash
kubectl describe deployment registry-smoke \
  --namespace registry-validation

kubectl get pods \
  --namespace registry-validation \
  --output wide

kubectl get events \
  --namespace registry-validation \
  --sort-by='.lastTimestamp'
```

For a failing pod:

```bash
kubectl describe pod POD_NAME \
  --namespace registry-validation

kubectl logs POD_NAME \
  --namespace registry-validation \
  --all-containers

kubectl logs POD_NAME \
  --namespace registry-validation \
  --all-containers \
  --previous
```

Common causes include an invalid manifest, an AppProject permission denial, an unavailable image, invalid `ghcr-pull` credentials, failed readiness checks, or insufficient node resources.

## 6. Immediate Operational Rollback

Use this when a synchronized revision is unhealthy and the prior Argo CD history entry is known to be good.

Inspect history first:

```bash
argocd app history registry-smoke
```

Record the current and target history IDs in the incident notes. Then roll back:

```bash
argocd app rollback registry-smoke HISTORY_ID
```

Validate recovery:

```bash
argocd app wait registry-smoke \
  --health \
  --timeout 180

kubectl rollout status \
  deployment/registry-smoke \
  --namespace registry-validation \
  --timeout=120s

kubectl get deployment,pods \
  --namespace registry-validation
```

After an operational rollback, `OutOfSync` is expected if `main` still contains the bad desired state. Do not treat that status as a reason to synchronize the bad revision again.

## 7. Reconcile the Git Source of Truth

Identify the bad Git commit and revert it transparently:

```bash
cd ~/projects/cloud-native-service-control-plane

git status --short --branch
git show --stat --oneline BAD_COMMIT
git revert --no-edit BAD_COMMIT
git push origin main
```

Capture and synchronize the revert commit:

```bash
GIT_ROLLBACK_REVISION="$(git rev-parse HEAD)"

argocd app sync registry-smoke \
  --revision "$GIT_ROLLBACK_REVISION"

argocd app wait registry-smoke \
  --sync \
  --health \
  --timeout 180
```

Final validation:

```bash
git status --short --branch
argocd app get registry-smoke
argocd app history registry-smoke

kubectl get deployment,pods \
  --namespace registry-validation
```

The final state must be both healthy in Kubernetes and synchronized with the corrected Git revision.

## 8. Repository Credential Checks

Check connectivity without displaying credential data:

```bash
argocd repo get \
  git@github.com:etclank/cloud-native-service-control-plane.git

kubectl get secrets \
  --namespace argocd \
  --selector argocd.argoproj.io/secret-type=repository
```

Never use `kubectl get secret ... -o yaml` in logs, documentation, issues, or chat.

If the GitHub deploy key must be rotated:

1. generate a new dedicated key outside the repository;
2. add its public key to the GitHub repository as read-only;
3. update repository credentials through `argocd repo add`;
4. confirm connection status is `Successful`;
5. remove the old GitHub deploy key;
6. securely remove superseded local key material.

Do not reuse the GHCR pull token as an Argo CD repository credential.

## 9. Stop Private Administrative Access

Stop the foreground Argo CD port-forward with `Ctrl+C` in its terminal.

When Kubernetes administration is also finished, stop the API SSH tunnel:

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O exit \
  portfolio-k3s
```

Stopping either tunnel does not stop Argo CD or deployed applications. It only removes local administrative access.

## 10. Tested H6 Recovery Evidence

The H6 exercise produced this sequence:

| History ID | Revision | Result |
| --- | --- | --- |
| 0 | `385dcf9` | Initial one-replica synchronization |
| 1 | `4014081` | Explicit two-replica synchronization |
| 2 | `385dcf9` | Immediate rollback to the healthy baseline |
| 3 | `d908af3` | Git revert synchronized as authoritative state |

The final workload was one ready replica using digest `sha256:53019aa1ef2810c7d5e9085c31a0518bafd9bb48f1be61433883f0bdac206ca9`.