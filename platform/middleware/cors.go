package middleware

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func CORS() fiber.Handler {
	origin := os.Getenv("CORS_ALLOW_ORIGIN")

	// In production CORS_ALLOW_ORIGIN must be explicitly set.
	// For non-production environments (local dev, staging) fall back to localhost.
	if origin == "" {
		if os.Getenv("APP_ENV") == "production" {
			// Fail loudly — a missing CORS origin in production is a misconfiguration
			panic("CORS_ALLOW_ORIGIN must be set in production")
		}
		origin = "http://localhost:3000"
	}

	return cors.New(cors.Config{
		AllowOrigins:     origin,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID",
		ExposeHeaders:    "X-Request-ID",
		AllowCredentials: false,
		MaxAge:           86400,
	})
}
