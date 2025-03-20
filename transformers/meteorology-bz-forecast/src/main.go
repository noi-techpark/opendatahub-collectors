// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	"github.com/noi-techpark/go-timeseries-writer-client/bdplib"
)

// hard coded bz coordinates for main Station Dto location 46.49067, 11.33982
const (
	BZ_LAT = 46.49067
	BZ_LON = 11.33982

	PERIOD3H  = 10800
	PERIOD12H = 43200
	PERIOD24H = 86400

	OriginStationType = "WeatherForecast"
	DataStationType   = "WeatherForecastService"

	ForecastAirTemperatureMax        = "forecast-air-temperature-max"
	ForecastAirTemperatureMin        = "forecast-air-temperature-min"
	ForecastPrecipitationMax         = "forecast-precipitation-max"
	ForecastPrecipitationMin         = "forecast-precipitation-min"
	ForecastAirTemperature           = "forecast-air-temperature"
	ForecastWindDirection            = "forecast-wind-direction"
	ForecastWindSpeed                = "forecast-wind-speed"
	ForecastSunshineDuration         = "forecast-sunshine-duration"
	ForecastPrecipitationProbability = "forecast-precipitation-probability"
	QualitativeForecast              = "qualitative-forecast"
	ForecastPrecipitationSum         = "forecast-precipitation-sum"
)

type MunicipalityMap struct {
	municipalities []MunicipalityDto
}

func NewMunicipalityMap() *MunicipalityMap {
	// Open the JSON file
	file, err := os.Open("municipalities.json")
	ms.FailOnError(err, "cannot open municipalities.json")
	defer file.Close()

	// Read the file content
	byteValue, err := io.ReadAll(file)
	ms.FailOnError(err, "cannot read municipalities.json")

	// Unmarshal JSON into struct
	var mun []MunicipalityDto
	err = json.Unmarshal(byteValue, &mun)
	ms.FailOnError(err, "cannot unmarshal municipalities.json")

	return &MunicipalityMap{
		municipalities: mun,
	}
}

