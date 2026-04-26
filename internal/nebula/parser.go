package nebula

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vooon/nebula-mnemosina/internal/model"
)

func ParseVersion(output string) string {
	return strings.TrimSpace(output)
}

func ParseDeviceInfo(output string) (model.DeviceInfo, error) {
	var info model.DeviceInfo
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return model.DeviceInfo{}, fmt.Errorf("parse device-info: %w", err)
	}
	return info, nil
}

func ParseHostmap(output string) ([]model.HostmapEntry, error) {
	var entries []model.HostmapEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return nil, fmt.Errorf("parse hostmap: %w", err)
	}
	rawEntries := make([]json.RawMessage, 0, len(entries))
	if err := json.Unmarshal([]byte(output), &rawEntries); err != nil {
		return nil, fmt.Errorf("split hostmap raw records: %w", err)
	}
	for i := range entries {
		if i < len(rawEntries) {
			entries[i].Raw = append([]byte(nil), rawEntries[i]...)
		}
	}
	return entries, nil
}

func ParseLighthouseAddrmap(output string) ([]model.LighthouseAddrmapEntry, error) {
	var entries []model.LighthouseAddrmapEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return nil, fmt.Errorf("parse lighthouse addrmap: %w", err)
	}

	rawEntries := make([]json.RawMessage, 0, len(entries))
	if err := json.Unmarshal([]byte(output), &rawEntries); err != nil {
		return nil, fmt.Errorf("split lighthouse addrmap raw records: %w", err)
	}
	for i := range entries {
		if i < len(rawEntries) {
			entries[i].Raw = append([]byte(nil), rawEntries[i]...)
		}
	}
	return entries, nil
}

func ParseRelays(output string) ([]byte, error) {
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("parse relays: %w", err)
	}
	return append([]byte(nil), raw...), nil
}
