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

package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	prometheusTargetMatrixPath = "docs/h8-prometheus-target-matrix.json"
	acceptedTargetStatus       = "accepted"
	deferredTargetStatus       = "deferred"
	optionalPostH8Owner        = "Optional post-H8 enhancement"
	prometheusSelfTarget       = "prometheus-self"
	platformOperatorTarget     = "platform-operator"
	controlPlaneAPITarget      = "control-plane-api"
	managedDemoTarget          = "managed-demo"
	smartEnergyTarget          = "smartenergy-api"
	kubeletTarget              = "kubelet"
	cadvisorTarget             = "cadvisor"
	httpScheme                 = "http"
	httpsScheme                = "https"
	metricsPath                = "/metrics"
	defaultScrapeInterval      = "30s"
	defaultScrapeTimeout       = "10s"
	nodeScrapeInterval         = "60s"
	nodeScrapeTimeout          = "15s"
	applicationInstanceLabel   = "app.kubernetes.io/instance"
	applicationComponentLabel  = "app.kubernetes.io/component"
	namespaceField             = "namespace"
	pathField                  = "path"
)

type prometheusTargetMatrix struct {
	Version             string   `json:"version"`
	AcceptedH8TargetIDs []string `json:"acceptedH8TargetIDs"`
	PostH8OptionalIDs   []string `json:"postH8OptionalTargetIDs"`
	H84ScopeDecision    struct {
		AuthenticationAuthorization string `json:"authenticationAuthorization"`
		MetricAllowlists            string `json:"metricAllowlists"`
		KubeletServingCertificate   string `json:"kubeletServingCertificate"`
		NetworkPolicyEnforcement    string `json:"networkPolicyEnforcement"`
		KubeletStatus               string `json:"kubeletStatus"`
		CadvisorStatus              string `json:"cadvisorStatus"`
		ImplementationStatus        string `json:"implementationStatus"`
		H84CKR3Status               string `json:"h84CKR3Status"`
		H84CKIStatus                string `json:"h84CKIStatus"`
		H84DStatus                  string `json:"h84DStatus"`
		H8Status                    string `json:"h8Status"`
		EvidenceReference           string `json:"evidenceReference"`
		ScopeRationale              string `json:"scopeRationale"`
	} `json:"h84ScopeDecision"`
	Global struct {
		ScrapeInterval          string            `json:"scrapeInterval"`
		EvaluationInterval      string            `json:"evaluationInterval"`
		ServiceAccountTokenFile string            `json:"serviceAccountTokenFile"`
		PrometheusPodSelector   map[string]string `json:"prometheusPodSelector"`
	} `json:"global"`
	CardinalityPolicy struct {
		LabelmapAllowed                     bool     `json:"labelmapAllowed"`
		CopiedKubernetesLabelsOrAnnotations []string `json:"copiedKubernetesLabelsOrAnnotations"`
		ForbiddenTargetLabels               []string `json:"forbiddenTargetLabels"`
		DiscoveryMetadataDisposition        string   `json:"discoveryMetadataDisposition"`
		MetricFiltering                     string   `json:"metricFiltering"`
	} `json:"cardinalityPolicy"`
	Jobs                        []prometheusTargetDesign `json:"jobs"`
	SharedDiscoveryRBACDecision struct {
		H84CRequiredRules          []string `json:"h8.4cRequiredRules"`
		OperatorAuthorizationRule  string   `json:"operatorAuthorizationRule"`
		RemoveUntilKubeletFollowUp []string `json:"removeUntilKubeletFollowUp"`
		Rationale                  string   `json:"rationale"`
	} `json:"sharedDiscoveryRBACDecision"`
	FixedNetworkBoundaries struct {
		KubernetesAPIEgress   string   `json:"kubernetesAPIEgress"`
		ForbiddenDestinations []string `json:"forbiddenDestinations"`
	} `json:"fixedNetworkBoundaries"`
}