func (mm MunicipalityMap) GetLocation(de_name string) *LocationDto {
	for _, municipality := range mm.municipalities {
		if municipality.Name == de_name {
			return &LocationDto{Lat: municipality.Latitude, Lon: municipality.Longitude}
		}
	}
	return nil
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[Forecast] {
	return func(ctx context.Context, payload *dto.Raw[Forecast]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *dto.Raw[Forecast]) error {
	forecast := payload.Rawdata

	municipalities := NewMunicipalityMap()

	modelMetadata := bdplib.CreateStation(forecast.Info.Model,
		forecast.Info.Model, OriginStationType, BZ_LAT, BZ_LON, bdp.GetOrigin())

	modelMetadata.MetaData = map[string]interface{}{
		"currentModelRun": forecast.Info.CurrentModelRun,
		"nextModelRun":    forecast.Info.NextModelRun,
		"fileName":        forecast.Info.FileName,
	}

	runTimestamp, err := time.Parse(time.RFC3339, forecast.Info.CurrentModelRun)
	ms.FailOnError(err, "failed to parse time")

	dm := bdp.CreateDataMap()
	dm.AddRecord(forecast.Info.Model, ForecastAirTemperatureMax, bdplib.CreateRecord(runTimestamp.UnixMilli(), forecast.Info.AbsTempMax, PERIOD12H))
	dm.AddRecord(forecast.Info.Model, ForecastAirTemperatureMin, bdplib.CreateRecord(runTimestamp.UnixMilli(), forecast.Info.AbsTempMin, PERIOD12H))
	dm.AddRecord(forecast.Info.Model, ForecastPrecipitationMax, bdplib.CreateRecord(runTimestamp.UnixMilli(), forecast.Info.AbsPrecMax, PERIOD12H))
	dm.AddRecord(forecast.Info.Model, ForecastPrecipitationMin, bdplib.CreateRecord(runTimestamp.UnixMilli(), forecast.Info.AbsPrecMin, PERIOD12H))

	/// --------

	meas3h := bdp.CreateDataMap()
	meas24h := bdp.CreateDataMap()

	mun_stations := make([]bdplib.Station, 0)
	for _, mun := range forecast.Municipalities {
		loc := municipalities.GetLocation(mun.NameDe)
		if loc == nil {
			slog.Error("Location not found. Setting to default BZ location.", "municipality_name", mun.NameDe)
			loc = &LocationDto{Lat: BZ_LAT, Lon: BZ_LON}
		}

		mun_station := bdplib.CreateStation(mun.Code, fmt.Sprintf("%s_%s", mun.NameDe, mun.NameIt),
			DataStationType, loc.Lat, loc.Lon, bdp.GetOrigin())

		mun_station.MetaData = map[string]interface{}{
			"nameEn": mun.NameEn,
			"nameRm": mun.NameRm,
		}
		mun_station.ParentStation = modelMetadata.Id

		mun_stations = append(mun_stations, mun_station)

		// temperature min 24 hours
		addDoubleRecords(mun.Code, ForecastAirTemperatureMin, &meas24h, mun.TempMin24.Data, PERIOD24H)

		// temperature max 24 hours
		addDoubleRecords(mun.Code, ForecastAirTemperatureMax, &meas24h, mun.TempMax24.Data, PERIOD24H)

		// temperature every 3 hours
		addDoubleRecords(mun.Code, ForecastAirTemperature, &meas3h, mun.Temp3.Data, PERIOD3H)

		// sunshine duration 24 hours
		addDoubleRecords(mun.Code, ForecastSunshineDuration, &meas24h, mun.Ssd24.Data, PERIOD24H)

		// precipitation probability 3 hours
		addDoubleRecords(mun.Code, ForecastPrecipitationProbability, &meas3h, mun.PrecProb3.Data, PERIOD3H)

		// probably precipitation 24 hours
		addDoubleRecords(mun.Code, ForecastPrecipitationProbability, &meas24h, mun.PrecProb24.Data, PERIOD24H)

		// probably precipitation sum 3 hours
		addDoubleRecords(mun.Code, ForecastPrecipitationSum, &meas3h, mun.PrecSum3.Data, PERIOD3H)

		// probably precipitation sum 24 hours
		addDoubleRecords(mun.Code, ForecastPrecipitationSum, &meas24h, mun.PrecSum24.Data, PERIOD24H)

		// wind direction 3 hours
		addDoubleRecords(mun.Code, ForecastWindDirection, &meas3h, mun.WindDir3.Data, PERIOD3H)

		// wind speed 3 hours
		addDoubleRecords(mun.Code, ForecastWindSpeed, &meas3h, mun.WindSpd3.Data, PERIOD3H)

		// weather status symbols 3 hours
		addConvertedStringRecords(mun.Code, QualitativeForecast, &meas3h, mun.Symbols3.Data, PERIOD3H)

		// weather status symbols 24 hours
		addConvertedStringRecords(mun.Code, QualitativeForecast, &meas24h, mun.Symbols24.Data, PERIOD24H)
	}

	// -------
	bdp.SyncStations(OriginStationType, []bdplib.Station{modelMetadata}, true, false)
	bdp.PushData(OriginStationType, dm)

	bdp.SyncStations(DataStationType, mun_stations, true, false)
	bdp.PushData(DataStationType, meas3h)
	bdp.PushData(DataStationType, meas24h)
	return nil
}

func addDoubleRecords(stationId string, data_name string, dataMap *bdplib.DataMap, forecasts []ForecastDouble, period uint64) {
	for _, forecast := range forecasts {
		tm, err := time.Parse(time.RFC3339, forecast.Date)
		if err != nil {
			fmt.Println("Error parsing time:", err)
			return
		}

		dataMap.AddRecord(stationId, data_name,
			bdplib.CreateRecord(tm.UnixMilli(), forecast.Value, period),
		)
	}
}

func addConvertedStringRecords(stationId string, data_name string, dataMap *bdplib.DataMap, forecasts []ForecastString, period uint64) {
	for _, forecast := range forecasts {
		tm, err := time.Parse(time.RFC3339, forecast.Date)
		if err != nil {
			fmt.Println("Error parsing time:", err)
			return
		}

		dataMap.AddRecord(stationId, data_name,
			bdplib.CreateRecord(tm.UnixMilli(), mapQuantitativeValues(forecast.Value), period),
		)
	}
}

func mapQuantitativeValues(value string) string {
	mapping := map[string]string{
		"a_n": "sunny", "a_d": "sunny",
		"b_n": "partly cloudy", "b_d": "partly cloudy",
		"c_n": "cloudy", "c_d": "cloudy",
		"d_n": "very cloudy", "d_d": "very cloudy",
		"e_n": "overcast", "e_d": "overcast",
		"f_n": "cloudy with moderate rain", "f_d": "cloudy with moderate rain",
		"g_n": "cloudy with intense rain", "g_d": "cloudy with intense rain",
		"h_n": "overcast with moderate rain", "h_d": "overcast with moderate rain",
		"i_n": "overcast with intense rain", "i_d": "overcast with intense rain",
		"j_n": "overcast with light rain", "j_d": "overcast with light rain",
		"k_n": "translucent cloudy", "k_d": "translucent cloudy",
		"l_n": "cloudy with light snow", "l_d": "cloudy with light snow",
		"m_n": "cloudy with heavy snow", "m_d": "cloudy with heavy snow",
		"n_n": "overcast with light snow", "n_d": "overcast with light snow",
		"o_n": "overcast with moderate snow", "o_d": "overcast with moderate snow",
		"p_n": "overcast with intense snow", "p_d": "overcast with intense snow",
		"q_n": "cloudy with rain and snow", "q_d": "cloudy with rain and snow",
		"r_n": "overcast with rain and snow", "r_d": "overcast with rain and snow",
		"s_n": "low cloudiness", "s_d": "low cloudiness",
		"t_n": "fog", "t_d": "fog",
		"u_n": "cloudy, thunderstorms with moderate showers", "u_d": "cloudy, thunderstorms with moderate showers",
		"v_n": "cloudy, thunderstorms with intense showers", "v_d": "cloudy, thunderstorms with intense showers",
		"w_n": "cloudy, thunderstorms with moderate snowy and rainy showers", "w_d": "cloudy, thunderstorms with moderate snowy and rainy showers",
		"x_n": "cloudy, thunderstorms with intense snowy and rainy showers", "x_d": "cloudy, thunderstorms with intense snowy and rainy showers",
		"y_n": "cloudy, thunderstorms with moderate snowy showers", "y_d": "cloudy, thunderstorms with moderate snowy showers",
	}

	if description, exists := mapping[value]; exists {
		return description
	}

	slog.Error("No mapping configured for value", "value", value)
	return ""
}

var env tr.Env

func main() {
	envconfig.MustProcess("", &env)
	b := bdplib.FromEnv()

	// The old data collector was trying to enrich the type with the localization of each type in the Metadata.
	// Unfortunately the received JSON has different localizations for the same DataType but different period:
	// precProb24: maximum precipitation probability
	// precProb3: precipitation probability
	// Therefore we aribtrary chose one
	dataTypeList := bdplib.NewDataTypeList(nil)
	err := dataTypeList.Load("datatypes.json")
	ms.FailOnError(err, "could not load datatypes")

	slog.Info("pushing datatypes on startup")
	b.SyncDataTypes(OriginStationType, dataTypeList.All())

	slog.Info("listening")
	listener := tr.NewTrStack[Forecast](&env)
	err = listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(err, "error while listening to queue")
}
