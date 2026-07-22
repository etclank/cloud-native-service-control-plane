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

package main

import (
	"context"
	"net/http"
	"testing"
)

func TestServeUntilShutdownStopsBothServers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	servers := []serverSpec{
		{
			name: "public API",
			server: &http.Server{
				Addr:    "127.0.0.1:0",
				Handler: http.NewServeMux(),
			},
		},
		{
			name: "internal metrics",
			server: &http.Server{
				Addr:    "127.0.0.1:0",
				Handler: http.NewServeMux(),
			},
		},
	}

	if err := serveUntilShutdown(ctx, servers); err != nil {
		t.Fatalf("serveUntilShutdown() error = %v", err)
	}
}
