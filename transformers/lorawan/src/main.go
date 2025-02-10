// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"slices"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
)

const Station = "IndoorStation"
const Period = 120
const Origin = "NOI"

var env struct {
	tr.Env
}

type Coordinates struct {
	Lat  float64
	Long float64
}

type Response struct {
	Results []Result `json:"results"`
}

type Result struct {
	StatementID int     `json:"statement_id"`
	Series      []Serie `json:"series"`
}

type Serie struct {
	Name    string     `json:"name"`
	Columns []string   `json:"columns"`
	Values  [][]string `json:"values"`
}

type SensorBasicMessage struct {
	Battery     int     `json:"battery"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity,hunidity"`
}

type SensorCo2Message struct {
	Battery     int     `json:"battery"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity,hunidity"`
	Co2         int     `json:"co2"`
}

type SensorGenericMessage struct {
	RawValue json.RawMessage `json:"raw_value"`
}

type ResponseArray []Response

var applicationsCo2 = []string{"Milesight-Temperature-Humidity-CO2_54"}

var applicationsBasic = []string{"Milesight-Temperature-Humidity_53"}

var sensorsNOIBZ = []string{"NOI-A1-Floor1-CO2", "FreeSoftwareLab-Temperature"}

var sensorsNOIBRK = []string{"NOI-Brunico-Temperature"}

var brkCoordinates = Coordinates{Lat: 46.796691423886045, Long: 11.934995358540007}

var bzCoordinates = Coordinates{Lat: 46.478686716987994, Long: 11.332795944869483}

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.Env.LOG_LEVEL)

	b := bdplib.FromEnv()
	if b == nil {
		slog.Error("Failed to initialize BDP client")
		os.Exit(1)
	}
	dtSensorTemperature := "air-temperature"
	dtSensorBattery := bdplib.CreateDataType("battery-state-percent", "%", "Battery level expressed in percentage over the total", "Instantaneous")
	dtSensorHumidity := "air-humidity"
	dtSensorCo2 := bdplib.CreateDataType("co2-ppm", "ppm", "CO2 concentration in ppm", "Instantaneous")
	dtSensorGenericValues := bdplib.CreateDataType("sensor-values", "", "generic values from sensors placed in NOI facilities that do not follow either of the two specifications defined ", "Instantaneous")
	ds := []bdplib.DataType{dtSensorGenericValues, dtSensorBattery, dtSensorCo2}
	failOnError(b.SyncDataTypes(Station, ds), "Error pushing datatypes")
	log.Println("Waiting for messages. To exit press CTRL+C")

	// rabbit, err := mq.Connect(env.Env.MQ_URI, env.Env.MQ_CLIENT)
	// failOnError(err, "failed connecting to rabbitmq")
	// defer rabbit.Close()

	// dataMQ, err := rabbit.Consume(env.Env.MQ_EXCHANGE, env.Env.MQ_QUEUE, env.Env.MQ_KEY)
	// failOnError(err, "failed creating data queue")

	stackOs := tr.NewTrStack[Response](&env.Env)

	go stackOs.Start(context.Background(), func(ctx context.Context, r *dto.Raw[Response]) error {
		fmt.Println("DATA FLOWING")
		sensorDataMap := b.CreateDataMap()
		payload := r.Rawdata
		applicationName := payload.Results[0].Series[0].Values[0][1]
		sensorId := stationId(payload.Results[0].Series[0].Values[0][3], Origin)
		position := processSensorPosition(payload.Results[0].Series[0].Values[0][3])

		switch position {
		case "BZ":
			s := bdplib.CreateStation(sensorId, sensorId, Station, bzCoordinates.Lat, bzCoordinates.Long, Origin)
			if err := b.SyncStations(Station, []bdplib.Station{s}, false, false); err != nil {
				slog.Error("Error syncing stations", "err", err)

			}
			fmt.Println("station pushed")
		case "BK":
			s := bdplib.CreateStation(sensorId, sensorId, Station, brkCoordinates.Lat, brkCoordinates.Long, Origin)
			if err := b.SyncStations(Station, []bdplib.Station{s}, false, false); err != nil {
				slog.Error("Error syncing stations", "err", err)

			}
			fmt.Println("station pushed")
		}

		sensorData, err := processSensorData(applicationName, payload.Results[0].Series[0].Values[0][5])
		if err != nil {
			return err
		}

		switch data := sensorData.(type) {
		case *SensorBasicMessage:
			sensorDataMap.AddRecord(sensorId, dtSensorBattery.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Battery, Period))
			sensorDataMap.AddRecord(sensorId, dtSensorTemperature, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Temperature, Period))
			sensorDataMap.AddRecord(sensorId, dtSensorHumidity, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Humidity, Period))

		case *SensorCo2Message:
			sensorDataMap.AddRecord(sensorId, dtSensorBattery.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Battery, Period))
			sensorDataMap.AddRecord(sensorId, dtSensorTemperature, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Temperature, Period))
			sensorDataMap.AddRecord(sensorId, dtSensorHumidity, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Humidity, Period))
			sensorDataMap.AddRecord(sensorId, dtSensorCo2.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Co2, Period))

		case *SensorGenericMessage:
			sensorDataMap.AddRecord(sensorId, dtSensorGenericValues.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.RawValue, Period))

		default:
			return fmt.Errorf("unknown sensor type: %T", data)
		}
		if err := b.PushData(Station, sensorDataMap); err != nil {
			return fmt.Errorf("error pushing  data: %w", err)
		}

		slog.Info("Updated sensors data")
		return nil

	})

	select {}
}

