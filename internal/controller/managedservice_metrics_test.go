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

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

type getErrorClient struct {
	client.Client
	err error
}

func (failingClient *getErrorClient) Get(
	context.Context,
	client.ObjectKey,
	client.Object,
	...client.GetOption,
) error {
	return failingClient.err
}

const (
	reconciliationsMetricName = "managedservice_reconciliations_total"
	durationMetricName        = "managedservice_reconcile_duration_seconds"
	transitionsMetricName     = "managedservice_condition_transitions_total"
)

func TestManagedServiceMetricsRecordExactlyOneOutcome(t *testing.T) {
	tests := []struct {
		name      string
		result    ctrl.Result
		err       error
		outcome   string
		forbidden string
	}{
		{name: "success", outcome: reconciliationOutcomeSuccess},
		{
			name:      "error",
			err:       errors.New("private reconciliation failure"),
			outcome:   reconciliationOutcomeError,
			forbidden: "private reconciliation failure",
		},
		{
			name:    "explicit requeue",
			result:  ctrl.Result{Requeue: true},
			outcome: reconciliationOutcomeRequeue,
		},
		{
			name:    "delayed requeue",
			result:  ctrl.Result{RequeueAfter: time.Minute},
			outcome: reconciliationOutcomeRequeue,
		},
		{
			name: "error takes precedence over requeue",
			result: ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Minute,
			},
			err:       errors.New("private precedence failure"),
			outcome:   reconciliationOutcomeError,
			forbidden: "private precedence failure",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics, err := NewManagedServiceMetrics(registry)
			if err != nil {
				t.Fatalf("NewManagedServiceMetrics() error = %v", err)
			}

			metrics.observeReconciliation(test.result, test.err, 250*time.Millisecond)

			families, err := registry.Gather()
			if err != nil {
				t.Fatalf("gather metrics: %v", err)
			}
			seenCounter := false
			seenHistogram := false
			var exposition strings.Builder
			for _, family := range families {
				exposition.WriteString(family.String())
				switch family.GetName() {
				case reconciliationsMetricName:
					seenCounter = true
					if family.GetHelp() != "Total number of completed ManagedService reconciliation attempts." {
						t.Errorf("reconciliation help = %q", family.GetHelp())
					}
					metricSeries := family.GetMetric()
					if len(metricSeries) != 1 {
						t.Fatalf("reconciliation metric series = %d, want 1", len(metricSeries))
					}
					labels := metricSeries[0].GetLabel()
					if len(labels) != 1 || labels[0].GetName() != outcomeLabel ||
						labels[0].GetValue() != test.outcome {
						t.Errorf("reconciliation labels = %#v", labels)
					}
					if metricSeries[0].GetCounter().GetValue() != 1 {
						t.Errorf("reconciliation count = %v, want 1", metricSeries[0].GetCounter().GetValue())
					}
				case durationMetricName:
					seenHistogram = true
					if family.GetHelp() != "Duration of completed ManagedService reconciliation attempts in seconds." {
						t.Errorf("duration help = %q", family.GetHelp())
					}
					metricSeries := family.GetMetric()
					if len(metricSeries) != 1 {
						t.Fatalf("duration metric series = %d, want 1", len(metricSeries))
					}
					labels := metricSeries[0].GetLabel()
					if len(labels) != 1 || labels[0].GetName() != outcomeLabel ||
						labels[0].GetValue() != test.outcome {
						t.Errorf("duration labels = %#v", labels)
					}
					histogram := metricSeries[0].GetHistogram()
					if histogram.GetSampleCount() != 1 {
						t.Errorf("duration observations = %d, want 1", histogram.GetSampleCount())
					}
					if len(histogram.GetBucket()) != len(reconciliationDurationBuckets) {
						t.Errorf("duration buckets = %d, want %d", len(histogram.GetBucket()), len(reconciliationDurationBuckets))
					}
				}
			}
			if !seenCounter || !seenHistogram {
				t.Errorf("metric families present: counter=%t histogram=%t", seenCounter, seenHistogram)
			}
			for _, forbidden := range []string{
				test.forbidden,
				"private-managed-service",
				"private-namespace",
				"private-uid",
				"private-configured-message",
			} {
				if forbidden != "" && strings.Contains(exposition.String(), forbidden) {
					t.Errorf("metric exposition contains forbidden value %q", forbidden)
				}
			}
		})
	}
}

