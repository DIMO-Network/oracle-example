package convert

import (
	"encoding/json"
	"github.com/DIMO-Network/oracle-example/internal/config"
	dbmodels "github.com/DIMO-Network/oracle-example/internal/db/models"
	"github.com/ethereum/go-ethereum/common"
	"github.com/volatiletech/null/v8"
	"testing"
	"time"

	"github.com/DIMO-Network/model-garage/pkg/defaultmodule"
	"github.com/DIMO-Network/oracle-example/internal/models"
	"github.com/stretchr/testify/assert"
)

var unbufferedMsg = `{
		"id": "01965837-7540-71fb-acc4-60264bfd17b4",
		"dataType": "telemetry",
		"vehicleId": "ffbf0b52-d478-4320-9a1c-3b83f547f33b",
		"deviceId": null,
		"timestamp": "2025-04-21T11:58:00.619Z",
		"data": {
			"location": {
				"lat": 36.5810399,
				"lon": -79.43646179999999
			},
			"speed": {
				"value": 0,
				"signalType": "canBus",
				"units": "mph"
			},
			"odometer": {
				"value": 27071.65,
				"signalType": "canBus",
				"units": "mi"
			},
			"fuelLevel": {
				"value": 79,
				"signalType": "canBus",
				"units": "pct"
			}
		}
	}`

var unbufferedMsgLocationIsZero = `{
		"id": "01965837-7540-71fb-acc4-60264bfd17b4",
		"dataType": "telemetry",
		"vehicleId": "ffbf0b52-d478-4320-9a1c-3b83f547f33b",
		"deviceId": null,
		"timestamp": "2025-04-21T11:58:00.619Z",
		"data": {
			"location": {
				"lat": 0,
				"lon": 0
			},
			"speed": {
				"value": 0,
				"signalType": "canBus",
				"units": "mph"
			},
			"odometer": {
				"value": 27071.65,
				"signalType": "canBus",
				"units": "mi"
			}
		}
	}`

var unbufferedMsgWithNoLoc = `{
		"id": "01965837-7540-71fb-acc4-60264bfd17b4",
		"dataType": "telemetry",
		"vehicleId": "ffbf0b52-d478-4320-9a1c-3b83f547f33b",
		"deviceId": null,
		"timestamp": "2025-04-21T11:58:00.619Z",
		"data": {
			"speed": {
				"value": 0,
				"signalType": "canBus",
				"units": "mph"
			},
			"odometer": {
				"value": 27071.65,
				"signalType": "canBus",
				"units": "mi"
			}
		}
	}`

func TestConvertToCloudEvent(t *testing.T) {
	// given
	veh := dbmodels.Vin{
		Vin:              "1HGCM82633A123456",
		VehicleTokenID:   null.Int64From(123456),
		SyntheticTokenID: null.Int64From(789012),
	}

	var telemetry models.UnbufferedMessageValue
	_ = json.Unmarshal([]byte(unbufferedMsg), &telemetry)
	settings := config.Settings{ChainID: 1, SyntheticNftAddress: common.HexToAddress("0x71C7656EC7ab88b098defB751B7401B5f6d8976F"), VehicleNftAddress: common.HexToAddress("0x51C7656EC7ab88b098defB751B7401B5f6d8976G")}

	// then
	ce, err := ToCloudEvent(veh, telemetry, settings)

	// verify
	assert.NoError(t, err)
	assert.NotNil(t, ce)

	// Validate the wrapped data
	var wrappedData struct {
		Signals []*defaultmodule.Signal `json:"signals"`
		Vin     string                  `json:"vin"`
	}
	err = json.Unmarshal(ce.Data, &wrappedData)
	assert.NoError(t, err)
	assert.Len(t, wrappedData.Signals, 5)
	assert.Equal(t, "1HGCM82633A123456", wrappedData.Vin)
}

func TestMapSignals(t *testing.T) {
	// given
	var telemetry models.UnbufferedMessageValue
	_ = json.Unmarshal([]byte(unbufferedMsg), &telemetry)
	ts := time.Now()

	// then
	signals, err := mapDataToSignals(telemetry.Data, ts)

	// verify
	assert.NoError(t, err)
	assert.Len(t, signals, 5)

	// Validate individual signals
	assert.Equal(t, "speed", signals[0].Name)
	assert.Equal(t, float64(0), signals[0].Value)
	assert.Equal(t, ts, signals[0].Timestamp)

	assert.Equal(t, "fuelLevel", signals[1].Name)
	assert.Equal(t, float64(79), signals[1].Value)
	assert.Equal(t, ts, signals[1].Timestamp)

	assert.Equal(t, "powertrainTransmissionTravelledDistance", signals[2].Name)
	assert.Equal(t, 43567.59749760001, signals[2].Value)
	assert.Equal(t, ts, signals[2].Timestamp)

	assert.Equal(t, "currentLocationLongitude", signals[3].Name)
	assert.Equal(t, -79.43646179999999, signals[3].Value)
	assert.Equal(t, ts, signals[3].Timestamp)

	assert.Equal(t, "currentLocationLatitude", signals[4].Name)
	assert.Equal(t, 36.5810399, signals[4].Value)
	assert.Equal(t, ts, signals[4].Timestamp)
}

func TestMapSignalsNoLocation(t *testing.T) {
	// given
	var telemetry models.UnbufferedMessageValue
	_ = json.Unmarshal([]byte(unbufferedMsgWithNoLoc), &telemetry)
	ts := time.Now()

	// then
	signals, err := mapDataToSignals(telemetry.Data, ts)

	// verify
	assert.NoError(t, err)
	assert.Len(t, signals, 2)

	// Validate individual signals
	assert.Equal(t, "speed", signals[0].Name)
	assert.Equal(t, float64(0), signals[0].Value)
	assert.Equal(t, ts, signals[0].Timestamp)

	assert.Equal(t, "powertrainTransmissionTravelledDistance", signals[1].Name)
	assert.Equal(t, 43567.59749760001, signals[1].Value)
	assert.Equal(t, ts, signals[1].Timestamp)
}

func TestMapSignalsLocationIsZero(t *testing.T) {
	// given
	var telemetry models.UnbufferedMessageValue
	_ = json.Unmarshal([]byte(unbufferedMsgLocationIsZero), &telemetry)
	ts := time.Now()

	// then
	signals, err := mapDataToSignals(telemetry.Data, ts)

	// verify
	assert.NoError(t, err)
	assert.Len(t, signals, 2)

	// Validate individual signals
	assert.Equal(t, "speed", signals[0].Name)
	assert.Equal(t, float64(0), signals[0].Value)
	assert.Equal(t, ts, signals[0].Timestamp)

	assert.Equal(t, "powertrainTransmissionTravelledDistance", signals[1].Name)
	assert.Equal(t, 43567.59749760001, signals[1].Value)
	assert.Equal(t, ts, signals[1].Timestamp)
}
