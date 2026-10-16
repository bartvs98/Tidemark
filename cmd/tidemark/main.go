// Command tidemark runs the Tidemark server.
//
//	tidemark              start the server
//	tidemark healthcheck  exit 0 if the local server answers /api/health (for Docker HEALTHCHECK)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bartvs98/tidemark/internal/api"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/config"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/icons"
	"github.com/bartvs98/tidemark/internal/registry"
	"github.com/bartvs98/tidemark/web"
)

// version is set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(cfg.Addr))
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := run(cfg, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	source, err := dockerSource(cfg)
	if err != nil {
		return err
	}
	info := source.Info()
	log.Info("container source", "kind", info.Kind, "location", info.Location)

	var iconSvc *icons.Service
	if cfg.Icons {
		iconSvc = icons.New(cfg.IconsURL, filepath.Join(cfg.DataDir, "icons"), log)
		iconSvc.Start(ctx)
	}

	checks := check.New(source, registry.New("tidemark/"+version), log)
	checks.Start(ctx) // first check right away; step 3 adds a schedule

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: api.New(api.Deps{
			Version:    version,
			Log:        log,
			Source:     source,
			Icons:      iconSvc,
			Checks:     checks,
			Background: ctx,
		}).Handler(web.SPA()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("tidemark listening", "addr", cfg.Addr, "version", version)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func dockerSource(cfg config.Config) (docker.Source, error) {
	if cfg.DockerSnapshot != "" {
		return docker.NewSnapshot(cfg.DockerSnapshot), nil
	}
	return docker.NewClient(cfg.DockerHost)
}

func healthcheck(addr string) int {
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + host + "/api/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy:", resp.Status)
		return 1
	}
	return 0
}
