package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			id, _ := uuid.NewV7()
			requestID = id.String()
		}
		c.Set("X-Request-ID", requestID)
		c.Locals("requestID", requestID)
		return c.Next()
	}
}
