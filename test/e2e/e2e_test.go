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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
	"github.com/etclank/cloud-native-service-control-plane/test/utils"
)

// namespace where the project is deployed in
const namespace = "platform-system"

// serviceAccountName created for the project
const serviceAccountName = "platform-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "platform-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "platform-operator-metrics-binding"

const managedServiceNamespace = "applications"

const metricsProbeImage = "docker.io/curlimages/curl@sha256:" +
	"1ab04d023ece37e6ec991bf3306ad04e0ef0084e94a5c6b6563cfcb9563169db"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command(
			"make", "deploy-test-e2e",
			"E2E_DEMO_HTTP_IMAGE="+demoHTTPImage,
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command(
			"kubectl", "delete", "pod", "curl-metrics",
			"-n", namespace, "--ignore-not-found",
		)
		_, _ = utils.Run(cmd)

		By("cleaning up the metrics reader binding")
		cmd = exec.Command(
			"kubectl", "delete", "clusterrolebinding",
			metricsRoleBindingName, "--ignore-not-found",
		)
		_, _ = utils.Run(cmd)

		By("removing the managed workload namespace")
		cmd = exec.Command(
			"kubectl", "delete", "namespace",
			managedServiceNamespace, "--ignore-not-found",
		)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy-test-e2e")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			for _, diagnostic := range [][]string{
				{"get", "service", metricsServiceName, "-n", namespace, "-o", "yaml"},
				{
					"get", "endpointslice", "-n", namespace,
					"-l", "kubernetes.io/service-name=" + metricsServiceName,
					"-o", "yaml",
				},
				{"get", "networkpolicy", "-n", namespace, "-o", "yaml"},
				{"get", "namespace", namespace, "--show-labels"},
				{"describe", "pod", "curl-metrics", "-n", namespace},
			} {
				cmd = exec.Command("kubectl", diagnostic...)
				output, err := utils.Run(cmd)
				if err == nil {
					_, _ = fmt.Fprintf(
						GinkgoWriter,
						"kubectl %s:\n%s",
						strings.Join(diagnostic, " "),
						output,
					)
				}
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				By("getting the name of the controller-manager pod")
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				By("validating the pod's status")
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=platform-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			verifyMetricsService := func(g Gomega) {
				service := getMetricsService(g)
				g.Expect(service.Spec.Ports).To(HaveLen(1))
				g.Expect(service.Spec.Ports[0].Port).To(Equal(int32(8443)))
				g.Expect(service.Spec.Ports[0].TargetPort.String()).To(Equal("8443"))
			}
			Eventually(verifyMetricsService).Should(Succeed())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the metrics service has a ready endpoint on port 8443")
			verifyMetricsEndpoint := func(g Gomega) {
				endpointSlices := getMetricsEndpointSlices(g)
				foundReadyEndpoint := false
				for _, endpointSlice := range endpointSlices.Items {
					hasMetricsPort := false
					for _, port := range endpointSlice.Ports {
						if port.Port != nil && *port.Port == 8443 {
							hasMetricsPort = true
						}
					}
					if !hasMetricsPort {
						continue
					}
					for _, endpoint := range endpointSlice.Endpoints {
						if endpoint.Conditions.Ready != nil &&
							*endpoint.Conditions.Ready &&
							len(endpoint.Addresses) > 0 {
							foundReadyEndpoint = true
						}
					}
				}
				g.Expect(foundReadyEndpoint).To(BeTrue())
			}
			Eventually(verifyMetricsEndpoint).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--labels=app.kubernetes.io/name=e2e-metrics-client,app.kubernetes.io/component=metrics-probe",
				"--image="+metricsProbeImage,
				"--image-pull-policy=IfNotPresent",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "%s",
							"command": ["/bin/sh", "-c"],
							"args": [
								"token_path=/var/run/secrets/kubernetes.io/serviceaccount/token; url=https://%s.%s.svc.cluster.local:8443/metrics; attempt=1; while [ \"$attempt\" -le 12 ]; do if output=$(curl --fail --silent --show-error --insecure --connect-timeout 2 --max-time 5 --header \"Authorization: Bearer $(cat \"$token_path\")\" \"$url\" 2>&1); then printf 'HTTP 200\n%%s\n' \"$output\"; exit 0; fi; printf 'metrics attempt %%s failed: %%s\n' \"$attempt\" \"$output\" >&2; attempt=$((attempt + 1)); [ \"$attempt\" -le 12 ] && sleep 2; done; exit 1"
							],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, metricsProbeImage, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete")
			Expect(waitForMetricsProbe(2 * time.Minute)).To(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput, err := getMetricsOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
			Expect(metricsOutput).To(ContainSubstring("HTTP 200"))
			Expect(metricsOutput).To(ContainSubstring(
				"# HELP controller_runtime_reconcile_total",
			))

		})

		It("should reconcile a ManagedService to a ready Deployment and Service", func() {
			By("creating the restricted managed workload namespace")
			cmd := exec.Command("kubectl", "create", "namespace", managedServiceNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command(
				"kubectl", "label", "--overwrite", "namespace",
				managedServiceNamespace,
				"pod-security.kubernetes.io/enforce=restricted",
			)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a ManagedService")
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(fmt.Sprintf(`
apiVersion: platform.eoghanclancy.eu/v1alpha1
kind: ManagedService
metadata:
  name: e2e-demo
  namespace: %s
spec:
  template: demo-http
  replicas: 1
  message: E2E managed service
`, managedServiceNamespace))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the generated Deployment to be created")
			cmd = exec.Command(
				"kubectl", "wait", "deployment/e2e-demo",
				"--namespace", managedServiceNamespace,
				"--for=create", "--timeout=1m",
			)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the generated Deployment to become Available")
			cmd = exec.Command(
				"kubectl", "wait", "deployment/e2e-demo",
				"--namespace", managedServiceNamespace,
				"--for=condition=Available", "--timeout=2m",
			)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the reconciled ManagedService status")
			Eventually(func(g Gomega) {
				managedService := getManagedService(g, "e2e-demo")
				g.Expect(managedService.Status.ObservedGeneration).To(Equal(
					managedService.Generation,
				))
				g.Expect(managedService.Status.ReadyReplicas).To(Equal(int32(1)))
				available := meta.FindStatusCondition(
					managedService.Status.Conditions,
					platformv1alpha1.ManagedServiceConditionAvailable,
				)
				g.Expect(available).NotTo(BeNil())
				g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(available.ObservedGeneration).To(Equal(
					managedService.Generation,
				))
			}).Should(Succeed())

			By("verifying the generated Deployment and Service")
			deployment := getManagedDeployment("e2e-demo")
			Expect(deployment.Status.ReadyReplicas).To(Equal(int32(1)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(
				demoHTTPImage,
			))
			Expect(
				deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy,
			).To(Equal(corev1.PullIfNotPresent))
			Expect(deployment.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())

			service := getManagedServiceResource("e2e-demo")
			Expect(service.Spec.Ports).To(HaveLen(2))
			servicePorts := make(map[string]corev1.ServicePort)
			for _, port := range service.Spec.Ports {
				servicePorts[port.Name] = port
			}
			Expect(servicePorts["http"].Port).To(Equal(int32(80)))
			Expect(servicePorts["http"].TargetPort.StrVal).To(Equal("http"))
			Expect(servicePorts["metrics"].Port).To(Equal(int32(9090)))
			Expect(servicePorts["metrics"].TargetPort.StrVal).To(Equal("metrics"))

			Eventually(func(g Gomega) {
				g.Expect(hasReadyServiceEndpoint(g, "e2e-demo", 8080)).To(BeTrue())
			}).Should(Succeed())

			expectCleanWorkloadState(namespace)
			expectCleanWorkloadState(managedServiceNamespace)
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput, err := getMetricsOutput()
		// Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})
})

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

func getMetricsService(g Gomega) *corev1.Service {
	cmd := exec.Command(
		"kubectl", "get", "service", metricsServiceName,
		"-n", namespace, "-o", "json",
	)
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

	service := &corev1.Service{}
	g.Expect(json.Unmarshal([]byte(output), service)).To(Succeed())

	return service
}

func getMetricsEndpointSlices(g Gomega) *discoveryv1.EndpointSliceList {
	cmd := exec.Command(
		"kubectl", "get", "endpointslice",
		"-n", namespace,
		"-l", "kubernetes.io/service-name="+metricsServiceName,
		"-o", "json",
	)
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred())

	endpointSlices := &discoveryv1.EndpointSliceList{}
	g.Expect(json.Unmarshal([]byte(output), endpointSlices)).To(Succeed())

	return endpointSlices
}

func waitForMetricsProbe(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command(
			"kubectl", "get", "pod", "curl-metrics",
			"-n", namespace, "-o", "json",
		)
		output, err := utils.Run(cmd)
		if err != nil {
			return fmt.Errorf("get metrics probe Pod: %w", err)
		}

		pod := &corev1.Pod{}
		if err := json.Unmarshal([]byte(output), pod); err != nil {
			return fmt.Errorf("decode metrics probe Pod: %w", err)
		}

		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			return nil
		case corev1.PodFailed:
			logs, logsErr := getMetricsOutput()
			if logsErr != nil {
				logs = fmt.Sprintf("<unavailable: %v>", logsErr)
			}
			return fmt.Errorf("metrics probe Pod failed: %s", logs)
		case corev1.PodPending, corev1.PodRunning:
			time.Sleep(time.Second)
		default:
			return fmt.Errorf(
				"metrics probe Pod entered unexpected phase %q",
				pod.Status.Phase,
			)
		}
	}

	logs, err := getMetricsOutput()
	if err != nil {
		logs = fmt.Sprintf("<unavailable: %v>", err)
	}

	return fmt.Errorf("metrics probe Pod did not finish within %s: %s", timeout, logs)
}

