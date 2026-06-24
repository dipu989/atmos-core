package shortaddress

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// geocodeCacheRow maps to the geocode_cache table (migration 025).
type geocodeCacheRow struct {
	LatRounded     float64   `gorm:"column:lat_rounded;primaryKey"`
	LngRounded     float64   `gorm:"column:lng_rounded;primaryKey"`
	DisplayAddress string    `gorm:"column:display_address"`
	PlaceID        string    `gorm:"column:place_id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (geocodeCacheRow) TableName() string { return "geocode_cache" }

// GormCache is a Cache backed by the geocode_cache Postgres table.
type GormCache struct {
	db *gorm.DB
}

func NewGormCache(db *gorm.DB) *GormCache {
	return &GormCache{db: db}
}

func (c *GormCache) Get(ctx context.Context, latRounded, lngRounded float64) (string, bool, error) {
	var row geocodeCacheRow
	err := c.db.WithContext(ctx).
		Where("lat_rounded = ? AND lng_rounded = ?", latRounded, lngRounded).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.DisplayAddress, true, nil
}

func (c *GormCache) Put(ctx context.Context, latRounded, lngRounded float64, placeID, displayAddress string) error {
	row := geocodeCacheRow{
		LatRounded:     latRounded,
		LngRounded:     lngRounded,
		DisplayAddress: displayAddress,
		PlaceID:        placeID,
		CreatedAt:      time.Now(),
	}
	return c.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}
