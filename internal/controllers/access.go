package controllers

import (
	"github.com/gofiber/fiber/v2"
)

type AccessController struct{}

func NewAccessController() *AccessController {
	return &AccessController{}
}
func (a *AccessController) CheckAccess(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{})
}