func getManagedService(g Gomega, name string) *platformv1alpha1.ManagedService {
	cmd := exec.Command(
		"kubectl", "get", "managedservice", name,
		"-n", managedServiceNamespace, "-o", "json",
	)
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred())

	managedService := &platformv1alpha1.ManagedService{}
	g.Expect(json.Unmarshal([]byte(output), managedService)).To(Succeed())

	return managedService
}

func getManagedDeployment(name string) *appsv1.Deployment {
	cmd := exec.Command(
		"kubectl", "get", "deployment", name,
		"-n", managedServiceNamespace, "-o", "json",
	)
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	deployment := &appsv1.Deployment{}
	Expect(json.Unmarshal([]byte(output), deployment)).To(Succeed())

	return deployment
}

func getManagedServiceResource(name string) *corev1.Service {
	cmd := exec.Command(
		"kubectl", "get", "service", name,
		"-n", managedServiceNamespace, "-o", "json",
	)
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	service := &corev1.Service{}
	Expect(json.Unmarshal([]byte(output), service)).To(Succeed())

	return service
}

func hasReadyServiceEndpoint(g Gomega, serviceName string, port int32) bool {
	cmd := exec.Command(
		"kubectl", "get", "endpointslice",
		"-n", managedServiceNamespace,
		"-l", "kubernetes.io/service-name="+serviceName,
		"-o", "json",
	)
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred())

	endpointSlices := &discoveryv1.EndpointSliceList{}
	g.Expect(json.Unmarshal([]byte(output), endpointSlices)).To(Succeed())
	for _, endpointSlice := range endpointSlices.Items {
		portMatches := false
		for _, endpointPort := range endpointSlice.Ports {
			if endpointPort.Port != nil && *endpointPort.Port == port {
				portMatches = true
			}
		}
		if !portMatches {
			continue
		}
		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.Conditions.Ready != nil &&
				*endpoint.Conditions.Ready &&
				len(endpoint.Addresses) > 0 {
				return true
			}
		}
	}

	return false
}

func expectCleanWorkloadState(targetNamespace string) {
	cmd := exec.Command(
		"kubectl", "get", "pods",
		"-n", targetNamespace, "-o", "json",
	)
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	pods := &corev1.PodList{}
	Expect(json.Unmarshal([]byte(output), pods)).To(Succeed())
	for _, pod := range pods.Items {
		Expect(pod.Status.Phase).NotTo(Equal(corev1.PodPending), pod.Name)
		Expect(pod.Status.Phase).NotTo(Equal(corev1.PodFailed), pod.Name)
		for _, container := range pod.Status.ContainerStatuses {
			Expect(container.RestartCount).To(BeZero(), pod.Name)
		}
	}

	cmd = exec.Command(
		"kubectl", "get", "events",
		"-n", targetNamespace,
		"--field-selector=type=Warning", "-o", "json",
	)
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	events := &corev1.EventList{}
	Expect(json.Unmarshal([]byte(output), events)).To(Succeed())
	Expect(events.Items).To(BeEmpty(), "unexpected Warning events in %s", targetNamespace)
}
