package mysql

import (
	"time"

	"gorm.io/gorm"
)

// SQLModel contains common GORM model fields for MySQL records.
type SQLModel struct {
	ID        string         `gorm:"column:id;type:varchar(32);primaryKey"`
	CreatedAt time.Time      `gorm:"column:created_at;type:datetime(6);not null;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:datetime(6);not null;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:datetime(6);index"`
}
