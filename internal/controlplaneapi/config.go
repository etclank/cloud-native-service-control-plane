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

// Package controlplaneapi provides HTTP and configuration behavior for the
// platform control-plane API.
package controlplaneapi

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/etclank/cloud-native-service-control-plane/internal/telemetry"
)

const (
	// DefaultPort is used when PORT is not configured.
	DefaultPort = 8080
	// DefaultMetricsPort is used when METRICS_PORT is not configured.
	DefaultMetricsPort = telemetry.DefaultMetricsPort
	// MinimumTokenLength is the minimum accepted API bearer token length.
	MinimumTokenLength = 32
)

// LoadToken reads and validates an API bearer token from path.
func LoadToken(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("API_TOKEN_FILE is required")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read API token file: %w", err)
	}

	contents = trimTrailingLineEnding(contents)
	if len(contents) == 0 {
		return "", fmt.Errorf("API token is empty")
	}
	if bytes.ContainsAny(contents, "\r\n") {
		return "", fmt.Errorf("API token contains an unexpected line ending")
	}
	if len(contents) < MinimumTokenLength {
		return "", fmt.Errorf(
			"API token must contain at least %d characters",
			MinimumTokenLength,
		)
	}

	return string(contents), nil
}

// ParsePort parses PORT, applying the default when the value is empty.
func ParsePort(value string) (int, error) {
	if value == "" {
		return DefaultPort, nil
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf(
			"PORT must be an integer between 1 and 65535: %q",
			value,
		)
	}

	return port, nil
}

// ParseMetricsPort parses METRICS_PORT and ensures the internal listener does
// not share the public API port.
func ParseMetricsPort(value string, publicPort int) (int, error) {
	return telemetry.ParseMetricsPort(value, publicPort)
}

func trimTrailingLineEnding(contents []byte) []byte {
	if bytes.HasSuffix(contents, []byte("\r\n")) {
		return contents[:len(contents)-2]
	}
	if bytes.HasSuffix(contents, []byte("\n")) {
		return contents[:len(contents)-1]
	}

	return contents
}
