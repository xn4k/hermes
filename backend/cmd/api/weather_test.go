package main

import (
	"testing"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{name: "empty", values: nil, expected: 0},
		{name: "odd", values: []float64{4, 1, 3}, expected: 3},
		{name: "even", values: []float64{4, 2, 8, 6}, expected: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := median(test.values); actual != test.expected {
				t.Fatalf("median(%v) = %v, expected %v", test.values, actual, test.expected)
			}
		})
	}
}

func TestBuildWeatherConsensus(t *testing.T) {
	forecast := WeatherForecast{
		Current: WeatherCurrent{Time: "2026-07-24T12:00"},
		Models: []WeatherModelForecast{
			{
				Daily: []WeatherDailyPoint{
					{TemperatureMax: 29, ApparentTemperatureMax: 31},
					{TemperatureMax: 31, ApparentTemperatureMax: 32},
				},
				Hourly: []WeatherHourlyPoint{
					{Time: "2026-07-24T12:00", PrecipitationProbability: 20},
					{Time: "2026-07-24T13:00", PrecipitationProbability: 60},
				},
			},
			{
				Daily: []WeatherDailyPoint{
					{TemperatureMax: 30, ApparentTemperatureMax: 30},
					{TemperatureMax: 32, ApparentTemperatureMax: 33},
				},
				Hourly: []WeatherHourlyPoint{
					{Time: "2026-07-24T12:00", PrecipitationProbability: 10},
					{Time: "2026-07-24T13:00", PrecipitationProbability: 70},
				},
			},
			{
				Daily: []WeatherDailyPoint{
					{TemperatureMax: 31, ApparentTemperatureMax: 29},
					{TemperatureMax: 33, ApparentTemperatureMax: 34},
				},
				Hourly: []WeatherHourlyPoint{
					{Time: "2026-07-24T12:00", PrecipitationProbability: 15},
					{Time: "2026-07-24T13:00", PrecipitationProbability: 30},
				},
			},
		},
	}

	consensus := buildWeatherConsensus(forecast)

	if consensus.TodayMaxMedian != 30 {
		t.Fatalf("today median = %v, expected 30", consensus.TodayMaxMedian)
	}
	if consensus.Confidence != "high" {
		t.Fatalf("confidence = %q, expected high", consensus.Confidence)
	}
	if consensus.RainAgreementNext6 != 2 {
		t.Fatalf("rain agreement = %d, expected 2", consensus.RainAgreementNext6)
	}
	if consensus.RainStart != "2026-07-24T13:00" {
		t.Fatalf("rain start = %q, expected 13:00", consensus.RainStart)
	}
	if consensus.HeatLevel != "warning" {
		t.Fatalf("heat level = %q, expected warning", consensus.HeatLevel)
	}
}
