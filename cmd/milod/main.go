package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/callmemhz/milo/internal/bootstrap"
	"github.com/callmemhz/milo/internal/config"
	"github.com/callmemhz/milo/internal/deploy"
	"github.com/callmemhz/milo/internal/docker"
	"github.com/callmemhz/milo/internal/server"
	"github.com/callmemhz/milo/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(2)
	}

	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		log.Error("state-dir", "err", err)
		os.Exit(1)
	}

	s, err := store.Open(filepath.Join(cfg.StateDir, "milo.db"))
	if err != nil {
		log.Error("store", "err", err)
		os.Exit(1)
	}
	defer s.Close()

	d, err := docker.New(docker.Config{
		Network:          cfg.Network,
		RegistryUser:     cfg.GHCRUser,
		RegistryPassword: cfg.GHCRToken,
	})
	if err != nil {
		log.Error("docker", "err", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.EnsureNetwork(context.Background()); err != nil {
		log.Error("network", "err", err)
		os.Exit(1)
	}

	if err := bootstrap.EnsureAdmin(context.Background(), s, log); err != nil {
		log.Error("bootstrap", "err", err)
		os.Exit(1)
	}

	h := &deploy.Hygiene{Store: s, Docker: d, Log: log}
	if err := h.Run(context.Background()); err != nil {
		log.Warn("hygiene", "err", err)
	}

	locks := deploy.NewLockManager()
	orch := &deploy.Orchestrator{
		Store: s, Docker: d, LockMgr: locks, Log: log,
	}

	srv := server.New(s, cfg.Version)
	srv.Deployer = orch
	srv.Docker = d
	srv.RootDomain = cfg.RootDomain

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
