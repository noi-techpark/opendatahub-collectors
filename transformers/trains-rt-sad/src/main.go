// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"log/slog"
	"os"
	"strings"

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
	NETEX_PATH string `default:"netex.xml"`
}

const STATIONTYPE = "ExampleStation"
const PERIOD = 600

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
			Line  string `json:"line"`
			Trip  string `json:"trip"`
			Train any    `json:"train"`
			Delay int    `json:"delay"`
			Time  string `json:"time"`
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

	netexF, err := os.ReadFile(env.NETEX_PATH)
	ms.FailOnError(ctx, err, "could not read netex file %s", env.NETEX_PATH)

	n := netex.PublicationDelivery{}
	err = xml.Unmarshal(netexF, &n)
	ms.FailOnError(ctx, err, "could not unmarshal netex")

	cache := NewCache()

	listener := tr.NewTr[string](ctx, env.Env)
	err = listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[string]) error {
		raw := Dto{}
		if err := json.Unmarshal([]byte(r.Rawdata), &raw); err != nil {
			return err
		}

		// lineRef = TimeTableFrame/VehicleJourneys[trainNumbers/TrainNumberRef like TrainNumber:(json.train)].LineRef
		// directionRef = ServiceFrame/journeyPatterns[id = ServiceJourney.ServiceJourneyPatternRef]/DirectionType
		// publishedLineName = ServiceFrame/Line[id = ServiceJourney.LineRef]/Name
		// DirectionName = ServiceFrame/destinationDisplay[id = journeyPattern/pointsinSequence[-1].DestinationDisplayRef]
		// OperatorRef = vehicleJourney.OperatorRef

		// compose siri-vm
		// upload
		return nil
	})

	ms.FailOnError(ctx, err, "error while listening to queue")
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
