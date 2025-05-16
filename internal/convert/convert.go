package convert

import (
	"encoding/json"
	"github.com/DIMO-Network/cloudevent"
	"github.com/DIMO-Network/model-garage/pkg/defaultmodule"
	"github.com/DIMO-Network/oracle-example/internal/config"
	dbmodels "github.com/DIMO-Network/oracle-example/internal/db/models"
	"github.com/DIMO-Network/oracle-example/internal/models"
	"github.com/segmentio/ksuid"
	"time"
)

// ToCloudEvent Convert the external vendor msg payload to a CloudEvent
func ToCloudEvent(veh dbmodels.Vin, msg models.UnbufferedMessageValue, settings config.Settings) (*cloudevent.CloudEvent[json.RawMessage], error) {
	// Construct the producer DID
	producer := cloudevent.NFTDID{
		ChainID:         uint64(settings.ChainID),
		ContractAddress: settings.SyntheticNftAddress,
		TokenID:         uint32(veh.SyntheticTokenID.Int64),
	}.String()

	// Construct the subject
	var subject string
	vehTokenId := uint32(veh.VehicleTokenID.Int64)
	if vehTokenId != 0 {
		subject = cloudevent.NFTDID{
			ChainID:         uint64(settings.ChainID),
			ContractAddress: settings.VehicleNftAddress,
			TokenID:         vehTokenId,
		}.String()
	}

	ch, err := createCloudEventHeader(msg.Timestamp, producer, subject, cloudevent.TypeStatus)
	if err != nil {
		return nil, err
	}

	// transform the data to default DIS format
	signals, err := mapDataToSignals(msg.Data, msg.Timestamp)

	if err != nil {
		return nil, err
	}

	// Wrap the signals into a struct
	wrappedData := struct {
		Signals []*defaultmodule.Signal `json:"signals"`
		Vin     string                  `json:"vin"`
	}{
		Signals: signals,
		Vin:     veh.Vin,
	}

	// Marshal the wrapped data to json.RawMessage
	data, err := json.Marshal(wrappedData)
	if err != nil {
		return nil, err
	}

	// Create the CloudEvent
	ce := &cloudevent.CloudEvent[json.RawMessage]{
		CloudEventHeader: ch,
		Data:             data,
	}

	return ce, nil
}

// mapDataToSignals maps the data from the message to the default DIS signals
func mapDataToSignals(data models.Data, ts time.Time) ([]*defaultmodule.Signal, error) {
	var signals []*defaultmodule.Signal

	sigMap, err := defaultmodule.LoadSignalMap()
	if err != nil {
		return nil, err
	}

	if _, exists := sigMap["speed"]; exists {
		speedSignal := &defaultmodule.Signal{
			Name:      "speed",
			Timestamp: ts,
			Value:     data.Speed.Value,
		}
		signals = append(signals, speedSignal)
	}

	if _, exists := sigMap["powertrainFuelSystemRelativeLevel"]; exists && data.FuelLevel.Value > 0 && data.FuelLevel.Value <= 100 {
		fuelSignal := &defaultmodule.Signal{
			Name:      "powertrainFuelSystemRelativeLevel",
			Timestamp: ts,
			Value:     data.FuelLevel.Value,
		}
		signals = append(signals, fuelSignal)
	}

	if _, exists := sigMap["powertrainTransmissionTravelledDistance"]; exists {
		odometerSignal := &defaultmodule.Signal{
			Name:      "powertrainTransmissionTravelledDistance",
			Timestamp: ts,
			Value:     milesToKilometers(data.Odometer.Value),
		}
		signals = append(signals, odometerSignal)
	}

	if _, exists := sigMap["currentLocationLongitude"]; exists && data.Location.Lon != 0 {
		odometerSignal := &defaultmodule.Signal{
			Name:      "currentLocationLongitude",
			Timestamp: ts,
			Value:     data.Location.Lon,
		}
		signals = append(signals, odometerSignal)
	}

	if _, exists := sigMap["currentLocationLatitude"]; exists && data.Location.Lat != 0 {
		odometerSignal := &defaultmodule.Signal{
			Name:      "currentLocationLatitude",
			Timestamp: ts,
			Value:     data.Location.Lat,
		}
		signals = append(signals, odometerSignal)
	}

	return signals, nil
}

// createCloudEvent creates a cloud event from autopi event.
func createCloudEventHeader(ts time.Time, producer, subject, eventType string) (cloudevent.CloudEventHeader, error) {
	return cloudevent.CloudEventHeader{
		DataContentType: "application/json",
		ID:              ksuid.New().String(),
		Subject:         subject,
		SpecVersion:     "1.0",
		Time:            ts,
		Type:            eventType,
		DataVersion:     "default/v1.0",
		Producer:        producer,
	}, nil
}
