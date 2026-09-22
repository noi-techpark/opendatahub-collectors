// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const StationTypeTrafficSensor = "TrafficSensor"

// Sampling period of a FAMAS aggregation bucket, in seconds.
const Period = 300

// Index N of this slice is the BDP data type for FAMAS vehicle class N in
// AggregatedRecord.TotaliPerClasseVeicolare (index 0 doubles as the
// unconditional total-transits type). Order and meaning are taken verbatim
// from the legacy Java collector's Parser.getTrafficDataTypes().
var trafficDataTypes = []string{
	"total-transits",
	"number-of-motorcycles",
	"number-of-cars",
	"number-of-cars-and-minivans-with-trailer",
	"number-of-small-trucks-and-vans",
	"number-of-medium-sized-trucks",
	"number-of-big-trucks",
	"number-of-articulated-trucks",
	"number-of-articulated-lorries",
	"number-of-busses",
	"number-of-unclassified-vehicles",
	"average-vehicle-speed",
	"headway",
	"headway-variance",
	"gap",
	"gap-variance",
}

const (
	idxTotalTransits = 0
	idxAvgSpeed      = 11
	idxHeadway       = 12
	idxHeadwayVar    = 13
	idxGap           = 14
	idxGapVar        = 15
)

var env struct {
	tr.Env
	bdplib.BdpEnv
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting traffic-famas-prov-bz data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	loadSensorTypeMapping("../resources/sensor-type-mapping.csv")

	ms.FailOnError(context.Background(), syncDataTypes(b), "failed to sync data types")

	listener := tr.NewTr[string](context.Background(), env.Env)
	err := listener.Start(context.Background(),
		tr.RawString2JsonMiddleware[RawData](TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[RawData] {
	return func(ctx context.Context, payload *rdb.Raw[RawData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[RawData]) error {
	var trafficStations []bdplib.Station

	trafficData := bdp.CreateDataMap()

	for _, station := range payload.Rawdata.Stations {
		for _, lane := range station.CorsieInfo {
			trafficStation := createTrafficStation(bdp, station, lane)
			trafficStations = append(trafficStations, trafficStation)
			addTrafficMeasurements(trafficData, trafficStation.Id, station.AggregatedData, lane)
		}
	}

	slog.InfoContext(ctx, "syncing stations and pushing data", "trafficStations", len(trafficStations))

	if err := bdp.SyncStations(StationTypeTrafficSensor, trafficStations, true, false); err != nil {
		return fmt.Errorf("failed to sync %s stations: %w", StationTypeTrafficSensor, err)
	}

	if err := bdp.PushData(StationTypeTrafficSensor, trafficData); err != nil {
		return fmt.Errorf("failed to push %s records: %w", StationTypeTrafficSensor, err)
	}

	return nil
}

// createTrafficStation mirrors the legacy Parser.createStation() for
// stationType=TrafficSensor: one station per lane, id
// "<station.Nome>:<lane.Descrizione>" - preserved verbatim so this
// migration keeps writing to the same BDP station codes the legacy
// collector already created.
func createTrafficStation(bdp bdplib.Bdp, station StationRaw, lane LaneRaw) bdplib.Station {
	id := station.Nome + ":" + lane.Descrizione

	s := bdplib.CreateStation(id, id, StationTypeTrafficSensor,
		station.GeoInfo.Latitudine, station.GeoInfo.Longitudine, bdp.GetOrigin())

	meta := commonStationMetadata(station)
	// []string, not a bare string: the legacy collector built this via a
	// JsonPath filter expression ("$.corsieInfo[?(@.id == laneId)].descrizione"),
	// which always yields an array even for a single match - production
	// BDP already has "direction": ["..."] for every existing station, so
	// this is kept array-shaped for metadata compatibility.
	meta["direction"] = []string{lane.Descrizione}
	meta["sensor_type"] = sensorType(id)
	s.MetaData = meta

	return s
}

func commonStationMetadata(station StationRaw) map[string]interface{} {
	return map[string]interface{}{
		"municipality": station.GeoInfo.Comune,
		"region":       station.GeoInfo.Regione,
		"street_name":  station.StradaInfo.Nome,
		"kilometric":   station.StradaInfo.Chilometrica,
		"total_lanes":  station.NumeroCorsie,
	}
}

// addTrafficMeasurements mirrors Parser.insertDataIntoStationMap(): out of
// all aggregated records for the station (one API call already scoped to
// the station, covering every lane), keep only the rows matching this
// lane's 0-indexed FAMAS lane number and direction.
func addTrafficMeasurements(dm bdplib.DataMap, stationCode string, records []AggregatedRecord, lane LaneRaw) {
	famasLaneIndex := lane.ID - 1

	for _, rec := range records {
		if rec.Corsia != famasLaneIndex || rec.Direzione != lane.SensoDiMarcia {
			continue
		}

		ts, err := parseFamasTime(rec.Data)
		if err != nil {
			slog.Warn("skipping aggregated record with unparseable timestamp", "station", stationCode, "data", rec.Data, "err", err)
			continue
		}

		addRecord(dm, stationCode, trafficDataTypes[idxTotalTransits], ts, rec.TotaleVeicoli)

		if rec.TotaliPerClasseVeicolare == nil {
			continue
		}

		for class, count := range rec.TotaliPerClasseVeicolare {
			idx, err := strconv.Atoi(class)
			if err != nil || idx < 0 || idx >= len(trafficDataTypes) {
				slog.Warn("skipping unknown vehicle class", "station", stationCode, "class", class)
				continue
			}
			count := count
			addRecord(dm, stationCode, trafficDataTypes[idx], ts, &count)
		}

		addRecord(dm, stationCode, trafficDataTypes[idxAvgSpeed], ts, rec.MediaArmonicaVelocita)
		addRecord(dm, stationCode, trafficDataTypes[idxHeadway], ts, rec.HeadwayMedioSecondi)
		addRecord(dm, stationCode, trafficDataTypes[idxHeadwayVar], ts, rec.VarianzaHeadwayMedioSecondi)
		addRecord(dm, stationCode, trafficDataTypes[idxGap], ts, rec.GapMedioSecondi)
		addRecord(dm, stationCode, trafficDataTypes[idxGapVar], ts, rec.VarianzaGapMedioSecondi)
	}
}

func addRecord(dm bdplib.DataMap, stationCode, dataType string, ts time.Time, value *float64) {
	if value == nil {
		return
	}
	dm.AddRecord(stationCode, dataType, bdplib.CreateRecord(ts.UnixMilli(), *value, Period))
}

// parseFamasTime parses FAMAS's "yyyy-MM-dd'T'HH:mm:ss" timestamps, always
// as UTC. FAMAS inconsistently appends a literal "Z" to some timestamps;
// strip it before parsing rather than relying on Go to validate it away,
// mirroring the legacy Java SimpleDateFormat's lenient (partial-match)
// parsing.
func parseFamasTime(s string) (time.Time, error) {
	s = strings.TrimSuffix(s, "Z")
	return time.ParseInLocation("2006-01-02T15:04:05", s, time.UTC)
}

// trafficDataTypeMeta holds unit/description/rtype exactly as already
// registered in production BDP for each name in trafficDataTypes (queried
// from the public ODH API) - kept byte-for-byte so this migration doesn't
// silently change labels/units already relied on by consumers (Italian
// descriptions and unit "nr"/"sec" included, not translated to English).
// headway-variance/gap-variance are the two types the legacy collector's
// SyncScheduler.initDataTypes() ever registered explicitly (rtype
// "Average", description literally the name, no unit) - everything else
// was apparently registered some other way (not visible in the legacy
// collector's own source) but is preserved as observed.
var trafficDataTypeMeta = map[string]struct{ unit, description, rtype string }{
	"total-transits":                           {"nr", "Totale Transiti", ""},
	"number-of-motorcycles":                    {"nr", "Totali per Classe Moto", ""},
	"number-of-cars":                           {"nr", "Totali per Classe Autovetture", ""},
	"number-of-cars-and-minivans-with-trailer": {"nr", "Totali per Classe Auto e monovolume con rimorchio", ""},
	"number-of-small-trucks-and-vans":          {"nr", "Totali per Classe Furgoncini e camioncini", ""},
	"number-of-medium-sized-trucks":            {"nr", "Totali per Classe Camion medi (fino a 7,5 m)", ""},
	"number-of-big-trucks":                     {"nr", "Totali per Classe Camion grandi", ""},
	"number-of-articulated-trucks":             {"nr", "Totali per Classe Autoarticolati (trattori con semirimorchio)", ""},
	"number-of-articulated-lorries":            {"nr", "Totali per Classe Autotreni (autocarri con rimorchio)", ""},
	"number-of-busses":                         {"nr", "Totali per Classe Autobus", ""},
	"number-of-unclassified-vehicles":          {"nr", "Totali per Classe Altri (veicoli non classificati)", ""},
	"average-vehicle-speed":                    {"km/h", "Velocità Media", ""},
	"headway":                                  {"sec", "Headway", ""},
	"headway-variance":                         {"", "headway-variance", "Average"},
	"gap":                                      {"sec", "Gap", ""},
	"gap-variance":                             {"", "gap-variance", "Average"},
}

func syncDataTypes(bdp bdplib.Bdp) error {
	var dataTypes []bdplib.DataType

	for _, name := range trafficDataTypes {
		meta := trafficDataTypeMeta[name]
		dataTypes = append(dataTypes, bdplib.CreateDataType(name, meta.unit, meta.description, meta.rtype))
	}

	return bdp.SyncDataTypes(dataTypes)
}

var sensorTypeByStation map[string]string

const defaultSensorType = "induction_loop"

// loadSensorTypeMapping mirrors SensorTypeUtil: an optional station-code ->
// sensor-technology CSV lookup, defaulting to "induction_loop" for any
// station not listed (which, as of the legacy CSV, is every listed station
// too - all rows currently map to the same default).
func loadSensorTypeMapping(path string) {
	sensorTypeByStation = map[string]string{}

	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("sensor-type-mapping.csv not found, defaulting every station to induction_loop", "path", path, "err", err)
		return
	}

	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if i == 0 || line == "" {
			continue // header
		}
		fields := strings.SplitN(line, ",", 2)
		if len(fields) != 2 {
			continue
		}
		sensorTypeByStation[fields[0]] = strings.TrimSpace(fields[1])
	}
}

func sensorType(stationCode string) string {
	if t, ok := sensorTypeByStation[stationCode]; ok && t != "" {
		return t
	}
	return defaultSensorType
}
