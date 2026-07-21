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
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	platformv1alpha1 "github.com/etclank/cloud-native-service-control-plane/api/v1alpha1"
)

const (
	reconciliationOutcomeSuccess = "success"
	reconciliationOutcomeError   = "error"
	reconciliationOutcomeRequeue = "requeue"
	outcomeLabel                 = "outcome"
)

var (
	// These buckets cover fast cached reconciliations through slower Kubernetes
	// API operations without creating labels from resource or error data.
	reconciliationDurationBuckets = []float64{
		0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
	}
	managedServiceConditionTypes = []string{
		platformv1alpha1.ManagedServiceConditionAvailable,
		platformv1alpha1.ManagedServiceConditionProgressing,
		platformv1alpha1.ManagedServiceConditionDegraded,
	}
)

// ManagedServiceMetrics records bounded reconciliation and condition outcomes.
type ManagedServiceMetrics struct {
	reconciliations      *prometheus.CounterVec
	reconcileDuration    *prometheus.HistogramVec
	conditionTransitions *prometheus.CounterVec
}

// NewManagedServiceMetrics registers operator metrics with registerer. When
// identical collectors are already registered, they are reused safely.
func NewManagedServiceMetrics(
	registerer prometheus.Registerer,
) (*ManagedServiceMetrics, error) {
	if registerer == nil {
		return nil, fmt.Errorf("prometheus registerer is required")
	}

	reconciliations, err := registerCounterVec(
		registerer,
		prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "managedservice_reconciliations_total",
			Help: "Total number of completed ManagedService reconciliation attempts.",
		}, []string{outcomeLabel}),
	)
	if err != nil {
		return nil, err
	}
	reconcileDuration, err := registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "managedservice_reconcile_duration_seconds",
			Help:    "Duration of completed ManagedService reconciliation attempts in seconds.",
			Buckets: reconciliationDurationBuckets,
		}, []string{outcomeLabel}),
	)
	if err != nil {
		return nil, err
	}
	conditionTransitions, err := registerCounterVec(
		registerer,
		prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "managedservice_condition_transitions_total",
			Help: "Total number of persisted ManagedService condition status transitions.",
		}, []string{"condition", "status"}),
	)
	if err != nil {
		return nil, err
	}

	return &ManagedServiceMetrics{
		reconciliations:      reconciliations,
		reconcileDuration:    reconcileDuration,
		conditionTransitions: conditionTransitions,
	}, nil
}

func (metrics *ManagedServiceMetrics) observeReconciliation(
	result ctrl.Result,
	reconcileErr error,
	duration time.Duration,
) {
	if metrics == nil {
		return
	}

	requeue := result.RequeueAfter > 0
	// Requeue is deprecated for new callers but remains part of ctrl.Result and
	// the required outcome contract for controllers that still return it.
	//nolint:staticcheck
	requeue = requeue || result.Requeue

	outcome := reconciliationOutcomeSuccess
	if reconcileErr != nil {
		outcome = reconciliationOutcomeError
	} else if requeue {
		outcome = reconciliationOutcomeRequeue
	}
	metrics.reconciliations.WithLabelValues(outcome).Inc()
	metrics.reconcileDuration.WithLabelValues(outcome).Observe(duration.Seconds())
}

func (metrics *ManagedServiceMetrics) observeConditionTransitions(
	before []metav1.Condition,
	after []metav1.Condition,
) {
	if metrics == nil {
		return
	}

	for _, conditionType := range managedServiceConditionTypes {
		beforeCondition := apiMeta.FindStatusCondition(before, conditionType)
		afterCondition := apiMeta.FindStatusCondition(after, conditionType)
		if afterCondition == nil ||
			(beforeCondition != nil && beforeCondition.Status == afterCondition.Status) {
			continue
		}

		metrics.conditionTransitions.WithLabelValues(
			conditionType,
			strings.ToLower(string(afterCondition.Status)),
		).Inc()
	}
}

func registerCounterVec(
	registerer prometheus.Registerer,
	collector *prometheus.CounterVec,
) (*prometheus.CounterVec, error) {
	if err := registerer.Register(collector); err != nil {
		alreadyRegistered, ok := err.(prometheus.AlreadyRegisteredError)
		if !ok {
			return nil, fmt.Errorf("register counter metric: %w", err)
		}
		existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec)
		if !ok {
			return nil, fmt.Errorf("registered counter metric has an incompatible type")
		}

		return existing, nil
	}

	return collector, nil
}

func registerHistogramVec(
	registerer prometheus.Registerer,
	collector *prometheus.HistogramVec,
) (*prometheus.HistogramVec, error) {
	if err := registerer.Register(collector); err != nil {
		alreadyRegistered, ok := err.(prometheus.AlreadyRegisteredError)
		if !ok {
			return nil, fmt.Errorf("register histogram metric: %w", err)
		}
		existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.HistogramVec)
		if !ok {
			return nil, fmt.Errorf("registered histogram metric has an incompatible type")
		}

		return existing, nil
	}

	return collector, nil
}
