// cmd/seed runs all database seeders.
// Safe to run multiple times — all seeders are idempotent.
//
// Usage:
//
//	go run ./cmd/seed
package main

import (
	"context"

	"github.com/dipu/atmos-core/config"
	"github.com/dipu/atmos-core/internal/seeds"
	"github.com/dipu/atmos-core/platform/database"
	"github.com/dipu/atmos-core/platform/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger.Init(cfg.App.Env)
	defer logger.Sync()
	log := logger.L()

	db, err := database.Connect(&cfg.DB)
	if err != nil {
		log.Fatal("database connection failed", zap.Error(err))
	}

	log.Info("starting seeders")
	seeds.Run(context.Background(), db, seeds.All())
	log.Info("all seeders complete")
}
