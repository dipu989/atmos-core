package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/dipu/atmos-core/internal/apikey/domain"
	"github.com/dipu/atmos-core/internal/apikey/dto"
	"github.com/dipu/atmos-core/internal/apikey/repository"
	"github.com/google/uuid"
)

var (
	ErrLimitReached = errors.New("api key limit reached")
	ErrNotFound     = errors.New("api key not found")
)

type APIKeyService struct {
	repo *repository.APIKeyRepository
}

func NewAPIKeyService(repo *repository.APIKeyRepository) *APIKeyService {
	return &APIKeyService{repo: repo}
}

func (s *APIKeyService) Create(ctx context.Context, userID uuid.UUID, name string) (*dto.CreateAPIKeyResponse, error) {
	count, err := s.repo.CountActiveByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if count >= domain.MaxKeysPerUser {
		return nil, ErrLimitReached
	}

	raw, err := generateRawKey()
	if err != nil {
		return nil, err
	}

	hash := sha256hex(raw)
	prefix := raw[:12]

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	key := &domain.APIKey{
		ID:      id,
		UserID:  userID,
		Name:    name,
		KeyHash: hash,
		Prefix:  prefix,
	}

	if err := s.repo.Create(ctx, key); err != nil {
		return nil, err
	}

	return &dto.CreateAPIKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Key:       raw,
		Prefix:    prefix,
		CreatedAt: key.CreatedAt,
	}, nil
}

func (s *APIKeyService) List(ctx context.Context, userID uuid.UUID) ([]dto.APIKeyItem, error) {
	keys, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.APIKeyItem, len(keys))
	for i, k := range keys {
		items[i] = dto.APIKeyItem{
			ID:         k.ID,
			Name:       k.Name,
			Prefix:     k.Prefix,
			LastUsedAt: k.LastUsedAt,
			ExpiresAt:  k.ExpiresAt,
			CreatedAt:  k.CreatedAt,
		}
	}
	return items, nil
}

func (s *APIKeyService) Revoke(ctx context.Context, id, userID uuid.UUID) error {
	err := s.repo.Revoke(ctx, id, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func generateRawKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "atm_" + hex.EncodeToString(b), nil
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
