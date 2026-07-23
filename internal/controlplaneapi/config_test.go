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

package controlplaneapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadToken(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "without line ending", contents: testToken, want: testToken},
		{name: "newline", contents: testToken + "\n", want: testToken},
		{name: "Windows line ending", contents: testToken + "\r\n", want: testToken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTokenFile(t, test.contents)
			token, err := LoadToken(path)
			if err != nil {
				t.Fatalf("LoadToken() error = %v", err)
			}
			if token != test.want {
				t.Errorf("token does not match expected value")
			}
		})
	}
}

func TestLoadTokenRejectsInvalidConfiguration(t *testing.T) {
	shortToken := "sensitive-short-token"
	tests := []struct {
		name       string
		path       func(*testing.T) string
		wantError  string
		notInError string
	}{
		{
			name:      "missing path",
			path:      func(*testing.T) string { return "" },
			wantError: "API_TOKEN_FILE is required",
		},
		{
			name: "missing file",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing-token")
			},
			wantError: "read API token file",
		},
		{
			name:      "empty token",
			path:      func(t *testing.T) string { return writeTokenFile(t, "") },
			wantError: "API token is empty",
		},
		{
			name: "short token",
			path: func(t *testing.T) string {
				return writeTokenFile(t, shortToken)
			},
			wantError:  "at least 32 characters",
			notInError: shortToken,
		},
		{
			name: "multiple line endings",
			path: func(t *testing.T) string {
				return writeTokenFile(t, testToken+"\n\n")
			},
			wantError: "unexpected line ending",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadToken(test.path(t))
			if err == nil {
				t.Fatal("LoadToken() returned no error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Errorf("error = %q, want substring %q", err, test.wantError)
			}
			if test.notInError != "" && strings.Contains(err.Error(), test.notInError) {
				t.Error("error exposed token contents")
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      int
		wantError bool
	}{
		{name: "default", value: "", want: DefaultPort},
		{name: "custom", value: "8443", want: 8443},
		{name: "zero", value: "0", wantError: true},
		{name: "negative", value: "-1", wantError: true},
		{name: "too large", value: "65536", wantError: true},
		{name: "non-numeric", value: "https", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port, err := ParsePort(test.value)
			if test.wantError {
				if err == nil {
					t.Fatalf("ParsePort(%q) returned no error", test.value)
				}

				return
			}
			if err != nil {
				t.Fatalf("ParsePort(%q) error = %v", test.value, err)
			}
			if port != test.want {
				t.Errorf("ParsePort(%q) = %d, want %d", test.value, port, test.want)
			}
		})
	}
}

func TestParseMetricsPort(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		publicPort int
		want       int
		wantError  bool
	}{
		{name: "default", publicPort: DefaultPort, want: DefaultMetricsPort},
		{name: "custom", value: "9191", publicPort: DefaultPort, want: 9191},
		{name: "same as public", value: "8080", publicPort: DefaultPort, wantError: true},
		{name: "default same as public", publicPort: DefaultMetricsPort, wantError: true},
		{name: "zero", value: "0", publicPort: DefaultPort, wantError: true},
		{name: "negative", value: "-1", publicPort: DefaultPort, wantError: true},
		{name: "too large", value: "65536", publicPort: DefaultPort, wantError: true},
		{name: "non-numeric", value: metricsPortName, publicPort: DefaultPort, wantError: true},
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

func writeTokenFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "api-token")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	return path
}
