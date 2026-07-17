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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/etclank/cloud-native-service-control-plane/internal/controlplaneapi"
)

const (
	shutdownTimeout = 10 * time.Second
	maxHeaderBytes  = 16 << 10
)

func main() {
	if err := run(); err != nil {
		slog.Error("Control-plane API server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	token, err := controlplaneapi.LoadToken(os.Getenv("API_TOKEN_FILE"))
	if err != nil {
		return fmt.Errorf("configure API token: %w", err)
	}

	port, err := controlplaneapi.ParsePort(os.Getenv("PORT"))
	if err != nil {
		return fmt.Errorf("configure server: %w", err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           controlplaneapi.NewHandler(token, nil),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	serverErrors := make(chan error, 1)
	slog.Info("Starting control-plane API server", "address", server.Addr)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}

		return nil
	case <-ctx.Done():
		slog.Info("Shutting down control-plane API server")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			shutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}

		if err := <-serverErrors; err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}

		slog.Info("Control-plane API server stopped")

		return nil
	}
}
