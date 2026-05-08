package response

import "github.com/gofiber/fiber/v2"

type envelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

func OK(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(envelope{Success: true, Data: data})
}

func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(envelope{Success: true, Data: data})
}

func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func BadRequest(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(envelope{Success: false, Error: msg})
}

func Unauthorized(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(envelope{Success: false, Error: msg})
}

func Forbidden(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusForbidden).JSON(envelope{Success: false, Error: msg})
}

func NotFound(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusNotFound).JSON(envelope{Success: false, Error: msg})
}

func Conflict(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusConflict).JSON(envelope{Success: false, Error: msg})
}

func InternalError(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(envelope{Success: false, Error: msg})
}
