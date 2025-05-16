package onboarding

import (
	"context"
	"github.com/DIMO-Network/oracle-example/internal/config"
	"github.com/DIMO-Network/oracle-example/internal/gateway"
	"github.com/DIMO-Network/oracle-example/internal/models"
	"github.com/DIMO-Network/oracle-example/internal/service"
	"github.com/DIMO-Network/shared/pkg/logfields"
	"github.com/rs/zerolog"
)

type VendorCapabilityStatus struct {
	VIN    string `json:"vin"`
	Status string `json:"status"`
}

type VendorConnectionStatus struct {
	VIN        string `json:"vin"`
	ExternalID string `json:"externalId"`
	Status     string `json:"status"`
}

type VendorOnboardingAPI interface {
	Validate(vins []string) ([]VendorCapabilityStatus, error)
	Connect(vins []string) ([]VendorConnectionStatus, error)
	//Disconnect(identifier string) (bool, error)
}

type ExternalOnboardingService struct {
	db                *service.Vehicle
	logger            *zerolog.Logger
	externalVendorAPI gateway.ExternalVendorAPI
	enrollmentChannel chan models.EnrollmentMessage
}

func NewExternalOnboardingService(settings *config.Settings, db *service.Vehicle, logger *zerolog.Logger, enrollmentChannel chan models.EnrollmentMessage) *ExternalOnboardingService {
	return &ExternalOnboardingService{
		db:                db,
		logger:            logger,
		externalVendorAPI: gateway.NewExternalVendorAPI(logger, settings),
		enrollmentChannel: enrollmentChannel,
	}
}

func (s *ExternalOnboardingService) Validate(vins []string) ([]VendorCapabilityStatus, error) {
	s.logger.Debug().Strs("vins", vins).Msg("onboarding.Validate")

	capabilities, err := s.externalVendorAPI.ValidateVehicles(vins)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to validate vehicles")
		return nil, err
	}

	result := make([]VendorCapabilityStatus, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, VendorCapabilityStatus{
			VIN:    capability.VIN,
			Status: capability.ConnectedCapability,
		})
	}

	return result, nil
}

func (s *ExternalOnboardingService) Connect(vins []string) ([]VendorConnectionStatus, error) {
	s.logger.Debug().Strs("vins", vins).Msg("onboarding.Connect")

	vehicles, err := s.externalVendorAPI.EnrollVehicles(vins)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to enroll vehicles")
		return nil, err
	}

	result := make([]VendorConnectionStatus, 0, len(vehicles))
	vinsToWait := make([]string, 0, len(vehicles))

	for _, vehicle := range vehicles {
		if err := s.db.UpdateEnrollmentStatus(context.Background(), vehicle.VIN, vehicle.Status, vehicle.ID); err != nil {
			return nil, err
		}

		s.logger.Debug().Str(logfields.VIN, vehicle.VIN).Str("externalId", vehicle.ID).Str("status", vehicle.Status).Msg("onboarding.Connect.vehicle")
		if vehicle.Status != "succeeded" {
			vinsToWait = append(vinsToWait, vehicle.VIN)
		}
	}

	for len(vinsToWait) > 0 {
		s.logger.Debug().Strs("vinsToWait", vinsToWait).Msg("onboarding.Connect.waiting")

		// TODO: Add timeout
		message := <-s.enrollmentChannel

		n := 0
		for _, x := range vinsToWait {
			if x != message.VIN {
				vinsToWait[n] = x
				n++
			} else {
				s.logger.Debug().Str(logfields.VIN, message.VIN).Str("externalId", message.ID).Str("status", message.Status).Msg("connection successfully initiated")
				result = append(result, VendorConnectionStatus{
					VIN:        message.VIN,
					ExternalID: message.ID,
					Status:     message.Status,
				})
				break
			}
		}

		vinsToWait = vinsToWait[:n]
	}

	return result, nil
}
