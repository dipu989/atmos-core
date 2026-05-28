package repository

import (
	"context"
	"errors"

	"github.com/dipu/atmos-core/internal/gmail/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConnectionRepository struct {
	db *gorm.DB
}

func NewConnectionRepository(db *gorm.DB) *ConnectionRepository {
	return &ConnectionRepository{db: db}
}

func (r *ConnectionRepository) Upsert(ctx context.Context, conn *domain.GmailConnection) error {
	return r.db.WithContext(ctx).
		Where(domain.GmailConnection{UserID: conn.UserID}).
		Assign(domain.GmailConnection{
			Email:        conn.Email,
			AccessToken:  conn.AccessToken,
			RefreshToken: conn.RefreshToken,
			TokenExpiry:  conn.TokenExpiry,
			ConnectedAt:  conn.ConnectedAt,
		}).
		FirstOrCreate(conn).Error
}

func (r *ConnectionRepository) Save(ctx context.Context, conn *domain.GmailConnection) error {
	return r.db.WithContext(ctx).Save(conn).Error
}

func (r *ConnectionRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.GmailConnection, error) {
	var conn domain.GmailConnection
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&conn).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &conn, err
}

func (r *ConnectionRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&domain.GmailConnection{}).Error
}

// FindAllConnected returns every connection — used by the background daily scheduler.
func (r *ConnectionRepository) FindAllConnected(ctx context.Context) ([]domain.GmailConnection, error) {
	var conns []domain.GmailConnection
	err := r.db.WithContext(ctx).Find(&conns).Error
	return conns, err
}
