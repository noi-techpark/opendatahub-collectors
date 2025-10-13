// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"

	_ "time/tzdata"
)

var env tr.Env

const (
	PERIOD3H  = 10800
	PERIOD24H = 86400

	DataStationType = "WeatherForecast"
)

var (
	// Data types corresponding to fields in the JSON response.
	ForecastAirTemperatureMax        = bdplib.CreateDataType("forecast-air-temperature-max", "°C", "Maximum air temperature", "instant")
	ForecastAirTemperatureMin        = bdplib.CreateDataType("forecast-air-temperature-min", "°C", "Minimum air temperature", "instant")
	ForecastAirTemperature           = bdplib.CreateDataType("forecast-air-temperature", "°C", "Current air temperature", "instant")
	ForecastWindDirection            = bdplib.CreateDataType("forecast-wind-direction", "°", "Wind direction", "instant")
	ForecastWindSpeed                = bdplib.CreateDataType("forecast-wind-speed", "m/s", "Wind speed", "instant")
	ForecastSunshineDuration         = bdplib.CreateDataType("forecast-sunshine-duration", "hours", "Sunshine duration", "instant")
	ForecastPrecipitationProbability = bdplib.CreateDataType("forecast-precipitation-probability", "%", "Precipitation probability", "instant")
	QualitativeForecast              = bdplib.CreateDataType("qualitative-forecast", "", "Qualitative sky condition", "instant")
	ForecastPrecipitationSum         = bdplib.CreateDataType("forecast-precipitation-sum", "mm", "Total precipitation sum", "sum")
	ForecastWindGust                 = bdplib.CreateDataType("forecast-wind-gust", "m/s", "Maximum wind gust", "instant")
	ForecastFreshSnow                = bdplib.CreateDataType("forecast-fresh-snow", "mm", "Fresh snow depth", "instant")
	ForecastSnowLevel                = bdplib.CreateDataType("forecast-snow-level", "m", "Snow level", "instant")
	ForecastFreezingLevel            = bdplib.CreateDataType("forecast-freezing-level", "m", "Freezing level", "instant")
)

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[ForecastData] {
	return func(ctx context.Context, payload *rdb.Raw[ForecastData]) error {
		return Transform(ctx, bdp, payload)
	}
}

var stationsMap Stations
var err error

