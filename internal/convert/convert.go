package convert

import (
	"github.com/DIMO-Network/model-garage/pkg/defaultmodule"
	"github.com/DIMO-Network/oracle-example/internal/models"
	"time"
)

// MapDataToSignals maps the data from the message to the default DIS signals
func MapDataToSignals(data models.Data, ts time.Time) ([]*defaultmodule.Signal, error) {
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
