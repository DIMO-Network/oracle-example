package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DIMO-Network/oracle-example/internal/config"
	"github.com/friendsofgo/errors"
	"github.com/rs/zerolog"
	"io"
	"net/http"
	"time"
)

/**
* This is an example implementation of an API wrapper for your external API that handles enrollment or any operations that are required to integrate with your external API.
* This is not a complete implementation and is only meant to showcase how to integrate with your external API.
* You can use this as a reference for your implementation.
*
* The external vendor API is used to validate and enroll vehicles.
* If you don't use streaming, it could also be used to periodically query the external API for vehicle telemetry.
*
 */

var ErrVehicleAlreadyEnrolled = errors.New("vehicle already enrolled")

//go:generate mockgen -source external_vendor_api.go -destination mocks/external_vendor_api_mock.go -package mocks
type ExternalVendorAPI interface {
	ValidateVehicles(vins []string) (CapabilityItems, error)
	EnrollVehicles(vins []string) (EnrolledItems, error)
	GetLatestTelemetry(vehicleId string) (*TelemetryResponse, error)
}

type externalVendorAPI struct {
	logger   *zerolog.Logger
	settings *config.Settings
}

func NewExternalVendorAPI(logger *zerolog.Logger, settings *config.Settings) ExternalVendorAPI {
	return &externalVendorAPI{
		logger:   logger,
		settings: settings,
	}
}

func (m *externalVendorAPI) ValidateVehicles(vins []string) (CapabilityItems, error) {
	token, err := m.getToken()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get token")
	}

	items, err := m.checkVehiclesCapabilities(token, vins)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate vehicle")
	}
	return items, nil
}

func (m *externalVendorAPI) EnrollVehicles(vins []string) (EnrolledItems, error) {
	token, err := m.getToken()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get token")
	}
	// Get Data Sources
	dataSource, account, dataServices, err := m.getDataSources(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get data sources")
	}

	result := EnrolledItems{}

	for _, vin := range vins {
		items, err := m.enrollVehicle(token, dataSource, account, dataServices, vin)
		if err != nil {
			if errors.Is(err, ErrVehicleAlreadyEnrolled) {
				enrollments, err := m.getEnrollmentsForVin(token, vin)
				if err != nil {
					return nil, errors.Wrap(err, "failed to get enrollments for already enrolled vehicle")
				}

				result = append(result, EnrolledItems{{
					VIN:    vin,
					Status: "succeeded",
					ID:     enrollments[0].VehicleID,
				}}...)
			} else {
				return nil, errors.Wrap(err, "failed to enroll vehicle")
			}
		}
		result = append(result, items...)
	}

	return result, nil
}

func (m *externalVendorAPI) GetLatestTelemetry(vehicleId string) (*TelemetryResponse, error) {
	token, err := m.getToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/fleet/v3/telemetry/latest", m.settings.ExternalVendorAPIURL)
	type TelemetryRequest struct {
		Fields     []string `json:"fields"`
		VehicleIds []string `json:"vehicleIds"`
	} // theres is also an option to add an orgId but I doubt we'll use for this
	payload := TelemetryRequest{
		Fields: []string{
			"location",
			"speed",
			"ignitionStatus",
			"odometer",
			"engineRuntime",
			"fuelLevel",
			"checkEngineLight",
			"engineOilLife",
			"tirePressure",
			"deviceConnectivityStatus",
			"evBatteryRange",
			"evBatteryLevel",
			"evChargingState",
		},
		VehicleIds: []string{vehicleId},
	}

	payloadBody, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payloadBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			m.logger.Err(err).Msg("Failed to close response body for enroll vehicle request")
		}
	}(resp.Body)
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMultiStatus {
		var response TelemetryResponse

		//fmt.Println(string(body))

		if err := json.Unmarshal(body, &response); err != nil {
			return nil, errors.Wrap(err, "failed to parse response")
		}

		return &response, nil
	}

	return nil, fmt.Errorf("failed to get vehicle telemetry, status code: %d, response: %s", resp.StatusCode, string(body))
}

