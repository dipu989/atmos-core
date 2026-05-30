package middleware

import (
	"crypto/subtle"

	"github.com/dipu/atmos-core/platform/response"
	"github.com/gofiber/fiber/v2"
)

// RequireInternalKey protects internal-only endpoints (e.g. /internal/gmail/sync-all)
// that are called by the Linux cron job, not by end-users.
//
// The caller must send the key in the X-Internal-Key header.
// Comparison uses subtle.ConstantTimeCompare to prevent timing attacks.
func RequireInternalKey(key string) fiber.Handler {
	keyBytes := []byte(key)
	return func(c *fiber.Ctx) error {
		provided := c.Get("X-Internal-Key")
		if len(provided) == 0 || subtle.ConstantTimeCompare([]byte(provided), keyBytes) != 1 {
			return response.Unauthorized(c, "missing or invalid internal key")
		}
		return c.Next()
	}
}
