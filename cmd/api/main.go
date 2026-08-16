package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/internal/server"
	"github.com/joho/godotenv"
)

func main() {
	// godotenv.Load reads .env in the current directory into the real
	// process environment, before config.Load ever runs — config.Load
	// only ever reads os.LookupEnv, plain `go run`/`go build` binaries
	// don't source .env files on their own the way some other language
	// runtimes do. Without this, the README's own "cp .env.example .env,
	// then edit it" instructions would silently do nothing: every value
	// (including the JWT secret) would fall back to config.Load's
	// hardcoded defaults no matter what the file said.
	//
	// A missing .env is fine — every value already has a workable
	// default (see config.Load) — so only a malformed/unreadable file
	// that exists is treated as fatal, not the common "no .env at all"
	// case.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("failed to load .env: %v", err)
	}

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