func createStationsFromLookup(bdp bdplib.Bdp, lookup Stations, origin string) []bdplib.Station {
	stations := make([]bdplib.Station, 0, len(lookup))

	for _, stationData := range lookup {
		stations = append(stations, stationData.ToBdp(bdp))
	}

	return stations
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()

	stationsMap, err = LoadAllStations()
	ms.FailOnError(context.Background(), err, "failed to load stations")

	// Call the function with all the data types.
	err = b.SyncDataTypes([]bdplib.DataType{
		ForecastAirTemperatureMax,
		ForecastAirTemperatureMin,
		ForecastAirTemperature,
		ForecastWindDirection,
		ForecastWindSpeed,
		ForecastSunshineDuration,
		ForecastPrecipitationProbability,
		QualitativeForecast,
		ForecastPrecipitationSum,
		ForecastWindGust,
		ForecastFreshSnow,
		ForecastSnowLevel,
		ForecastFreezingLevel,
	})
	ms.FailOnError(context.Background(), err, "failed to sync datatypes")

	// station sync
	stations := createStationsFromLookup(b, stationsMap, b.GetOrigin())
	err = b.SyncStations(DataStationType, stations, true, false)
	ms.FailOnError(context.Background(), err, "failed to sync stations")

	listener := tr.NewTr[string](context.Background(), env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, bdp bdplib.Bdp, data *rdb.Raw[ForecastData]) error {
	forecast := data.Rawdata.Forecasts
	s := stationsMap.GetStationByID(data.Rawdata.Id)
	if nil == s {
		slog.Error("station not found", "id", data.Rawdata.Id)
		return fmt.Errorf("station not found")
	}
	bdpStation := s.ToBdp(bdp)

	// Load the location for handling CET/CEST.
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return fmt.Errorf("failed to load location Europe/Berlin: %w", err)
	}

	// Parse the base forecast start time using the specified location.
	baseStart, err := time.ParseInLocation("2006-01-02T15:04:05", forecast.Start, loc)
	if err != nil {
		return fmt.Errorf("failed to parse base start time: %w", err)
	}

	// Create DataMaps for different forecast periods.
	meas180 := bdp.CreateDataMap()
	meas1440 := bdp.CreateDataMap()

	// Process HourlyData (180 minutes)
	slog.Info("Processing hourly data (180 minutes)...", "station_id", bdpStation.Id)
	for key, hourlyData := range forecast.HourlyData {

		forecastStart, _, err := calculateForecastTimes(baseStart, key)
		if err != nil {
			slog.Error("Failed to calculate forecast times for key", "key", key, "error", err, "station_id", data.Rawdata.Id)
			return err
		}
		timestampMilli := forecastStart.UnixMilli()

		meas180.AddRecord(bdpStation.Id, ForecastWindGust.Name, bdplib.CreateRecord(timestampMilli, hourlyData.WindGust, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastFreshSnow.Name, bdplib.CreateRecord(timestampMilli, hourlyData.FreshSnow, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastSnowLevel.Name, bdplib.CreateRecord(timestampMilli, hourlyData.SnowLevel, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastWindSpeed.Name, bdplib.CreateRecord(timestampMilli, hourlyData.WindSpeed, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastAirTemperature.Name, bdplib.CreateRecord(timestampMilli, hourlyData.Temperature, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, QualitativeForecast.Name, bdplib.CreateRecord(timestampMilli, hourlyData.SkyCondition, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastFreezingLevel.Name, bdplib.CreateRecord(timestampMilli, hourlyData.FreezingLevel, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastWindDirection.Name, bdplib.CreateRecord(timestampMilli, hourlyData.WindDirection, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastPrecipitationProbability.Name, bdplib.CreateRecord(timestampMilli, hourlyData.RainProbability, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastSunshineDuration.Name, bdplib.CreateRecord(timestampMilli, hourlyData.SunshineDuration, PERIOD3H))
		meas180.AddRecord(bdpStation.Id, ForecastPrecipitationSum.Name, bdplib.CreateRecord(timestampMilli, hourlyData.RainFall, PERIOD3H))
	}

	// Process DailyData (1440 minutes) - only for tomorrow.
	slog.Info("Processing daily data for 'tomorrow' (1440 minutes)...", "station_id", bdpStation.Id)
	tomorrow := baseStart.Truncate(24 * time.Hour).Add(24 * time.Hour)
	for key, dailyData := range forecast.DailyData {
		forecastStart, _, err := calculateForecastTimes(baseStart, key)
		if err != nil {
			slog.Error("Failed to calculate forecast times", "key", key, "error", err, "station_id", data.Rawdata.Id)
			return err
		}

		// Check if forecastStart is the exact start of 'tomorrow'.
		if !forecastStart.Truncate(24 * time.Hour).Equal(tomorrow) {
			// slog.Info("Skipping entry as it's not for tomorrow", "key", key, "time", forecastStart)
			continue
		}

		timestampMilli := forecastStart.UnixMilli()

		meas1440.AddRecord(bdpStation.Id, ForecastWindGust.Name, bdplib.CreateRecord(timestampMilli, dailyData.WindGust, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastFreshSnow.Name, bdplib.CreateRecord(timestampMilli, dailyData.FreshSnow, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastSnowLevel.Name, bdplib.CreateRecord(timestampMilli, dailyData.SnowLevel, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastWindSpeed.Name, bdplib.CreateRecord(timestampMilli, dailyData.WindSpeed, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, QualitativeForecast.Name, bdplib.CreateRecord(timestampMilli, dailyData.SkyCondition, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastFreezingLevel.Name, bdplib.CreateRecord(timestampMilli, dailyData.FreezingLevel, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastWindDirection.Name, bdplib.CreateRecord(timestampMilli, dailyData.WindDirection, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastPrecipitationProbability.Name, bdplib.CreateRecord(timestampMilli, dailyData.RainProbability, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastSunshineDuration.Name, bdplib.CreateRecord(timestampMilli, dailyData.SunshineDuration, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastAirTemperatureMax.Name, bdplib.CreateRecord(timestampMilli, dailyData.TemperatureMaximum, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastAirTemperatureMin.Name, bdplib.CreateRecord(timestampMilli, dailyData.TemperatureMinimum, PERIOD24H))
		meas1440.AddRecord(bdpStation.Id, ForecastPrecipitationSum.Name, bdplib.CreateRecord(timestampMilli, dailyData.RainFall, PERIOD24H))
	}

	// Push the created data maps to the mocked BDPLib
	bdp.PushData(DataStationType, meas180)
	bdp.PushData(DataStationType, meas1440)

	return nil
}
