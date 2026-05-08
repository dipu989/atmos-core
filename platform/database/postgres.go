package database

import (
	"github.com/dipu/atmos-core/config"
	"github.com/dipu/atmos-core/platform/logger"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func Connect(cfg *config.DBConfig) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
		PrepareStmt:                              true,
		DisableForeignKeyConstraintWhenMigrating: false,
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), gormCfg)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.MaxLife)

	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	logger.L().Info("database connected", zap.String("host", cfg.Host), zap.String("name", cfg.Name))
	return db, nil
}