func (m *externalVendorAPI) getToken() (string, error) {
	url := fmt.Sprintf("%s/fleet/v3/oauth/token", m.settings.ExternalVendorAPIURL)
	payload := map[string]string{
		"clientId":     m.settings.ClientID,
		"clientSecret": m.settings.ClientSecret,
		"audience":     m.settings.Audience,
		"grantType":    "client_credentials",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			m.logger.Err(err).Msg("Failed to close response body for token request")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token, status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	token, ok := result["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("access_token not found in response")
	}
	return token, nil
}

func (m *externalVendorAPI) getDataSources(token string) (string, string, []string, error) {
	url := fmt.Sprintf("%s/fleet/v3/data-sources", m.settings.ExternalVendorAPIURL)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			m.logger.Err(err).Msg("Failed to close response body for data sources request")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("failed to get data sources, status code: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", nil, err
	}

	items, ok := result["items"].([]interface{})
	if !ok || len(items) == 0 {
		return "", "", nil, fmt.Errorf("no data sources found")
	}

	firstItem := items[0].(map[string]interface{})
	dataSource := firstItem["name"].(string)
	account := firstItem["accounts"].([]interface{})[0].(string)
	dataServices := []string{}
	for _, dService := range firstItem["dataServices"].(map[string]interface{})["names"].([]interface{}) {
		dataServices = append(dataServices, dService.(string))
	}

	return dataSource, account, dataServices, nil
}

func (m *externalVendorAPI) checkVehiclesCapabilities(token string, vins []string) (CapabilityItems, error) {
	url := fmt.Sprintf("%s/fleet/v3/connected-capability/quick/vins", m.settings.ExternalVendorAPIURL)
	payload := map[string]interface{}{
		"vins": vins,
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			m.logger.Err(err).Msg("Failed to close response body for check vehicle capability request")
		}
	}(resp.Body)

	body, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to check vehicle capability, status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var response CapabilityResponse

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse capability response: %v", err)
	}

	if len(response.Items) == 0 {
		return nil, fmt.Errorf("no items found in capability response")
	}

	return response.Items, nil
}

func (m *externalVendorAPI) getEnrollmentsForVin(token, vin string) (Enrollments, error) {
	url := fmt.Sprintf("%s/fleet/v3/enrollments?vin=%s", m.settings.ExternalVendorAPIURL, vin)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			m.logger.Err(err).Msg("Failed to close response body for enroll vehicle request")
		}
	}(resp.Body)

	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		response := EnrollmentsResponse{}
		err = json.Unmarshal(body, &response)
		if err != nil {
			return nil, fmt.Errorf("failed to parse enrollments response: %v", err)
		}

		return response.Items, nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to enroll vehicle, status code: %d, response: %s", resp.StatusCode, string(body))
	}
}

func (m *externalVendorAPI) enrollVehicle(token, dataSource, account string, dataServices []string, vin string) (EnrolledItems, error) {
	url := fmt.Sprintf("%s/fleet/v3/enrollments", m.settings.ExternalVendorAPIURL)
	var payload []map[string]interface{}

	payload = append(payload, map[string]interface{}{
		"dataSource":                    dataSource,
		"account":                       account,
		"vin":                           vin,
		"dataServices":                  dataServices,
		"allowMultipleSourceEnrollment": false,
	})

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			m.logger.Err(err).Msg("Failed to close response body for enroll vehicle request")
		}
	}(resp.Body)

	if resp.StatusCode == http.StatusAccepted { // Handle 202 response
		responseBody, _ := io.ReadAll(resp.Body)
		var response EnrollResponse

		if err := json.Unmarshal(responseBody, &response); err != nil {
			return nil, fmt.Errorf("failed to parse 202 response: %v", err)
		}

		if len(response.Items) == 0 {
			return nil, fmt.Errorf("no items found in 202 response")
		}

		return response.Items, nil
	}

	if resp.StatusCode == http.StatusBadRequest {
		body, _ = io.ReadAll(resp.Body)
		errorResponse := ErrorResponse{}
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return nil, fmt.Errorf("failed to parse error response: %v", err)
		}

		if errorResponse.Error.Type == "VinAlreadyEnrolledForDataService" {
			return nil, ErrVehicleAlreadyEnrolled
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ = io.ReadAll(resp.Body)

		return nil, fmt.Errorf("failed to enroll vehicle, status code: %d, response: %s", resp.StatusCode, string(body))
	}

	return nil, nil
}

type ErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type CapabilityResponse struct {
	Items CapabilityItems `json:"items"`
}

type CapabilityItems []struct {
	VIN                         string `json:"vin"`
	Year                        int    `json:"year"`
	Make                        string `json:"make"`
	Model                       string `json:"model"`
	Trim                        string `json:"trim"`
	DataSource                  string `json:"dataSource"`
	DataSourceIntegrationStatus string `json:"dataSourceIntegrationStatus"`
	ConnectedCapability         string `json:"connectedCapability"`
}