type prometheusTargetDesign struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Purpose        string `json:"purpose"`
	OwnerBatch     string `json:"ownerBatch"`
	DeferredReason string `json:"deferredReason"`
	Namespace      string `json:"namespace"`
	OwnerObject    string `json:"ownerObject"`
	Service        string `json:"service"`
	Discovery      struct {
		Role            string   `json:"role"`
		Namespaces      []string `json:"namespaces"`
		FieldSelectors  []string `json:"fieldSelectors"`
		Address         string   `json:"address"`
		Selectors       []string `json:"selectors"`
		RelabelSequence []string `json:"relabelSequence"`
	} `json:"discovery"`
	Scheme          string                         `json:"scheme"`
	Port            int                            `json:"port"`
	Path            string                         `json:"path"`
	Interval        string                         `json:"interval"`
	Timeout         string                         `json:"timeout"`
	Authentication  string                         `json:"authentication"`
	TokenFile       string                         `json:"tokenFile"`
	TLSVerification string                         `json:"tlsVerification"`
	TargetLabels    []string                       `json:"targetLabels"`
	RBAC            []string                       `json:"rbac"`
	NetworkPolicy   []string                       `json:"networkPolicy"`
	MetricPolicy    string                         `json:"metricPolicy"`
	ProofEvidence   *prometheusTargetProofEvidence `json:"proofEvidence,omitempty"`
	MetricAllowlist []prometheusMetricAllowlist    `json:"metricAllowlist,omitempty"`
	Lifecycle       string                         `json:"lifecycle"`
}

type prometheusTargetProofEvidence struct {
	Authentication   string   `json:"authentication"`
	Authorization    string   `json:"authorization"`
	TLS              string   `json:"tls"`
	NetworkPolicy    string   `json:"networkPolicy"`
	SchemaInspection string   `json:"schemaInspection"`
	Unresolved       []string `json:"unresolved"`
}

type prometheusMetricAllowlist struct {
	Family         string   `json:"family"`
	Purpose        string   `json:"purpose"`
	ObservedLabels []string `json:"observedLabels"`
	RetainedLabels []string `json:"retainedLabels"`
	RemovedLabels  []string `json:"removedLabels"`
	Cardinality    string   `json:"cardinality"`
}

func TestPrometheusTargetMatrixIsClosedAndImplementationReady(t *testing.T) {
	matrix, _ := readPrometheusTargetMatrix(t)

	if matrix.Version != "stage6b-seven-targets" {
		t.Errorf("target matrix version = %q", matrix.Version)
	}
	if matrix.Global.ScrapeInterval != defaultScrapeInterval ||
		matrix.Global.EvaluationInterval != defaultScrapeInterval {
		t.Errorf("global timing = %#v", matrix.Global)
	}
	wantPrometheusSelector := map[string]string{
		applicationNameLabel:      prometheusChartName,
		applicationInstanceLabel:  observabilityNamespace,
		applicationComponentLabel: serverValue,
	}
	if !reflect.DeepEqual(
		matrix.Global.PrometheusPodSelector,
		wantPrometheusSelector,
	) {
		t.Errorf(
			"Prometheus Pod selector = %#v, want %#v",
			matrix.Global.PrometheusPodSelector,
			wantPrometheusSelector,
		)
	}

	wantStatuses := map[string]string{
		prometheusSelfTarget:      acceptedTargetStatus,
		kubeStateMetricsChartName: acceptedTargetStatus,
		platformOperatorTarget:    acceptedTargetStatus,
		collectorChartName:        acceptedTargetStatus,
		controlPlaneAPITarget:     acceptedTargetStatus,
		managedDemoTarget:         acceptedTargetStatus,
		smartEnergyTarget:         acceptedTargetStatus,
		kubeletTarget:             deferredTargetStatus,
		cadvisorTarget:            deferredTargetStatus,
	}
	wantAcceptedIDs := []string{
		prometheusSelfTarget,
		kubeStateMetricsChartName,
		platformOperatorTarget,
		collectorChartName,
		controlPlaneAPITarget,
		managedDemoTarget,
		smartEnergyTarget,
	}
	if !reflect.DeepEqual(matrix.AcceptedH8TargetIDs, wantAcceptedIDs) {
		t.Errorf(
			"accepted H8 target IDs = %#v, want %#v",
			matrix.AcceptedH8TargetIDs,
			wantAcceptedIDs,
		)
	}
	if !reflect.DeepEqual(
		matrix.PostH8OptionalIDs,
		[]string{kubeletTarget, cadvisorTarget},
	) {
		t.Errorf("post-H8 optional target IDs = %#v", matrix.PostH8OptionalIDs)
	}
	gotStatuses := make(map[string]string, len(matrix.Jobs))
	for index := range matrix.Jobs {
		job := &matrix.Jobs[index]
		if _, duplicate := gotStatuses[job.ID]; duplicate {
			t.Errorf("duplicate target job %q", job.ID)
		}
		gotStatuses[job.ID] = job.Status
		assertCompletePrometheusTargetDesign(t, job)
	}
	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Errorf("target statuses = %#v, want %#v", gotStatuses, wantStatuses)
	}
}

