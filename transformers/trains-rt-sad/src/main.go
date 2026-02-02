// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-netex"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env struct {
	tr.Env
	bdplib.BdpEnv
	NETEX_PATH      string `default:"netex.xml"`
	FILESERVER_HOST string `default:"http://files-nginx-internal.core.svc.cluster.local"`
}

type Dto struct {
	Status string `json:"status"`
	Data   []struct {
		RollingStock struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"rolling_stock"`
		Position struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Time      string  `json:"time"`
		} `json:"position"`
		Status struct {
			Code int    `json:"code"`
			Time string `json:"time"`
		} `json:"status"`
		Trip struct {
			// These fields are sometimes "0" and sometimes 0
			Line  json.Number `json:"line"`
			Trip  json.Number `json:"trip"`
			Train string      `json:"train"`
			Delay int         `json:"delay"`
			Time  string      `json:"time"`
		} `json:"trip"`
		Composition struct {
			Chain struct {
				PositionInChain int      `json:"positionInChain"`
				Chain           []string `json:"chain"`
			} `json:"chain"`
			Time string `json:"time"`
		} `json:"composition"`
	} `json:"data"`
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	n := netex.PublicationDelivery{}
	cache := NewCache()
	nTime := time.Time{}

	var mu sync.Mutex

	listener := tr.NewTr[string](ctx, env.Env)
	err := listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[string]) error {

		// Every 24 hours there is a new Netex file. Make sure we're somewhat up to date
		mu.Lock()
		defer mu.Unlock()
		if time.Since(nTime).Hours() > 24 {
			ms.FailOnError(ctx, downloadLatestNetex(), "could not download most recent netex file")
			n = netex.PublicationDelivery{}
			netexF, err := os.ReadFile(env.NETEX_PATH)
			ms.FailOnError(ctx, err, "could not read netex file %s", env.NETEX_PATH)
			err = xml.Unmarshal(netexF, &n)
			ms.FailOnError(ctx, err, "could not unmarshal netex")
			cache = NewCache()
			nTime = time.Now()
		}

		dto := Dto{}
		if err := json.Unmarshal([]byte(r.Rawdata), &dto); err != nil {
			return err
		}

		s, err := raw2Siri(cache, r.Timestamp, dto, n)
		if err != nil {
			return err
		}

		siriBytes, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := putFile(env.FILESERVER_HOST, "/siri-lite/vehicle-monitoring/trains/sad.json", siriBytes); err != nil {
			return err
		}

		siriBytes, err = xml.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := putFile(env.FILESERVER_HOST, "/siri-lite/vehicle-monitoring/trains/sad.xml", siriBytes); err != nil {
			return err
		}

		return nil
	})

	ms.FailOnError(ctx, err, "error while listening to queue")
}

func raw2Siri(c *Cache, refTime time.Time, r Dto, n netex.PublicationDelivery) (Siri, error) {
	producer := "TBD"
	respTs := time.Now().Format(time.RFC3339)
	s := NewSiri()
	s.ServiceDelivery.ProducerRef = producer
	s.ServiceDelivery.ResponseTimestamp = respTs
	s.ServiceDelivery.VehicleMonitoringDelivery.ProducerRef = producer
	s.ServiceDelivery.VehicleMonitoringDelivery.ResponseTimestamp = respTs

	locItaly, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		return s, fmt.Errorf("cannot find Europe/Rome tz data: %w", err)
	}

	parseTs := func(ts string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04:05", ts, locItaly)
		if err != nil {
			fmt.Println(err)
			return time.Time{}
		}
		return tm
	}

	for _, upd := range r.Data {
		// filter out trains which have state different from 3 and state = 3 but with “old” timestamps
		statusTime := parseTs(upd.Status.Time)
		train := upd.Trip.Train
		// TODO: define exactly what old timestamp means
		if upd.Status.Code != 3 || refTime.Sub(statusTime).Hours() > 24 || train == "" {
			continue
		}

		va := VehicleActivity{}
		posTime := parseTs(upd.Position.Time)
		va.RecordedAtTime = posTime.Format(time.RFC3339)
		va.ValidUntilTime = posTime.Add(time.Hour * 24).Format(time.RFC3339)
		vj := &va.MonitoredVehicleJourney

		nJourney := findJourney(n, c, train)
		if nJourney == nil {
			slog.Warn("could not find journey for train in static data. Ignoring record", "train", train)
			continue
		}
		vj.LineRef = nJourney.LineRef.Ref

		nJourneyPattern := findJourneyPattern(n, c, nJourney.ServiceJourneyPatternRef.Ref)
		if nJourneyPattern == nil {
			return s, fmt.Errorf("could not find journey pattern for train %s in static data", train)
		}
		vj.DirectionRef = nJourneyPattern.DirectionType

		vj.FramedVehicleJourneyRef.DataFrameRef = refTime.Format(time.RFC3339)
		vj.FramedVehicleJourneyRef.DatedVehicleJourneyRef = "TBD"

		nLine := findLine(n, c, nJourney.LineRef.Ref)
		if nLine == nil {
			return s, fmt.Errorf("could not find Line %s in static data", nJourney.LineRef.Ref)
		}
		vj.PublishedLineName = nLine.Name

		lastStop := (*nJourneyPattern.PointsInSequence)[len((*nJourneyPattern.PointsInSequence))-1]
		nDestDis := findDestinationDisplay(n, c, lastStop.DestinationDisplayRef.Ref)
		if nDestDis != nil {
			vj.DirectionName = nDestDis.Name
		}

		vj.OperatorRef = nJourney.OperatorRef.Ref

		vj.ProductCategoryRef = "unknown"
		vj.Monitored = true
		vj.InCongestion = false
		vj.VehicleLocation.Latitude = float32(upd.Position.Latitude)
		vj.VehicleLocation.Longitude = float32(upd.Position.Longitude)
		vj.Delay = mapDelay(upd.Trip.Delay)
		vj.VehicleRef = train //TODO: this is not correct, should be a valid ID, but there are no vehicles defined in the reference Netex. Sta does it like this on their other SIRI-VM though

		s.ServiceDelivery.VehicleMonitoringDelivery.VehicleActivity = append(s.ServiceDelivery.VehicleMonitoringDelivery.VehicleActivity, va)
	}
	return s, nil
}

func mapDelay(d int) string {
	// delay for Siri is in seconds. we assume our source is in minutes
	return fmt.Sprintf("PT%dS", d*60)
}

type Cache struct {
	journeys            map[string]*netex.ServiceJourney
	journeyPatterns     map[string]*netex.ServiceJourneyPattern
	lines               map[string]*netex.Line
	destinationDisplays map[string]*netex.DestinationDisplay
}

func NewCache() *Cache {
	return &Cache{
		journeys:            make(map[string]*netex.ServiceJourney),
		journeyPatterns:     make(map[string]*netex.ServiceJourneyPattern),
		lines:               make(map[string]*netex.Line),
		destinationDisplays: make(map[string]*netex.DestinationDisplay),
	}
}

// TODO: all these do not handle versioning, we just find the first one that's matching and hope for the best
func findJourney(n netex.PublicationDelivery, cache *Cache, train string) *netex.ServiceJourney {
	if journey, ok := cache.journeys[train]; ok {
		return journey
	}

	suffix := "TrainNumber:" + train
	for _, cf := range n.DataObjects {
		for _, tf := range cf.Frames.TimetableFrame {
			if tf.VehicleJourneys != nil {
				for _, journey := range *tf.VehicleJourneys {
					if journey.TrainNumbers != nil {
						for _, ref := range *journey.TrainNumbers {
							if strings.HasSuffix(ref.Ref, suffix) {
								cache.journeys[train] = &journey
								return &journey
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func findJourneyPattern(n netex.PublicationDelivery, cache *Cache, id string) *netex.ServiceJourneyPattern {
	if pattern, ok := cache.journeyPatterns[id]; ok {
		return pattern
	}

	for _, cf := range n.DataObjects {
		for _, sf := range cf.Frames.ServiceFrame {
			if sf.JourneyPatterns != nil {
				for _, pattern := range *sf.JourneyPatterns {
					if pattern.Id == id {
						cache.journeyPatterns[id] = &pattern
						return &pattern
					}
				}
			}
		}
	}
	return nil
}

func findLine(n netex.PublicationDelivery, cache *Cache, id string) *netex.Line {
	if line, ok := cache.lines[id]; ok {
		return line
	}

	for _, cf := range n.DataObjects {
		for _, sf := range cf.Frames.ServiceFrame {
			if sf.Lines != nil {
				for _, line := range *sf.Lines {
					if line.Id == id {
						cache.lines[id] = &line
						return &line
					}
				}
			}
		}
	}
	return nil
}

func findDestinationDisplay(n netex.PublicationDelivery, cache *Cache, id string) *netex.DestinationDisplay {
	if display, ok := cache.destinationDisplays[id]; ok {
		return display
	}

	for _, cf := range n.DataObjects {
		for _, sf := range cf.Frames.ServiceFrame {
			if sf.DestinationDisplays != nil {
				for _, display := range *sf.DestinationDisplays {
					if display.Id == id {
						cache.destinationDisplays[id] = &display
						return &display
					}
				}
			}
		}
	}
	return nil
}

func downloadLatestNetex() error {
	ftpHost := "ftp.sta.bz.it:21"
	ftpDir := fmt.Sprintf("/netex/%d/plan/allTrains", time.Now().Year())

	// Connect
	conn, err := ftp.Dial(ftpHost, ftp.DialWithTimeout(10*time.Second))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Quit()

	// Anonymous login
	if err := conn.Login("anonymous", "anonymous"); err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}

	// List files matching *.xml
	entries, err := conn.List(ftpDir)
	if err != nil {
		return fmt.Errorf("failed to list directory: %w", err)
	}

	// Filter *.xml and sort by time descending
	var xmlEntries []*ftp.Entry
	for _, e := range entries {
		if e.Type == ftp.EntryTypeFile && strings.HasSuffix(e.Name, ".xml") {
			xmlEntries = append(xmlEntries, e)
		}
	}

	if len(xmlEntries) == 0 {
		return fmt.Errorf("no XML files found")
	}

	sort.Slice(xmlEntries, func(i, j int) bool {
		return xmlEntries[i].Time.After(xmlEntries[j].Time)
	})

	latest := xmlEntries[0].Name
	fmt.Println("Downloading:", latest)

	// Download
	r, err := conn.Retr(ftpDir + "/" + latest)
	if err != nil {
		return fmt.Errorf("failed to retrieve file: %w", err)
	}
	defer r.Close()

	tmpFile, err := os.Create("netex.new.xml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, r); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	tmpFile.Close()

	return os.Rename("netex.new.xml", "netex.xml")
}

func putFile(host, path string, data []byte) error {
	url := host + path
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return nil
}
