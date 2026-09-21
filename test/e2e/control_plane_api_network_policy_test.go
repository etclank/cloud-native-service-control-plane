//go:build e2e
// +build e2e

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/etclank/cloud-native-service-control-plane/test/utils"
)

const (
	apiNetworkPolicyTestNamespace = "platform-system"
	prometheusTestNamespace       = "observability"
	traefikProbePod               = "traefik-network-policy-probe"
	prometheusProbePod            = "prometheus-network-policy-probe"
	unrelatedProbePod             = "unrelated-network-policy-probe"
)

var _ = Describe("Control-plane API NetworkPolicy", Ordered, func() {
	BeforeAll(func() {
		for _, name := range []string{
			apiNetworkPolicyTestNamespace,
			prometheusTestNamespace,
			managedServiceNamespace,
		} {
			cmd := exec.Command("kubectl", "create", "namespace", name)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		}

		cmd := exec.Command(
			"kubectl", "label", "--overwrite", "namespace",
			apiNetworkPolicyTestNamespace,
			"pod-security.kubernetes.io/enforce=restricted",
		)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("deploying an API-shaped HTTP and metrics endpoint")
		applyManifest(fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: control-plane-api
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: control-plane-api
  template:
    metadata:
      labels:
        app.kubernetes.io/name: control-plane-api
        app.kubernetes.io/component: api
        app.kubernetes.io/part-of: cloud-native-service-control-plane
    spec:
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: api
          image: %s
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 8080
            - name: metrics
              containerPort: 9090
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
          resources:
            requests:
              cpu: 5m
              memory: 16Mi
            limits:
              cpu: 100m
              memory: 64Mi
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
            readOnlyRootFilesystem: true
---
apiVersion: v1
kind: Service
metadata:
  name: control-plane-api
  namespace: %s
spec:
  selector:
    app.kubernetes.io/name: control-plane-api
  ports:
    - name: http
      port: 80
      targetPort: http
    - name: metrics
      port: 9090
      targetPort: metrics
`, apiNetworkPolicyTestNamespace, demoHTTPImage, apiNetworkPolicyTestNamespace))

		cmd = exec.Command(
			"kubectl", "rollout", "status", "deployment/control-plane-api",
			"--namespace", apiNetworkPolicyTestNamespace, "--timeout=2m",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("applying the production API ingress policies")
		for _, path := range []string{
			"kubernetes/platform/control-plane-api/network-policy.yaml",
			"kubernetes/platform/control-plane-api/network-policy-traefik.yaml",
		} {
			cmd = exec.Command("kubectl", "apply", "-f", path)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		}

		createNetworkProbe(
			traefikProbePod,
			"kube-system",
			"app.kubernetes.io/name=traefik,app.kubernetes.io/instance=traefik-kube-system",
		)
		createNetworkProbe(
			prometheusProbePod,
			prometheusTestNamespace,
			"app.kubernetes.io/name=prometheus,app.kubernetes.io/instance=observability,app.kubernetes.io/component=server",
		)
		createNetworkProbe(
			unrelatedProbePod,
			apiNetworkPolicyTestNamespace,
			"app.kubernetes.io/name=unrelated",
		)
	})

	AfterAll(func() {
		for _, name := range []string{
			traefikProbePod,
			prometheusProbePod,
			unrelatedProbePod,
		} {
			ns := apiNetworkPolicyTestNamespace
			if name == traefikProbePod {
				ns = "kube-system"
			} else if name == prometheusProbePod {
				ns = prometheusTestNamespace
			}
			cmd := exec.Command(
				"kubectl", "delete", "pod", name,
				"--namespace", ns, "--ignore-not-found",
			)
			_, _ = utils.Run(cmd)
		}
		for _, name := range []string{
			apiNetworkPolicyTestNamespace,
			prometheusTestNamespace,
			managedServiceNamespace,
		} {
			cmd := exec.Command(
				"kubectl", "delete", "namespace", name, "--ignore-not-found",
			)
			_, _ = utils.Run(cmd)
		}
	})

	It("allows only the intended HTTP and metrics identities", func() {
		apiHTTPURL := "http://control-plane-api.platform-system.svc.cluster.local/healthz"
		apiMetricsURL := "http://control-plane-api.platform-system.svc.cluster.local:9090/metrics"

		By("allowing the Traefik identity to receive HTTP 200 from /healthz")
		Eventually(func() error {
			return probeURL(traefikProbePod, "kube-system", apiHTTPURL)
		}, time.Minute, time.Second).Should(Succeed())

		By("allowing the Prometheus identity to reach TCP 9090")
		Eventually(func() error {
			return probeURL(prometheusProbePod, prometheusTestNamespace, apiMetricsURL)
		}, time.Minute, time.Second).Should(Succeed())

		By("proving the unrelated identity is denied on TCP 8080 and TCP 9090")
		for _, url := range []string{apiHTTPURL, apiMetricsURL} {
			Eventually(func() bool {
				return probeURL(
					unrelatedProbePod,
					apiNetworkPolicyTestNamespace,
					url,
				) != nil
			}, 30*time.Second, time.Second).Should(BeTrue())
			Consistently(func() bool {
				return probeURL(
					unrelatedProbePod,
					apiNetworkPolicyTestNamespace,
					url,
				) != nil
			}, 5*time.Second, time.Second).Should(BeTrue())
		}
	})
})

func applyManifest(manifest string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func createNetworkProbe(name string, namespace string, labels string) {
	overrides := fmt.Sprintf(`{
		"spec": {
			"automountServiceAccountToken": false,
			"securityContext": {
				"runAsNonRoot": true,
				"runAsUser": 1000,
				"runAsGroup": 1000,
				"seccompProfile": {"type": "RuntimeDefault"}
			},
			"containers": [{
				"name": "%s",
				"image": "%s",
				"imagePullPolicy": "IfNotPresent",
				"command": ["/bin/sh", "-c", "sleep 600"],
				"resources": {
					"requests": {"cpu": "1m", "memory": "8Mi"},
					"limits": {"cpu": "50m", "memory": "32Mi"}
				},
				"securityContext": {
					"allowPrivilegeEscalation": false,
					"capabilities": {"drop": ["ALL"]},
					"readOnlyRootFilesystem": true
				}
			}]
		}
	}`, name, metricsProbeImage)
	cmd := exec.Command(
		"kubectl", "run", name,
		"--namespace", namespace,
		"--restart=Never",
		"--labels="+labels,
		"--image="+metricsProbeImage,
		"--image-pull-policy=IfNotPresent",
		"--overrides", overrides,
	)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cmd = exec.Command(
		"kubectl", "wait", "pod/"+name,
		"--namespace", namespace,
		"--for=condition=Ready", "--timeout=2m",
	)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func probeURL(name string, namespace string, url string) error {
	cmd := exec.Command(
		"kubectl", "exec", name,
		"--namespace", namespace,
		"--", "curl", "--fail", "--silent", "--show-error",
		"--connect-timeout", "2", "--max-time", "5", url,
	)
	_, err := utils.Run(cmd)

	return err
}