type EnrollmentsResponse struct {
	Items Enrollments `json:"items"`
}

type Enrollments []struct {
	ID               string    `json:"id"`
	CreatedTimestamp time.Time `json:"createdTimestamp"`
	VehicleID        string    `json:"vehicleID"`
	DataSource       string    `json:"dataSource"`
	Account          string    `json:"account"`
	Vin              string    `json:"vin"`
	DataServices     []string  `json:"dataServices"`
	SerialNo         string    `json:"serialNo"`
}

type EnrollResponse struct {
	Items EnrolledItems `json:"items"`
}

type EnrolledItems []struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	VIN       string `json:"vin"`
	SubStatus struct {
		SourceTelemetryFuel struct {
			Status string `json:"status"`
		} `json:"source.telemetryFuel"`
	} `json:"subStatus"`
}

type TelemetryResponse struct {
	Items []struct {
		VehicleId string `json:"vehicleId"`
		DeviceId  string `json:"deviceId"`
		Location  struct {
			Timestamp time.Time `json:"timestamp"`
			Lat       float64   `json:"lat"`
			Lon       float64   `json:"lon"`
		} `json:"location"`
		Speed struct {
			Timestamp  time.Time `json:"timestamp"`
			Value      float64   `json:"value"`
			SignalType string    `json:"signalType"`
			Units      string    `json:"units"`
		} `json:"speed"`
		IgnitionStatus struct {
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"ignitionStatus"`
		Odometer struct {
			Timestamp  time.Time `json:"timestamp"`
			Value      float64   `json:"value"`
			SignalType string    `json:"signalType"`
			Units      string    `json:"units"`
		} `json:"odometer"`
		EngineRuntime struct {
			Timestamp  time.Time `json:"timestamp"`
			Value      float64   `json:"value"`
			SignalType string    `json:"signalType"`
			Units      string    `json:"units"`
		} `json:"engineRuntime"`
		FuelLevel struct {
			Timestamp time.Time `json:"timestamp"`
			Value     float64   `json:"value"`
			Units     string    `json:"units"`
		} `json:"fuelLevel"`
		CheckEngineLight struct {
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"checkEngineLight"`
		EngineOilLife struct {
			Timestamp time.Time `json:"timestamp"`
			Value     float64   `json:"value"`
			Units     string    `json:"units"`
		} `json:"engineOilLife"`
		BrakePadLife struct {
			Front struct {
				Value float64 `json:"value"`
				Units string  `json:"units"`
			} `json:"front"`
			Rear struct {
				Value float64 `json:"value"`
				Units string  `json:"units"`
			} `json:"rear"`
		} `json:"brakePadLife"`
		EngineAirFilterLife struct {
			Value float64 `json:"value"`
			Units string  `json:"units"`
		} `json:"engineAirFilterLife"`
		TirePressure struct {
			FrontLeft struct {
				Timestamp time.Time `json:"timestamp"`
				Value     float64   `json:"value"`
				Units     string    `json:"units"`
			} `json:"frontLeft"`
			FrontRight struct {
				Timestamp time.Time `json:"timestamp"`
				Value     float64   `json:"value"`
				Units     string    `json:"units"`
			} `json:"frontRight"`
			RearLeft struct {
				Timestamp time.Time `json:"timestamp"`
				Value     float64   `json:"value"`
				Units     string    `json:"units"`
			} `json:"rearLeft"`
			RearRight struct {
				Timestamp time.Time `json:"timestamp"`
				Value     float64   `json:"value"`
				Units     string    `json:"units"`
			} `json:"rearRight"`
		} `json:"tirePressure"`
		GearPosition struct {
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"gearPosition"`
		DeviceConnectivityStatus struct {
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"deviceConnectivityStatus"`
		EvBatteryRange struct {
			Timestamp time.Time `json:"timestamp"`
			Value     float64   `json:"value"`
			Units     string    `json:"units"`
		} `json:"evBatteryRange"`
		EvBatteryLevel struct {
			Timestamp time.Time `json:"timestamp"`
			Value     float64   `json:"value"`
			Units     string    `json:"units"`
		} `json:"evBatteryLevel"`
		EvChargingState struct {
			Timestamp time.Time `json:"timestamp"`
			Value     string    `json:"value"`
		} `json:"evChargingState"`
	} `json:"items"`
	Cursor string `json:"cursor"`
}
