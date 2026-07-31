package main

import (
	"log"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/server"
)

func main() {
	cfg := config.Load()

	srv := server.New(cfg)

	log.Printf("starting ops-platform-starter backend on :%s in %s mode", cfg.Port, cfg.AppEnv)

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