func TestH84ScopeDecisionDefersOptionalNodeJobsAndUnblocksGitOps(t *testing.T) {
	matrix, raw := readPrometheusTargetMatrix(t)
	decision := matrix.H84ScopeDecision

	if decision.AuthenticationAuthorization !=
		"proven and accepted as a future design input" ||
		decision.MetricAllowlists !=
			"proven and accepted as bounded future design inputs" ||
		decision.KubeletServingCertificate != "still unproven" ||
		decision.NetworkPolicyEnforcement != "still unproven" ||
		decision.KubeletStatus != "deferred optional post-H8 enhancement" ||
		decision.CadvisorStatus != "deferred optional post-H8 enhancement" ||
		decision.ImplementationStatus != "not implemented" ||
		decision.H84CKR3Status != "cancelled" ||
		decision.H84CKIStatus != "cancelled" ||
		decision.H84DStatus != "executable" ||
		!strings.Contains(decision.H8Status, "live validation") {
		t.Errorf("H8.4 scope decision = %#v", decision)
	}
	if !strings.Contains(decision.ScopeRationale, "personal single-node") ||
		!strings.Contains(decision.ScopeRationale, "deliberate scope decision") ||
		!strings.Contains(decision.ScopeRationale, "not proof") {
		t.Errorf("H8.4 scope rationale = %q", decision.ScopeRationale)
	}
	evidencePath := filepath.Join("..", "..", decision.EvidenceReference)
	if _, err := os.Stat(evidencePath); err != nil {
		t.Errorf("H8.4 scope evidence reference %q: %v", evidencePath, err)
	}

	for _, jobID := range []string{kubeletTarget, cadvisorTarget} {
		job := findPrometheusTarget(t, matrix.Jobs, jobID)
		if job.Status != deferredTargetStatus ||
			job.OwnerBatch != optionalPostH8Owner ||
			!strings.Contains(
				strings.ToLower(job.DeferredReason),
				"not an h8 completion dependency",
			) {
			t.Errorf("job %q H8.4 scope decision = %#v", jobID, job)
		}
		if strings.Contains(job.TLSVerification, "accepted") ||
			strings.Contains(job.TLSVerification, "insecure_skip_verify: true") {
			t.Errorf("job %q prematurely accepts TLS: %q", jobID, job.TLSVerification)
		}
		if job.ProofEvidence == nil ||
			!strings.Contains(job.ProofEvidence.TLS, "authenticated") ||
			!strings.Contains(job.ProofEvidence.TLS, "sudo -n") ||
			!strings.Contains(job.ProofEvidence.NetworkPolicy, "sudo -n") {
			t.Errorf("job %q preserved proof = %#v", jobID, job.ProofEvidence)
		}
	}

	for _, forbiddenField := range []string{
		`"certificateSubject"`,
		`"certificateIssuer"`,
		`"certificateSANs"`,
		`"certificateFingerprint"`,
		`"trustAnchorPath"`,
	} {
		if strings.Contains(string(raw), forbiddenField) {
			t.Errorf("matrix contains unobserved certificate field %s", forbiddenField)
		}
	}
}

