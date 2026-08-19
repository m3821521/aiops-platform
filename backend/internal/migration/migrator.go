package migration

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// Migrator 是数据库迁移抽象接口。
// 当前实现使用 GORM AutoMigrate（开发环境友好），
// 未来可替换为 versioned migration（golang-migrate / goose）。
type Migrator interface {
	Migrate(models ...any) error
}

// GormMigrator 基于 GORM AutoMigrate 的实现。
type GormMigrator struct {
	db *gorm.DB
}

func NewGormMigrator(db *gorm.DB) *GormMigrator {
	return &GormMigrator{db: db}
}

func (m *GormMigrator) Migrate(models ...any) error {
	if len(models) == 0 {
		return nil
	}
	if err := m.db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}
	slog.Info("database migration completed", "models", len(models))
	return nil
}
