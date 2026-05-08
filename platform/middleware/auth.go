package middleware

import (
	"strings"

	"github.com/dipu/atmos-core/platform/jwt"
	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const UserIDKey = "userID"

func RequireAuth(jwtManager *jwt.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			return response.Unauthorized(c, "missing or malformed authorization header")
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
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
