// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ForecastData struct {
	Id        string           `json:"id"`
	Forecasts WeatherForecasts `json:"forecasts"`
}

// WeatherForecasts represents the top-level structure of the JSON object.
type WeatherForecasts struct {
	// used for data bulletins that carry observations
	// HalfHourData map[string]*DailyData `json:"30"`

	// used for data bulletins that carry forecasts
	HourlyData map[string]*HourlyData `json:"180"`

	// used for textual bulletins
	DailyData map[string]*DailyData `json:"1440"`

	// used for mountain weather textual bulletins
	// ThreeDaysData map[string]*DailyData `json:"4320"`

	// Start and End are the date and time ranges for the forecast.
	Start string `json:"start"`
	End   string `json:"end"`
}

// HourlyData defines the structure for the weather data found under the "180" key.
type HourlyData struct {
	RainFall         float64 `json:"rain_fall"`
	WindGust         float64 `json:"wind_gust"`
	FreshSnow        float64 `json:"fresh_snow"`
	SnowLevel        int     `json:"snow_level"`
	WindSpeed        float64 `json:"wind_speed"`
	Temperature      int     `json:"temperature"`
	SkyCondition     string  `json:"sky_condition"`
	FreezingLevel    int     `json:"freezing_level"`
	WindDirection    int     `json:"wind_direction"`
	RainProbability  int     `json:"rain_probability"`
	SunshineDuration float64 `json:"sunshine_duration"`
}

// DailyData defines the structure for the weather data found under the "1440" key.
// It includes all the fields from HourlyData plus temperature_maximum and temperature_minimum.
type DailyData struct {
	RainFall           float64 `json:"rain_fall"`
	WindGust           float64 `json:"wind_gust"`
	FreshSnow          float64 `json:"fresh_snow"`
	SnowLevel          int     `json:"snow_level"`
	WindSpeed          float64 `json:"wind_speed"`
	SkyCondition       string  `json:"sky_condition"`
	FreezingLevel      int     `json:"freezing_level"`
	WindDirection      int     `json:"wind_direction"`
	RainProbability    int     `json:"rain_probability"`
	SunshineDuration   float64 `json:"sunshine_duration"`
	TemperatureMaximum int     `json:"temperature_maximum"`
	TemperatureMinimum int     `json:"temperature_minimum"`
}

// calculateForecastTimes parses a key string and a base UTC start time
// to determine the specific start and end times for a forecast period.
// The key format is based on `forecastType * 100000 + offset_in_minutes`.
func calculateForecastTimes(baseStart time.Time, key string) (time.Time, time.Time, error) {
	// The key is a string representing an integer. We first convert it to a number.
	keyInt, err := strconv.Atoi(key)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid key format: %w", err)
	}

	// Determine the forecast type (180 for hourly, 1440 for daily)
	// by checking the prefix of the key string.
	var forecastType int
	if strings.HasPrefix(key, "180") {
		forecastType = 180
	} else if strings.HasPrefix(key, "1440") {
		forecastType = 1440
	} else {
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported forecast type in key: %s", key)
	}

	// Calculate the offset in minutes from the key.
	// The offset is the remainder after dividing by 100,000, as per the
	// pattern `type * 100000 + offset`.
	offsetMinutes := keyInt % 100000

	// Calculate the specific start time for this forecast period.
	forecastStart := baseStart.Add(time.Minute * time.Duration(offsetMinutes))

	// Calculate the end time by adding the forecast type (duration) to the start time.
	// This represents the length of the forecast period (e.g., 180 minutes or 1440 minutes).
	forecastEnd := forecastStart.Add(time.Minute * time.Duration(forecastType))

	return forecastStart, forecastEnd, nil
}
