package main

import (
	"encoding/json"
	"testing"
)

func TestQuantile(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50}

	tests := []struct {
		probability float64
		expected    float64
	}{
		{probability: 0, expected: 10},
		{probability: 0.1, expected: 14},
		{probability: 0.5, expected: 30},
		{probability: 0.9, expected: 46},
		{probability: 1, expected: 50},
	}

	for _, test := range tests {
		actual := quantile(values, test.probability)
		if actual != test.expected {
			t.Fatalf(
				"quantile(%v, %v) = %v, expected %v",
				values,
				test.probability,
				actual,
				test.expected,
			)
		}
	}
}

func TestDecodeMemberSeries(t *testing.T) {
	daily := map[string]json.RawMessage{
		"time":                         json.RawMessage(`["2026-07-24"]`),
		"temperature_2m_mean":          json.RawMessage(`[20]`),
		"temperature_2m_mean_member01": json.RawMessage(`[18]`),
		"temperature_2m_mean_member02": json.RawMessage(`[22]`),
		"precipitation_sum":            json.RawMessage(`[1]`),
	}

	series, err := decodeMemberSeries(daily, "temperature_2m_mean")
	if err != nil {
		t.Fatalf("decodeMemberSeries returned error: %v", err)
	}

	values := memberValuesAt(series, 0)
	if len(values) != 3 {
		t.Fatalf("member count = %d, expected 3", len(values))
	}
	if median(values) != 20 {
		t.Fatalf("member median = %v, expected 20", median(values))
	}
}

func TestNormalizeOutlookModelSkipsUnavailableDays(t *testing.T) {
	daily := map[string]json.RawMessage{
		"temperature_2m_min_test":            json.RawMessage(`[10,11,null]`),
		"temperature_2m_max_test":            json.RawMessage(`[20,21,null]`),
		"precipitation_probability_max_test": json.RawMessage(`[30,40,null]`),
		"precipitation_sum_test":             json.RawMessage(`[0,1,null]`),
	}

	model, err := normalizeOutlookModel(
		daily,
		[]string{"2026-07-24", "2026-07-25", "2026-07-26"},
		outlookModelDefinition{
			id:     "test",
			name:   "Test",
			short:  "TEST",
			color:  "#fff",
			suffix: "test",
		},
	)
	if err != nil {
		t.Fatalf("normalizeOutlookModel returned error: %v", err)
	}

	if model.HorizonDays != 2 {
		t.Fatalf("horizon = %d, expected 2", model.HorizonDays)
	}
	if model.Daily[1].TemperatureMax != 21 {
		t.Fatalf("second maximum = %v, expected 21", model.Daily[1].TemperatureMax)
	}
}
