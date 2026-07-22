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

package telemetry

import (
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const statusClassLabel = "status_class"

// HTTPMetrics contains bounded-cardinality HTTP server metrics.
type HTTPMetrics struct {
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestsInFlight *prometheus.GaugeVec
}

// NewHTTPMetrics registers HTTP server metrics with registerer.
func NewHTTPMetrics(
	registerer prometheus.Registerer,
	serviceName string,
) (*HTTPMetrics, error) {
	if registerer == nil {
		return nil, fmt.Errorf("prometheus registerer is required")
	}
	if serviceName == "" {
		return nil, fmt.Errorf("telemetry service name is required")
	}

	registerer = prometheus.WrapRegistererWith(
		prometheus.Labels{"service": serviceName},
		registerer,
	)
	metrics := &HTTPMetrics{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "Total HTTP server requests.",
		}, []string{methodLabel, routeLabel, statusClassLabel}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_server_request_duration_seconds",
			Help:    "HTTP server request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{methodLabel, routeLabel, statusClassLabel}),
		requestsInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "http_server_requests_in_flight",
			Help: "Current HTTP server requests.",
		}, []string{methodLabel, routeLabel}),
	}

	for _, collector := range []prometheus.Collector{
		metrics.requestsTotal,
		metrics.requestDuration,
		metrics.requestsInFlight,
	} {
		if err := registerer.Register(collector); err != nil {
			return nil, fmt.Errorf("register HTTP metric: %w", err)
		}
	}

	return metrics, nil
}

func (metrics *HTTPMetrics) requestStarted(method, route string) {
	metrics.requestsInFlight.WithLabelValues(method, route).Inc()
}

func (metrics *HTTPMetrics) requestFinished(
	method string,
	route string,
	recorder *responseRecorder,
	started time.Time,
) {
	statusClass := strconv.Itoa(recorder.statusCode()/100) + "xx"
	metrics.requestsInFlight.WithLabelValues(method, route).Dec()
	metrics.requestsTotal.WithLabelValues(method, route, statusClass).Inc()
	metrics.requestDuration.WithLabelValues(method, route, statusClass).
		Observe(time.Since(started).Seconds())
}