func TestH84CKNodeTargetsRemainEvidenceBackedAndDeferred(t *testing.T) {
	matrix, _ := readPrometheusTargetMatrix(t)

	wantFamilies := map[string][]string{
		kubeletTarget: {
			"kubelet_pleg_relist_duration_seconds",
			"kubelet_running_containers",
			"kubelet_running_pods",
			"kubelet_runtime_operations_errors_total",
			"kubelet_runtime_operations_total",
		},
		cadvisorTarget: {
			"container_cpu_usage_seconds_total",
			"container_memory_working_set_bytes",
		},
	}
	for jobID, expectedFamilies := range wantFamilies {
		job := findPrometheusTarget(t, matrix.Jobs, jobID)
		if job.Status != deferredTargetStatus ||
			job.OwnerBatch != optionalPostH8Owner ||
			!strings.Contains(
				strings.ToLower(job.DeferredReason),
				"not an h8 completion dependency",
			) {
			t.Errorf("job %q deferral = %#v", jobID, job)
		}
		if job.ProofEvidence == nil ||
			len(job.ProofEvidence.Unresolved) == 0 ||
			!strings.Contains(job.ProofEvidence.Authorization, "nodes/metrics get") ||
			!strings.Contains(job.ProofEvidence.SchemaInspection, "no values") {
			t.Errorf("job %q proof evidence = %#v", jobID, job.ProofEvidence)
		}
		joinedUnresolved := strings.Join(job.ProofEvidence.Unresolved, "\n")
		if !strings.Contains(joinedUnresolved, "certificate") ||
			(!strings.Contains(strings.ToLower(joinedUnresolved), "networkpolicy") &&
				!strings.Contains(strings.ToLower(joinedUnresolved), "kube-router")) {
			t.Errorf("job %q unresolved proof = %#v", jobID, job.ProofEvidence.Unresolved)
		}

		gotFamilies := make([]string, 0, len(job.MetricAllowlist))
		for _, metric := range job.MetricAllowlist {
			gotFamilies = append(gotFamilies, metric.Family)
			if metric.Purpose == "" || metric.Cardinality == "" ||
				len(metric.RetainedLabels) == 0 {
				t.Errorf("job %q metric policy is incomplete: %#v", jobID, metric)
			}
			for _, forbidden := range []string{
				"id",
				"image",
				"image_id",
				"container_id",
				"name",
				"pod_uid",
			} {
				if slices.Contains(metric.RetainedLabels, forbidden) {
					t.Errorf(
						"job %q metric %q retains unsafe label %q",
						jobID,
						metric.Family,
						forbidden,
					)
				}
			}
		}
		slices.Sort(gotFamilies)
		slices.Sort(expectedFamilies)
		if !reflect.DeepEqual(gotFamilies, expectedFamilies) {
			t.Errorf(
				"job %q candidate metric families = %#v, want %#v",
				jobID,
				gotFamilies,
				expectedFamilies,
			)
		}
	}

	cadvisor := findPrometheusTarget(t, matrix.Jobs, cadvisorTarget)
	for _, metric := range cadvisor.MetricAllowlist {
		if strings.Contains(metric.Family, "network") {
			t.Errorf("cAdvisor prematurely accepts network family %q", metric.Family)
		}
	}
}