func TestReconcileRecordsExactlyOneFinalOutcome(t *testing.T) {
	testScheme := runtime.NewScheme()
	if err := platformv1alpha1.AddToScheme(testScheme); err != nil {
		t.Fatalf("add ManagedService API to scheme: %v", err)
	}
	privateErr := errors.New("private Kubernetes client failure")
	tests := []struct {
		name      string
		client    func() client.Client
		wantError bool
		outcome   string
	}{
		{
			name: "successful not found completion",
			client: func() client.Client {
				return fake.NewClientBuilder().WithScheme(testScheme).Build()
			},
			outcome: reconciliationOutcomeSuccess,
		},
		{
			name: "returned client error",
			client: func() client.Client {
				return &getErrorClient{
					Client: fake.NewClientBuilder().WithScheme(testScheme).Build(),
					err:    privateErr,
				}
			},
			wantError: true,
			outcome:   reconciliationOutcomeError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics, err := NewManagedServiceMetrics(registry)
			if err != nil {
				t.Fatalf("NewManagedServiceMetrics() error = %v", err)
			}
			reconciler := &ManagedServiceReconciler{
				Client:  test.client(),
				Scheme:  testScheme,
				Metrics: metrics,
			}

			_, reconcileErr := reconciler.Reconcile(context.Background(), ctrl.Request{})
			if test.wantError && !errors.Is(reconcileErr, privateErr) {
				t.Fatalf("Reconcile() error = %v, want %v", reconcileErr, privateErr)
			}
			if !test.wantError && reconcileErr != nil {
				t.Fatalf("Reconcile() error = %v", reconcileErr)
			}
			if count := reconciliationCount(t, registry, test.outcome); count != 1 {
				t.Errorf("%s reconciliation count = %v, want 1", test.outcome, count)
			}
			if count := totalReconciliationCount(t, registry); count != 1 {
				t.Errorf("total reconciliation count = %v, want 1", count)
			}
			if exposition := gatheredMetrics(t, registry); strings.Contains(exposition, privateErr.Error()) {
				t.Error("metric exposition contains the reconciliation error")
			}
		})
	}
}

func TestManagedServiceMetricsReuseIdenticalRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()
	first, err := NewManagedServiceMetrics(registry)
	if err != nil {
		t.Fatalf("first NewManagedServiceMetrics() error = %v", err)
	}
	second, err := NewManagedServiceMetrics(registry)
	if err != nil {
		t.Fatalf("second NewManagedServiceMetrics() error = %v", err)
	}

	first.observeReconciliation(ctrl.Result{}, nil, time.Millisecond)
	second.observeReconciliation(ctrl.Result{}, nil, time.Millisecond)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != reconciliationsMetricName {
			continue
		}
		metrics := family.GetMetric()
		if len(metrics) != 1 || metrics[0].GetCounter().GetValue() != 2 {
			t.Errorf("reconciliation counter after reuse = %#v", metrics)
		}

		return
	}
	t.Fatalf("metric %q not gathered", reconciliationsMetricName)
}

func TestManagedServiceConditionMetricsRecordOnlyStatusTransitions(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewManagedServiceMetrics(registry)
	if err != nil {
		t.Fatalf("NewManagedServiceMetrics() error = %v", err)
	}
	initial := []metav1.Condition{
		{
			Type:   platformv1alpha1.ManagedServiceConditionAvailable,
			Status: metav1.ConditionFalse,
		},
		{
			Type:   platformv1alpha1.ManagedServiceConditionProgressing,
			Status: metav1.ConditionTrue,
		},
		{
			Type:   platformv1alpha1.ManagedServiceConditionDegraded,
			Status: metav1.ConditionFalse,
		},
	}
	metrics.observeConditionTransitions(nil, initial)
	metrics.observeConditionTransitions(initial, initial)

	changed := append([]metav1.Condition(nil), initial...)
	changed[0].Status = metav1.ConditionTrue
	metrics.observeConditionTransitions(initial, changed)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != transitionsMetricName {
			continue
		}
		if family.GetHelp() != "Total number of persisted ManagedService condition status transitions." {
			t.Errorf("condition transition help = %q", family.GetHelp())
		}
		if len(family.GetMetric()) != 4 {
			t.Fatalf("condition transition series = %d, want 4", len(family.GetMetric()))
		}
		allowedConditions := map[string]bool{
			platformv1alpha1.ManagedServiceConditionAvailable:   true,
			platformv1alpha1.ManagedServiceConditionProgressing: true,
			platformv1alpha1.ManagedServiceConditionDegraded:    true,
		}
		allowedStatuses := map[string]bool{"true": true, "false": true, "unknown": true}
		for _, metric := range family.GetMetric() {
			if metric.GetCounter().GetValue() != 1 {
				t.Errorf("condition transition count = %v, want 1", metric.GetCounter().GetValue())
			}
			if len(metric.GetLabel()) != 2 {
				t.Errorf("condition transition labels = %d, want 2", len(metric.GetLabel()))
			}
			for _, label := range metric.GetLabel() {
				switch label.GetName() {
				case "condition":
					if !allowedConditions[label.GetValue()] {
						t.Errorf("condition label = %q", label.GetValue())
					}
				case "status":
					if !allowedStatuses[label.GetValue()] {
						t.Errorf("status label = %q", label.GetValue())
					}
				default:
					t.Errorf("unexpected condition metric label %q", label.GetName())
				}
			}
		}

		return
	}
	t.Fatalf("metric %q not gathered", transitionsMetricName)
}

func reconciliationCount(
	t *testing.T,
	registry *prometheus.Registry,
	outcome string,
) float64 {
	t.Helper()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != reconciliationsMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == outcomeLabel && label.GetValue() == outcome {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}

	return 0
}

func totalReconciliationCount(t *testing.T, registry *prometheus.Registry) float64 {
	t.Helper()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	var total float64
	for _, family := range families {
		if family.GetName() != reconciliationsMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}

	return total
}

func gatheredMetrics(t *testing.T, registry *prometheus.Registry) string {
	t.Helper()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	var exposition strings.Builder
	for _, family := range families {
		exposition.WriteString(family.String())
	}

	return exposition.String()
}
