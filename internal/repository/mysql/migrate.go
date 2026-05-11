package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// AutoMigrate creates and updates database tables managed by the MySQL repositories.
func AutoMigrate(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&postRecord{}, &userRecord{}, &refreshTokenRecord{}); err != nil {
		return fmt.Errorf("auto migrate mysql tables: %w", err)
	}

	return nil
}
