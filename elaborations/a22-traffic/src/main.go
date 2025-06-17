// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/opendatahub-go-sdk/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	_ "github.com/lib/pq"
)

const (
	sensorStationType = "TrafficSensor"

	DataTypeLightVehicles      = "Nr. Light Vehicles"
	DataTypeHeavyVehicles      = "Nr. Heavy Vehicles"
	DataTypeBuses              = "Nr. Buses"
	DataTypeEquivalentVehicles = "Nr. Equivalent Vehicles"
	DataTypeAvgSpeedLight      = "Average Speed Light Vehicles"
	DataTypeAvgSpeedHeavy      = "Average Speed Heavy Vehicles"
	DataTypeAvgSpeedBuses      = "Average Speed Buses"
	DataTypeVarSpeedLight      = "Variance Speed Light Vehicles"
	DataTypeVarSpeedHeavy      = "Variance Speed Heavy Vehicles"
	DataTypeVarSpeedBuses      = "Variance Speed Buses"
	DataTypeAvgGap             = "Average Gap"
	DataTypeAvgHeadway         = "Average Headway"
	DataTypeAvgDensity         = "Average Density"
	DataTypeAvgFlow            = "Average Flow"
	DataTypeEuroPct            = "EURO Category Pct"
	DataTypeNationalityCount   = "Plate Nationality Count"

	MeasurementPeriod uint64 = 600
)

// Hardcoded minimum allowed start time: 2024-Jul-09 23:59:59 UTC
var hardcodedMinStartTime = time.Date(2024, time.July, 9, 23, 59, 59, 0, time.UTC).UnixMilli()

// inconsistent data start:  2024-Jul-09 23:59:59 UTC
var InconsistentDataStart = time.Date(2024, time.July, 9, 23, 59, 59, 0, time.UTC).UnixMilli()

// inconsistent data end:  2024-Nov-26 23:59:59 UTC
var InconsistentDataEnd = time.Date(2024, time.November, 26, 23, 59, 59, 0, time.UTC).UnixMilli()

// allDataTypes is the complete list of data‐types we expect.
var allDataTypes = []string{
	DataTypeLightVehicles,
	DataTypeHeavyVehicles,
	DataTypeBuses,
	DataTypeEquivalentVehicles,
	DataTypeAvgSpeedLight,
	DataTypeAvgSpeedHeavy,
	DataTypeAvgSpeedBuses,
	DataTypeVarSpeedLight,
	DataTypeVarSpeedHeavy,
	DataTypeVarSpeedBuses,
	DataTypeAvgGap,
	DataTypeAvgHeadway,
	DataTypeAvgDensity,
	DataTypeAvgFlow,
	DataTypeEuroPct,
	DataTypeNationalityCount,
}

var dataTypes []bdplib.DataType
var dataTypesFilter []string

var env struct {
	tr.Env
	DatabaseURL string `envconfig:"DATABASE_URL"`

	NINJA_URL      string `envconfig:"NINJA_URL"`
	NINJA_CONSUMER string `envconfig:"NINJA_CONSUMER"`

	CRON string `envconfig:"CRON"`
}

type CronLogger struct {
	log *slog.Logger
}

func (c CronLogger) Info(msg string, keysAndValues ...interface{}) {
	c.log.Info(msg, keysAndValues...)
}

func (c CronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	c.log.Error(msg, append([]interface{}{"error", err}, keysAndValues...)...)
}

func setupNinja() {
	odhts.C.BaseUrl = env.NINJA_URL
	odhts.C.Referer = env.NINJA_CONSUMER
}

func createDBConnection() (*sqlx.DB, error) {
	host := os.Getenv("A22DB_HOST")
	port := os.Getenv("A22DB_PORT")
	dbname := os.Getenv("A22DB_NAME")
	sslmode := os.Getenv("A22DB_SSLMODE")

	user := os.Getenv("A22DB_USER")
	password := os.Getenv("A22DB_PASSWORD")

	dsn := fmt.Sprintf("host=%s port=%s dbname=%s sslmode=%s user=%s password=%s",
		host, port, dbname, sslmode, user, password)

	return sqlx.Open("postgres", dsn)
}

var sensorUtils *SensorTypeUtil = nil
var euroTypeUtils *EUROTypeUtil = nil

func milliToRFC3339(milli int64) string {
	return time.Unix(milli/1000, (milli%1000)*1_000_000).UTC().Format(time.RFC3339)
}

const MaxWorkers = 8 // tune based on your CPU/resources

type stationTask struct {
	Station Station
	Meas    *measurementMap
}

func createBatchSpan(ctx context.Context) (context.Context, trace.Span) {
	// span per window and link to main trace
	windowCtx := ctx
	var windowSpan trace.Span = noop.Span{}
	// link span without full trace
	rootContext := trace.SpanContextFromContext(ctx)
	if rootContext.IsValid() {
		windowCtx, windowSpan = tel.TraceStart(context.Background(), fmt.Sprintf("%s.station-window", tel.GetServiceName()),
			trace.WithLinks(trace.Link{
				SpanContext: rootContext,
			}),
			trace.WithSpanKind(trace.SpanKindInternal),
		)
	}
	return windowCtx, windowSpan
}

