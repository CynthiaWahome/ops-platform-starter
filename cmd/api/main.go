package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/internal/server"
)

func main() {
	cfg := config.Load()

	// ctx is cancelled the moment the process receives SIGINT/SIGTERM
	// (Ctrl+C, or a container orchestrator asking it to stop) — this is
	// what lets Shutdown below run instead of the process just dying
	// mid-request with an open database pool.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("starting ops-platform-starter backend on :%s in %s mode", cfg.Port, cfg.AppEnv)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
		log.Println("shutdown signal received, closing connections")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
	}
}
