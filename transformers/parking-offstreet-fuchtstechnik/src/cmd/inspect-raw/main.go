// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// inspect-raw is a one-shot diagnostic: it queries the Fuchtstechnik raw
// data collection in MongoDB, decodes each document's base64 rawdata
// (which is JSON shaped like ParkingEvent), and prints the wall-clock
// timestamps each measurement carries. It flags any measurement whose
// timestamp lies in the future relative to "now" — useful to tell
// provider-emitted future timestamps apart from our own parse bugs.
//
// Usage:
//
//	go run ./cmd/inspect-raw \
//	  --uri='mongodb://root:PASS@localhost:27017/?directConnection=true' \
//	  --db=fuchstechnik --collection=parking-stations \
//	  --limit=50
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// rawDoc mirrors the document shape stored by the ingest pipeline. Only
// the fields we read are listed; everything else in the BSON is ignored.
type rawDoc struct {
	ID        any       `bson:"_id"`
	Provider  string    `bson:"provider"`
	Timestamp time.Time `bson:"timestamp"`
	Rawdata   string    `bson:"rawdata"`
}

// parkingEvent matches the transformer's DTO: one provider event with
// 1+ availability measurements. The provider emits naive local time
// (Europe/Rome) in the timestamp string.
type parkingEvent struct {
	ID           string        `json:"id"`
	NameIT       string        `json:"name_IT"`
	NameDE       string        `json:"name_DE"`
	Capacity     int           `json:"capacity"`
	Measurements []measurement `json:"measurements"`
}

type measurement struct {
	Timestamp    string `json:"timestamp"`
	Availability int    `json:"availability"`
}

const tsLayout = "2006-01-02 15:04:05"

func main() {
	uri := flag.String("uri", "mongodb://root:02sf5DrHDn@localhost:27017/?directConnection=true", "MongoDB URI")
	dbName := flag.String("db", "fuchstechnik", "database name")
	coll := flag.String("collection", "parking-stations", "collection name")
	limit := flag.Int64("limit", 0, "max documents to inspect (0 = unlimited, newest first)")
	sinceHours := flag.Int("since-hours", 0, "only inspect docs stored within the last N hours (0 = all)")
	onlyFuture := flag.Bool("only-future", false, "print only docs containing a future measurement")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*uri))
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Ping(ctx, nil); err != nil {
		fatalf("ping: %v", err)
	}

	c := client.Database(*dbName).Collection(*coll)

	now := time.Now()
	romeLoc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		fatalf("load Europe/Rome: %v", err)
	}

	filter := bson.M{}
	if *sinceHours > 0 {
		// The ingest pipeline stores two timestamp fields:
		//   - "timestamp"      → RFC3339 *string*
		//   - "bsontimestamp"  → BSON Date
		// We filter on the Date field so $gte works against a Go time.Time.
		since := now.Add(-time.Duration(*sinceHours) * time.Hour).UTC().Round(0)
		filter["bsontimestamp"] = bson.M{"$gte": since}
		fmt.Printf("    since (UTC)=%s\n", since.Format(time.RFC3339))
	}
	findOpts := options.Find().SetSort(bson.D{{Key: "bsontimestamp", Value: -1}})
	if *limit > 0 {
		findOpts.SetLimit(*limit)
	}
	cur, err := c.Find(ctx, filter, findOpts)
	if err != nil {
		fatalf("find: %v", err)
	}
	defer cur.Close(ctx)

	fmt.Printf("=== inspecting %s.%s (newest first, filter=%v, limit=%d) ===\n", *dbName, *coll, filter, *limit)
	fmt.Printf("    now (local)=%s   now (UTC)=%s\n\n",
		now.Format(time.RFC3339), now.UTC().Format(time.RFC3339))

	scanned, withFuture := 0, 0
	for cur.Next(ctx) {
		scanned++
		var doc rawDoc
		if err := cur.Decode(&doc); err != nil {
			fmt.Printf("  warn: decode doc: %v\n", err)
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(doc.Rawdata)
		if err != nil {
			fmt.Printf("  warn: base64 decode %v: %v\n", doc.ID, err)
			continue
		}

		var ev parkingEvent
		if err := json.Unmarshal(decoded, &ev); err != nil {
			fmt.Printf("  warn: json unmarshal %v: %v\n  raw: %s\n", doc.ID, err, string(decoded))
			continue
		}

		// Pre-pass: any future measurements?
		hasFuture := false
		for _, m := range ev.Measurements {
			if t, perr := time.ParseInLocation(tsLayout, m.Timestamp, romeLoc); perr == nil && t.After(now) {
				hasFuture = true
				break
			}
		}
		if hasFuture {
			withFuture++
		}
		if *onlyFuture && !hasFuture {
			continue
		}

		fmt.Printf("doc _id=%v  stored=%s  provider=%s  event_id=%q  capacity=%d  measurements=%d\n",
			doc.ID, doc.Timestamp.UTC().Format(time.RFC3339), doc.Provider,
			ev.ID, ev.Capacity, len(ev.Measurements))

		for i, m := range ev.Measurements {
			rawTs := m.Timestamp
			parsedRome, err := time.ParseInLocation(tsLayout, rawTs, romeLoc)
			if err != nil {
				fmt.Printf("    [%d] raw=%q  PARSE-ERR: %v\n", i, rawTs, err)
				continue
			}
			// Also show what naive time.Parse (UTC) would have produced
			// — that's the original bug.
			parsedUTC, _ := time.Parse(tsLayout, rawTs)

			tag := ""
			if parsedRome.After(now) {
				tag = "  <-- FUTURE relative to now (Rome interpretation)"
			}
			fmt.Printf("    [%d] raw=%q  rome=%s  asUTC=%s  avail=%d%s\n",
				i, rawTs,
				parsedRome.UTC().Format(time.RFC3339),
				parsedUTC.UTC().Format(time.RFC3339),
				m.Availability, tag)
		}
		fmt.Println()
	}
	if err := cur.Err(); err != nil {
		fatalf("cursor: %v", err)
	}

	fmt.Printf("=== scanned=%d, docs containing a future measurement=%d ===\n", scanned, withFuture)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
