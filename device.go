package digitalpaper

import (
	"context"
	"encoding/json"
	"net/http"
)

// DeviceService exposes read-only device status operations.
type DeviceService struct{ client *Client }

// FirmwareStatus identifies the connected device and firmware.
type FirmwareStatus struct {
	Version string `json:"value"`
	Model   string `json:"model_name"`
}

// BatteryStatus contains the string-valued status fields used by the API.
type BatteryStatus struct {
	Level    string `json:"level"`
	IconType string `json:"icon_type"`
	Status   string `json:"status"`
	Health   string `json:"health"`
	Plugged  string `json:"plugged"`
	Pen      string `json:"pen"`
}

// UnmarshalJSON accepts both the actual status key and the known translated
// Swagger typo, "Settings - status".
func (status *BatteryStatus) UnmarshalJSON(data []byte) error {
	type batteryAlias BatteryStatus
	var wire struct {
		batteryAlias
		TranslatedStatus string `json:"Settings - status"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*status = BatteryStatus(wire.batteryAlias)
	if status.Status == "" {
		status.Status = wire.TranslatedStatus
	}
	return nil
}

// StorageStatus contains byte counts as strings, matching the device API.
type StorageStatus struct {
	Capacity  string `json:"capacity"`
	Available string `json:"available"`
}

func (s *DeviceService) Firmware(ctx context.Context) (FirmwareStatus, error) {
	var result FirmwareStatus
	err := s.client.wire.DoJSON(ctx, http.MethodGet, "/system/status/firmware_version", nil, nil, &result, true)
	return result, publicError(err)
}

func (s *DeviceService) Battery(ctx context.Context) (BatteryStatus, error) {
	var result BatteryStatus
	err := s.client.wire.DoJSON(ctx, http.MethodGet, "/system/status/battery", nil, nil, &result, true)
	return result, publicError(err)
}

func (s *DeviceService) Storage(ctx context.Context) (StorageStatus, error) {
	var result StorageStatus
	err := s.client.wire.DoJSON(ctx, http.MethodGet, "/system/status/storage", nil, nil, &result, true)
	return result, publicError(err)
}