func processStationTask(ctx context.Context, task stationTask, horizon int64, bdp bdplib.Bdp, ad22DbConnection *sqlx.DB) {
	station := task.Station
	meas := task.Meas

	// if the min timestamp of this station (the type with the most past measurement) is >= station MaxTimestamp,
	// it means there are no new data to consume for this station, skip it
	minMeasTs := meas.startFrom(station)
	if minMeasTs.UnixMilli() >= station.MaxTimestamp {
		return
	}

	// we get vehicles from meas First to be sure to process all data types, not only the most ahead
	startTime := max(station.MinTimestamp, minMeasTs.UnixMilli())
	// Apply hardcoded limit so startTime can't go back before 2024-Jul-09 23:59:59
	startTime = max(startTime, hardcodedMinStartTime)

	endTime := min(station.MaxTimestamp, horizon)
	logger.Get(ctx).Info("processing station",
		"stationcode", station.Id,
		"start_time", milliToRFC3339(startTime),
		"end_time", milliToRFC3339(endTime))

	windowLength := int64(MeasurementPeriod * 1000)
	batchWindowCount := 10
	batchWindowLength := windowLength * int64(batchWindowCount)

	// align start time with the nearest 10*x minute timestamp
	alignedStartTime := (startTime / windowLength) * windowLength
	for window := alignedStartTime; window < endTime; window += batchWindowLength {
		// exclude windows where data is insonsistent
		if window >= InconsistentDataStart && window <= InconsistentDataEnd {
			continue
		}

		// Cap windowEnd so it doesn't exceed endTime
		windowEnd := window + batchWindowLength
		if windowEnd > endTime {
			windowEnd = endTime
		}

		batchCtx, batchSpan := createBatchSpan(ctx)
		defer batchSpan.End()

		logger.Get(batchCtx).Info("batch query", "station", station.Id,
			"window_start", milliToRFC3339(window), "window_end", milliToRFC3339(windowEnd))

		vehicles, err := ReadVehiclesWindow(context.Background(), ad22DbConnection, window, windowEnd, station.Id)
		ms.FailOnError(batchCtx, err, "failed to get vehicles", "station", station.Id,
			"window_start", milliToRFC3339(window), "window_end", milliToRFC3339(windowEnd))

		// Split vehicles by their window
		vehicleMap := splitVehiclesByWindow(vehicles, window, windowLength)

		dataMap := bdp.CreateDataMap()

		for i := 0; i < batchWindowCount; i++ {
			winStart := window + int64(i)*windowLength
			if winStart >= windowEnd {
				break
			}

			winEnd := winStart + windowLength
			vInWindow := vehicleMap[i]

			logger.Get(batchCtx).Debug("elaborating", "station", station.Id,
				"window_start", milliToRFC3339(winStart), "window_end", milliToRFC3339(winEnd), "vehicle_count", len(vInWindow))

			err = elaborate(batchCtx, &dataMap, meas, station, vInWindow, winEnd, MeasurementPeriod)
			ms.FailOnError(batchCtx, err, "failed to elaborate vehicles", "station", station.Id,
				"window_start", milliToRFC3339(winStart), "window_end", milliToRFC3339(winEnd))
		}

		err = bdp.PushData(station.StationType, dataMap)
		ms.FailOnError(batchCtx, err, "failed to push data", "station", station.Id,
			"window_start", milliToRFC3339(window), "window_end", milliToRFC3339(windowEnd))
	}
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	// Setup utils
	sensorUtils = NewSensorTypeUtil()
	euroTypeUtils = NewEUROTypeUtil()

	// Setup connection
	ad22DbConnection, err := createDBConnection()
	defer ad22DbConnection.Close()
	ms.FailOnError(context.Background(), err, "failed to crated onnection to a22 db")

	// Setup Ninja
	setupNinja()

	// Setup BDP
	bdp := bdplib.FromEnv()

	// SyncTypes
	SyncDataTypes(bdp)

	///////////////////
	ninjaTokenProvider := NewOAuthProvider()

	// Setup Cron Job
	c := cron.New(
		cron.WithSeconds(),
		cron.WithChain(
			cron.DelayIfStillRunning(
				CronLogger{log: logger.Get(context.Background())},
			),
		),
	)

	c.AddFunc(env.CRON, func() {
		//////////////////////////
		now := time.Now().UTC()
		// horizon ensures that we do not use real-time data which might be incomplete
		// the horizon must be a multiple of the Period, otherwise it uses partial data of the last period and the rest of the
		// data wont be processed since we checkpoint the last window's endtime.
		// right now it is 5 * period meaning 50 minutes
		horizon := now.UnixMilli() - (5 * int64(MeasurementPeriod) * 1000)

		ctx := context.Background()

		// root server span to enable RED collection of the collector span
		ctx, serverSpan := tel.TraceStart(
			ctx,
			fmt.Sprintf("%s.trigger", tel.GetServiceName()),
			trace.WithSpanKind(trace.SpanKindServer),
		)

		// collect span creation
		ctx, producerSpan := tel.TraceStart(
			ctx,
			fmt.Sprintf("%s.collect", tel.GetServiceName()),
			trace.WithSpanKind(trace.SpanKindProducer),
		)

		defer serverSpan.End()
		defer producerSpan.End()

		///////////////// Read stations from DB
		stations, err := readStations(ctx, ad22DbConnection, bdp.GetOrigin(), sensorStationType)
		ms.FailOnError(ctx, err, "failed to get stations from a22 db")

		measurements, err := getMeasurementsByStation(ctx, ninjaTokenProvider)
		ms.FailOnError(ctx, err, "failed to get measurements from ninja")

		// sync stations
		bdpStations := make([]bdplib.Station, len(stations))
		for i, s := range stations {
			bdpStations[i] = bdplib.Station(s.Station)
		}
		err = bdp.SyncStations(sensorStationType, bdpStations, true, true)
		ms.FailOnError(ctx, err, "failed to sync stations")

		stationChan := make(chan stationTask)
		var wg sync.WaitGroup

		// Start workers
		logger.Get(ctx).Info(fmt.Sprintf("spawning %d workers", MaxWorkers))
		for i := 0; i < MaxWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for task := range stationChan {
					processStationTask(ctx, task, horizon, bdp, ad22DbConnection)
				}
			}()
		}

		// Enqueue tasks
		for _, station := range stations {
			meas, ok := measurements[station.Id]
			if !ok {
				continue
			}
			stationChan <- stationTask{Station: station, Meas: meas}
		}
		close(stationChan) // no more tasks

		wg.Wait()

		logger.Get(ctx).Info("elaboration completed", "runtime_ms", time.Since(now).Milliseconds())
	})

	c.Run()
}

