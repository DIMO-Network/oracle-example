package controllers

import (
	"fmt"
	dbmodels "github.com/DIMO-Network/oracle-example/internal/db/models"
	"github.com/DIMO-Network/oracle-example/internal/gateway"
	"github.com/DIMO-Network/oracle-example/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/volatiletech/null/v8"
)

type EnrollmentController struct {
	// database
	db                *service.Vehicle
	logger            *zerolog.Logger
	externalVendorAPI gateway.ExternalVendorAPI
}

type EnrollmentPayload struct {
	Vin string `json:"vin"`
}

func NewEnrollmentController(externalVendorAPI gateway.ExternalVendorAPI, db *service.Vehicle, logger *zerolog.Logger) *EnrollmentController {
	return &EnrollmentController{
		db:                db,
		logger:            logger,
		externalVendorAPI: externalVendorAPI,
	}
}

// EnrollVehicle
// @Summary Enrolls a vehicle using the provided VIN
// @Description Enrolls a vehicle in the external system
// @Produce json
// @Param payload body EnrollmentPayload true "Enrollment Payload"
// @Success 200 {object} fiber.Map
// @Failure 400 {object} fiber.Map
// @Failure 500 {object} fiber.Map
// @Router /v1/vehicle/enroll [post]
func (e *EnrollmentController) EnrollVehicle(c *fiber.Ctx) error {
	// Parse request body
	var payload EnrollmentPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request payload",
		})
	}

	if len(payload.Vin) != 17 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid VIN",
		})
	}

	vehicles, err := e.externalVendorAPI.EnrollVehicles([]string{payload.Vin})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to enroll vehicle. " + err.Error(),
		})
	}
	// persist to db
	for _, vehicle := range vehicles {
		newVin := dbmodels.Vin{
			Vin:              vehicle.VIN,
			ExternalID:       null.StringFrom(vehicle.ID),
			ConnectionStatus: null.StringFrom(vehicle.Status),
		}
		// Write to database
		if err := e.db.InsertVinToDB(c.Context(), &newVin); err != nil {
			return fmt.Errorf("failed to write to database: %v", err)
		}
	}

	return c.JSON(fiber.Map{
		"message": "Vehicle enrolled successfully",
	})
}
