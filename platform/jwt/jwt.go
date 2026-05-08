package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

type Claims struct {
	UserID    uuid.UUID `json:"uid"`
	TokenType TokenType `json:"typ"`
	jwt.RegisteredClaims
}

type Manager struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
}

func NewManager(accessSecret, refreshSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
	}
}

func (m *Manager) IssueAccessToken(userID uuid.UUID) (string, error) {
	return m.sign(userID, AccessToken, m.accessSecret, m.accessTTL)
}

func (m *Manager) IssueRefreshToken(userID uuid.UUID) (string, error) {
	return m.sign(userID, RefreshToken, m.refreshSecret, m.refreshTTL)
}

func (m *Manager) ParseAccessToken(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr, m.accessSecret)
}

func (m *Manager) ParseRefreshToken(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr, m.refreshSecret)
}

func (m *Manager) AccessTTL() time.Duration  { return m.accessTTL }
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

func (m *Manager) sign(userID uuid.UUID, tt TokenType, secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:    userID,
		TokenType: tt,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

func (m *Manager) parse(tokenStr string, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
