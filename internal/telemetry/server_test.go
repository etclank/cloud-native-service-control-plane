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
	"context"
	"net/http"
	"testing"
	"time"
)

func TestParseMetricsPort(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		publicPort int
		want       int
		wantError  bool
	}{
		{name: "default", publicPort: 8080, want: DefaultMetricsPort},
		{name: "custom", value: "9191", publicPort: 8080, want: 9191},
		{name: "same as public", value: "8080", publicPort: 8080, wantError: true},
		{name: "default collision", publicPort: DefaultMetricsPort, wantError: true},
		{name: "zero", value: "0", publicPort: 8080, wantError: true},
		{name: "negative", value: "-1", publicPort: 8080, wantError: true},
		{name: "too large", value: "65536", publicPort: 8080, wantError: true},
		{name: "non-numeric", value: "metrics", publicPort: 8080, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port, err := ParseMetricsPort(test.value, test.publicPort)
			if test.wantError {
				if err == nil {
					t.Fatalf("ParseMetricsPort(%q, %d) returned no error", test.value, test.publicPort)
				}

				return
			}
			if err != nil {
				t.Fatalf("ParseMetricsPort(%q, %d) error = %v", test.value, test.publicPort, err)
			}
			if port != test.want {
				t.Errorf("ParseMetricsPort(%q, %d) = %d, want %d", test.value, test.publicPort, port, test.want)
			}
		})
	}
}

func TestServeUntilShutdownStopsAllServers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ServeUntilShutdown(
		ctx,
		discardLogger(),
		time.Second,
		NamedServer{
			Name: "public demo",
			Server: &http.Server{
				Addr:    "127.0.0.1:0",
				Handler: http.NewServeMux(),
			},
		},
		NamedServer{
			Name: "internal metrics",
			Server: &http.Server{
				Addr:    "127.0.0.1:0",
				Handler: http.NewServeMux(),
			},
		},
	)
	if err != nil {
		t.Fatalf("ServeUntilShutdown() error = %v", err)
	}
}