//The payload structure could be better optimized since accessing sensor name this way is not the most elegant solution
// 	go tr.HandleQueue(dataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
// 		fmt.Println("DATA FLOWING")
// 		fmt.Println(r.Rawdata)
// 		sensorDataMap := b.CreateDataMap()
// 		payload, err := unmarshalGeneric[Response](r.Rawdata)
// 		if err != nil {
// 			slog.Error("cannot unmarshall raw data", "err", err)
// 			return err
// 		}

// 		applicationName := payload.Results[0].Series[0].Values[0][1]
// 		sensorId := stationId(payload.Results[0].Series[0].Values[0][3], Origin)

// 		position := processSensorPosition(payload.Results[0].Series[0].Values[0][3])

// 		switch position {
// 		case "BZ":
// 			s := bdplib.CreateStation(sensorId, sensorId, Station, bzCoordinates.Lat, bzCoordinates.Long, Origin)
// 			if err := b.SyncStations(Station, []bdplib.Station{s}, false, false); err != nil {
// 				slog.Error("Error syncing stations", "err", err)

// 			}
// 			fmt.Println("station pushed")
// 		case "BK":
// 			s := bdplib.CreateStation(sensorId, sensorId, Station, brkCoordinates.Lat, brkCoordinates.Long, Origin)
// 			if err := b.SyncStations(Station, []bdplib.Station{s}, false, false); err != nil {
// 				slog.Error("Error syncing stations", "err", err)

// 			}
// 			fmt.Println("station pushed")
// 		}

// 		sensorData, err := processSensorData(applicationName, payload.Results[0].Series[0].Values[0][5])
// 		if err != nil {
// 			return err
// 		}

// 		switch data := sensorData.(type) {
// 		case *SensorBasicMessage:
// 			sensorDataMap.AddRecord(sensorId, dtSensorBattery.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Battery, Period))
// 			sensorDataMap.AddRecord(sensorId, dtSensorTemperature, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Temperature, Period))
// 			sensorDataMap.AddRecord(sensorId, dtSensorHumidity, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Humidity, Period))

// 		case *SensorCo2Message:
// 			sensorDataMap.AddRecord(sensorId, dtSensorBattery.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Battery, Period))
// 			sensorDataMap.AddRecord(sensorId, dtSensorTemperature, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Temperature, Period))
// 			sensorDataMap.AddRecord(sensorId, dtSensorHumidity, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Humidity, Period))
// 			sensorDataMap.AddRecord(sensorId, dtSensorCo2.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.Co2, Period))

// 		case *SensorGenericMessage:
// 			sensorDataMap.AddRecord(sensorId, dtSensorGenericValues.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), data.RawValue, Period))

// 		default:
// 			return fmt.Errorf("unknown sensor type: %T", data)
// 		}
// 		if err := b.PushData(Station, sensorDataMap); err != nil {
// 			return fmt.Errorf("error pushing  data: %w", err)
// 		}

// 		slog.Info("Updated sensors data")
// 		return nil

// 	})

// 	select {}
// }

func failOnError(err error, msg string) {
	if err != nil {
		slog.Error(msg, "err", err)
		panic(err)
	}
}

func stationId(id string, origin string) string {
	return fmt.Sprintf("%s:%s", origin, id)
}

func unmarshalGeneric[T any](values string) (*T, error) {
	var result T
	if err := json.Unmarshal([]byte(values), &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
	}
	return &result, nil
}

func processSensorData(applicationName string, values string) (interface{}, error) {
	if slices.Contains(applicationsBasic, applicationName) {
		return unmarshalGeneric[SensorBasicMessage](values)
	} else if slices.Contains(applicationsCo2, applicationName) {
		return unmarshalGeneric[SensorCo2Message](values)
	} else {
		return &SensorGenericMessage{
			RawValue: json.RawMessage(values),
		}, nil
	}
}

func processSensorPosition(deviceName string) string {
	if slices.Contains(sensorsNOIBZ, deviceName) {
		fmt.Println("assigned")
		return "BZ"
	} else if slices.Contains(sensorsNOIBRK, deviceName) {
		fmt.Println("assigned")
		return "BK"
	} else {
		return ""
	}
}
