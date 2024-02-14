package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const defaultDrainDuration = 2 * time.Second

type AdminInterface struct {
	logger *zap.Logger
}

func NewAdminInterface(l *zap.Logger) *AdminInterface {
	return &AdminInterface{logger: l}
}

func (a *AdminInterface) Run(ctx context.Context, config AdminConfig, healthcheck http.Handler) error {
	sm := http.NewServeMux()
	sm.Handle("/metrics", promhttp.Handler())
	sm.Handle("/health", healthcheck)

	// The debug/pprof endpoints follow the pattern from the init function in net/http/pprof package.
	// ref: https://cs.opensource.google/go/go/+/refs/tags/go1.19.5:src/net/http/pprof/pprof.go;l=80-86
	sm.HandleFunc("/debug/pprof/", pprof.Index)
	sm.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	sm.HandleFunc("/debug/pprof/profile", pprof.Profile)
	sm.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	sm.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Addr: fmt.Sprintf(":%d", config.Port), Handler: sm, ReadHeaderTimeout: 15 * time.Second}

	a.logger.Debug("Starting admin interface")
	errs := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errs <- err
		}
	}()

	select {
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), defaultDrainDuration)
		defer func() {
			cancel()
		}()

		if err := server.Shutdown(ctx); err != nil {
			return err
		}

		return nil
	case err := <-errs:
		return err
	}
}

func HealthHandlerFunc(isHealthy func() bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isHealthy() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}
}