func TestPrometheusTargetMatrixTimingEndpointsAndCredentials(t *testing.T) {
	matrix, raw := readPrometheusTargetMatrix(t)
	type endpointContract struct {
		scheme   string
		port     int
		path     string
		interval string
		timeout  string
	}
	wantEndpoints := map[string]endpointContract{
		prometheusSelfTarget:      {httpScheme, 9090, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		kubeStateMetricsChartName: {httpScheme, 8080, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		platformOperatorTarget:    {httpsScheme, 8443, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		collectorChartName:        {httpScheme, 8888, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		controlPlaneAPITarget:     {httpScheme, 9090, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		managedDemoTarget:         {httpScheme, 9090, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		smartEnergyTarget:         {httpScheme, 9090, metricsPath, defaultScrapeInterval, defaultScrapeTimeout},
		kubeletTarget:             {httpsScheme, 10250, metricsPath, nodeScrapeInterval, nodeScrapeTimeout},
		cadvisorTarget:            {httpsScheme, 10250, "/metrics/cadvisor", nodeScrapeInterval, nodeScrapeTimeout},
	}

	for index := range matrix.Jobs {
		job := &matrix.Jobs[index]
		got := endpointContract{
			job.Scheme,
			job.Port,
			job.Path,
			job.Interval,
			job.Timeout,
		}
		if got != wantEndpoints[job.ID] {
			t.Errorf("job %q endpoint = %#v, want %#v", job.ID, got, wantEndpoints[job.ID])
		}
		interval, intervalErr := time.ParseDuration(job.Interval)
		timeout, timeoutErr := time.ParseDuration(job.Timeout)
		if intervalErr != nil || timeoutErr != nil || timeout >= interval {
			t.Errorf(
				"job %q interval/timeout = %q/%q, errors %v/%v",
				job.ID,
				job.Interval,
				job.Timeout,
				intervalErr,
				timeoutErr,
			)
		}
		if strings.Contains(job.Authentication, "bearer") &&
			job.TokenFile != matrix.Global.ServiceAccountTokenFile {
			t.Errorf("job %q token file = %q", job.ID, job.TokenFile)
		}
	}

	for _, forbidden := range []string{
		"-----BEGIN",
		"Authorization:",
		"\"token\":",
		"ghp_",
		"github_pat_",
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("target matrix contains credential marker %q", forbidden)
		}
	}
}

func TestPrometheusTargetMatrixCardinalityRBACAndPolicyBoundaries(t *testing.T) {
	matrix, _ := readPrometheusTargetMatrix(t)

	if matrix.CardinalityPolicy.LabelmapAllowed {
		t.Error("target matrix permits labelmap")
	}
	if len(matrix.CardinalityPolicy.CopiedKubernetesLabelsOrAnnotations) != 0 {
		t.Errorf(
			"copied Kubernetes metadata = %#v",
			matrix.CardinalityPolicy.CopiedKubernetesLabelsOrAnnotations,
		)
	}
	for _, required := range []string{
		"pod_uid",
		"container_id",
		"image_id",
		"request_id",
		"trace_id",
	} {
		if !slices.Contains(matrix.CardinalityPolicy.ForbiddenTargetLabels, required) {
			t.Errorf("forbidden target labels omit %q", required)
		}
	}

	wantDiscoveryRules := []string{
		"core/services get,list,watch",
		"core/pods get,list,watch",
		"discovery.k8s.io/endpointslices get,list,watch",
	}
	if !reflect.DeepEqual(
		matrix.SharedDiscoveryRBACDecision.H84CRequiredRules,
		wantDiscoveryRules,
	) {
		t.Errorf(
			"H8.4C discovery RBAC = %#v, want %#v",
			matrix.SharedDiscoveryRBACDecision.H84CRequiredRules,
			wantDiscoveryRules,
		)
	}
	if matrix.FixedNetworkBoundaries.KubernetesAPIEgress !=
		"142.132.178.45/32 TCP 6443" {
		t.Errorf(
			"Kubernetes API boundary = %q",
			matrix.FixedNetworkBoundaries.KubernetesAPIEgress,
		)
	}

	for index := range matrix.Jobs {
		job := &matrix.Jobs[index]
		if job.Status != acceptedTargetStatus {
			continue
		}
		joinedPolicy := strings.Join(job.NetworkPolicy, "\n")
		for _, forbidden := range []string{
			"0.0.0.0/0",
			"10.43.0.0/16",
			"10.42.0.0/16",
			"10.43.0.1/32 TCP 443",
			"port range",
			"internet egress",
		} {
			if strings.Contains(strings.ToLower(joinedPolicy), strings.ToLower(forbidden)) {
				t.Errorf("job %q policy contains forbidden boundary %q", job.ID, forbidden)
			}
		}
		if job.Discovery.Role == "endpointslice" {
			for _, rule := range wantDiscoveryRules {
				if !slices.Contains(job.RBAC, rule) {
					t.Errorf("job %q RBAC omits %q", job.ID, rule)
				}
			}
			if !relabelSequenceFailsClosed(job.Discovery.RelabelSequence) {
				t.Errorf("job %q does not fail closed: %#v", job.ID, job.Discovery.RelabelSequence)
			}
		}
	}
}

func assertCompletePrometheusTargetDesign(
	t *testing.T,
	job *prometheusTargetDesign,
) {
	t.Helper()

	requiredStrings := map[string]string{
		"id":                job.ID,
		"status":            job.Status,
		"purpose":           job.Purpose,
		"ownerBatch":        job.OwnerBatch,
		namespaceField:      job.Namespace,
		"ownerObject":       job.OwnerObject,
		serviceField:        job.Service,
		"discovery.role":    job.Discovery.Role,
		"discovery.address": job.Discovery.Address,
		"scheme":            job.Scheme,
		pathField:           job.Path,
		"interval":          job.Interval,
		"timeout":           job.Timeout,
		"authentication":    job.Authentication,
		"tlsVerification":   job.TLSVerification,
		"metricPolicy":      job.MetricPolicy,
		"lifecycle":         job.Lifecycle,
	}
	for field, value := range requiredStrings {
		if strings.TrimSpace(value) == "" {
			t.Errorf("job %q has empty %s", job.ID, field)
		}
	}
	if job.Port < 1 ||
		len(job.Discovery.FieldSelectors) == 0 ||
		len(job.Discovery.Selectors) == 0 ||
		len(job.Discovery.RelabelSequence) == 0 ||
		len(job.TargetLabels) == 0 ||
		len(job.NetworkPolicy) == 0 {
		t.Errorf("job %q has an incomplete target contract: %#v", job.ID, job)
	}
	if job.Status == deferredTargetStatus {
		if job.DeferredReason == "" ||
			job.OwnerBatch != optionalPostH8Owner {
			t.Errorf("job %q lacks an explicit deferral owner/reason", job.ID)
		}
	} else if job.DeferredReason != "" {
		t.Errorf("accepted job %q has deferred reason %q", job.ID, job.DeferredReason)
	}
}

func findPrometheusTarget(
	t *testing.T,
	jobs []prometheusTargetDesign,
	id string,
) *prometheusTargetDesign {
	t.Helper()

	for index := range jobs {
		if jobs[index].ID == id {
			return &jobs[index]
		}
	}
	t.Fatalf("Prometheus target %q not found", id)

	return nil
}

func relabelSequenceFailsClosed(sequence []string) bool {
	joined := strings.Join(sequence, "\n")
	return strings.Contains(joined, "keep namespace") &&
		strings.Contains(joined, "keep Service") &&
		strings.Contains(joined, "keep EndpointSlice port") &&
		strings.Contains(joined, "keep endpoint readiness") &&
		!strings.Contains(joined, "labelmap")
}

func readPrometheusTargetMatrix(
	t *testing.T,
) (prometheusTargetMatrix, []byte) {
	t.Helper()

	path := filepath.Join("..", "..", prometheusTargetMatrixPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Prometheus target matrix: %v", err)
	}
	matrix := prometheusTargetMatrix{}
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatalf("decode Prometheus target matrix: %v", err)
	}

	return matrix, raw
}