func SyncDataTypes(bdp bdplib.Bdp) {
	// Counts
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeLightVehicles, "", "Number of light vehicles", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeHeavyVehicles, "", "Number of heavy vehicles", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeBuses, "", "Number of buses", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeEquivalentVehicles, "", "Number of equivalent vehicles", "Mean"))

	// Average Speeds
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgSpeedLight, "km/h", "Average Speed Light Vehicles", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgSpeedHeavy, "km/h", "Average Speed Heavy Vehicles", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgSpeedBuses, "km/h", "Average Speed Buses", "Mean"))

	// Variance Speeds
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeVarSpeedLight, "km/h", "Variance Speed Light Vehicles", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeVarSpeedHeavy, "km/h", "Variance Speed Heavy Vehicles", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeVarSpeedBuses, "km/h", "Variance Speed Buses", "Mean"))

	// Gaps and Headways
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgGap, "s", "Average Gap", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgHeadway, "s", "Average Headway", "Mean"))

	// Density and Flow
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgDensity, "vehicles / km", "Average Density", "Mean"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeAvgFlow, "vehicles / hour", "Average Flow", "Mean"))

	// Euro emission category
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeEuroPct, "%", "Euro emission standards distribution", "Mean"))

	// Plate nationality
	dataTypes = append(dataTypes, bdplib.CreateDataType(DataTypeNationalityCount, "", "Vehicle Count by License Plate Nationality", "Count"))

	// Sync
	err := bdp.SyncDataTypes(sensorStationType, dataTypes)
	ms.FailOnError(context.Background(), err, "failed to sync data types")

	// Build filter
	for _, dt := range dataTypes {
		dataTypesFilter = append(dataTypesFilter, dt.Name)
	}
}

//////////////////////////////////////////

// OAuthProvider struct
type OAuthProvider struct {
	conf        *oauth2.Config
	clientCreds *clientcredentials.Config
	token       *oauth2.Token
	mu          sync.Mutex
}

// NewOAuthProvider initializes the OAuth2 wrapper
func NewOAuthProvider() *OAuthProvider {
	tokenURL := os.Getenv("NINJA_TOKEN_URL")
	clientID := os.Getenv("NINJA_CLIENT_ID")
	clientSecret := os.Getenv("NINJA_CLIENT_SECRET")
	scopes := os.Getenv("NINJA_SCOPES")

	wrapper := &OAuthProvider{}

	wrapper.clientCreds = &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       strings.Split(scopes, ","),
	}

	return wrapper
}

// GetToken retrieves a valid access token (refreshing if necessary)
func (w *OAuthProvider) GetToken() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ctx := context.Background()

	// If token exists and is still valid, return it
	if w.token != nil && w.token.Valid() {
		return w.token.AccessToken, nil
	}

	// Fetch new token
	var token *oauth2.Token
	var err error

	token, err = w.clientCreds.Token(ctx)

	if err != nil {
		return "", err
	}

	// Store new token
	w.token = token
	return token.AccessToken, nil
}
