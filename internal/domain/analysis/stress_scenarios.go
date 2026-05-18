package analysis

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed stress_scenarios.json
var stressScenariosFS embed.FS

// StressScenario describes a historical crisis scenario with estimated
// peak-to-trough sector returns (GICS sectors).
type StressScenario struct {
	Name          string             `json:"name"`
	DateRange     string             `json:"date_range"`
	SectorReturns map[string]float64 `json:"sector_returns"` // sector → peak-to-trough return %
}

// PredefinedScenarios holds the researched historical crisis scenarios.
// Populated at init time from an embedded JSON file.
var PredefinedScenarios []StressScenario

func init() {
	scenarios, err := LoadPredefinedScenarios()
	if err != nil {
		panic(fmt.Sprintf("analysis: failed to load stress scenarios: %v", err))
	}
	PredefinedScenarios = scenarios
}

// LoadPredefinedScenarios reads and validates the embedded stress scenario
// data file. Returns an error if the file is missing or contains invalid data.
func LoadPredefinedScenarios() ([]StressScenario, error) {
	data, err := stressScenariosFS.ReadFile("stress_scenarios.json")
	if err != nil {
		return nil, fmt.Errorf("read stress scenarios: %w", err)
	}

	var scenarios []StressScenario
	if err := json.Unmarshal(data, &scenarios); err != nil {
		return nil, fmt.Errorf("parse stress scenarios: %w", err)
	}

	// Validate structure.
	for i, s := range scenarios {
		if s.Name == "" {
			return nil, fmt.Errorf("scenario %d: missing name", i)
		}
		if s.DateRange == "" {
			return nil, fmt.Errorf("scenario %d (%q): missing date_range", i, s.Name)
		}
		if len(s.SectorReturns) == 0 {
			return nil, fmt.Errorf("scenario %d (%q): no sector returns", i, s.Name)
		}
	}

	return scenarios, nil
}
