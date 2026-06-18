package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	apikeyrepo "github.com/dipu/atmos-core/internal/apikey/repository"
	"github.com/dipu/atmos-core/platform/jwt"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const UserIDKey = "userID"

func RequireAuth(jwtManager *jwt.Manager, apiKeyRepo *apikeyrepo.APIKeyRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			return response.Unauthorized(c, "missing or malformed authorization header")
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")

		if strings.HasPrefix(tokenStr, "atm_") {
			h := sha256.Sum256([]byte(tokenStr))
			hash := hex.EncodeToString(h[:])

			key, err := apiKeyRepo.FindByHash(c.Context(), hash)
			if err != nil || !key.IsActive() {
				return response.Unauthorized(c, "invalid or revoked api key")
			}

			c.Locals(UserIDKey, key.UserID)
			// Use Background so the update outlives the request context.
			go apiKeyRepo.UpdateLastUsed(context.Background(), key.ID) //nolint:errcheck
			return c.Next()
		}

		claims, err := jwtManager.ParseAccessToken(tokenStr)
		if err != nil {
			return response.Unauthorized(c, "invalid or expired token")
		}

		c.Locals(UserIDKey, claims.UserID)
		return c.Next()
	}
}

func CurrentUserID(c *fiber.Ctx) uuid.UUID {
	id, _ := c.Locals(UserIDKey).(uuid.UUID)
	return id
}
