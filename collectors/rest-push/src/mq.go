// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type mqMsg struct {
	Provider    string    `json:"provider"`
	Timestamp   time.Time `json:"timestamp"`
	Rawdata     []byte    `json:"rawdata"`
	ID          string    `json:"id"`
	ContentType string    `json:"content_type,omitempty"`

	// Query carries the request's query parameters. MarshalJSON writes them
	// into a `meta` sub-document, which is where the raw data bridge looks for
	// anything it can group on — nothing inside `rawdata` is reachable once the
	// payload is stored as a string or as binary, and the document root belongs
	// to the pipeline.
	//
	// A single-valued parameter becomes a scalar. A repeated parameter stays an
	// array, which is faithful but not groupable — send it once to group on it.
	Query map[string]any `json:"-"`
}

// metaField is the sub-document publisher-supplied identity is written to.
//
// It has to match what raw-writer-2 harvests X-OpenDataHub-* headers into and
// what the raw data bridge groups on, because a consumer asking for
// `compacted/key` gets `meta.key` whichever collector published the document.
const metaField = "meta"

// MarshalJSON writes the pipeline's own fields at the root and the caller's
// query parameters under `meta`.
//
// Nesting is what makes them usable: the bridge groups on `meta.<field>` and
// cannot reach the document root, so a parameter written there would be stored
// faithfully and be invisible to every reference table. Nesting also removes
// any question of a parameter colliding with `provider` or `timestamp` — it
// simply cannot reach them.
func (m mqMsg) MarshalJSON() ([]byte, error) {
	out := map[string]any{
		"provider":  m.Provider,
		"timestamp": m.Timestamp,
		"rawdata":   m.Rawdata,
		"id":        m.ID,
	}
	if m.ContentType != "" {
		out["content_type"] = m.ContentType
	}
	if len(m.Query) > 0 {
		out[metaField] = m.Query
	}
	return json.Marshal(out)
}

// flattenQuery prepares query parameters for storage, collapsing the
// single-value case so the field is a scalar the database can group on.
func flattenQuery(q map[string][]string) map[string]any {
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]any, len(q))
	for k, v := range q {
		// Lowercased to match raw-writer-2, which lowercases the remainder of
		// an X-OpenDataHub-* header name. Without this, `?Key=A` and
		// `X-OpenDataHub-Key: A` would produce different fields and a consumer
		// would have to know which collector wrote the document.
		k = strings.ToLower(k)
		switch len(v) {
		case 0:
			continue
		case 1:
			out[k] = v[0]
		default:
			out[k] = v
		}
	}
	return out
}

type restMsg struct {
	Provider    string
	Dataset     string
	ID          string
	Timestamp   time.Time
	Query       map[string][]string
	ContentType string
	Payload     []byte
	Response    chan bool `json:"-"`
}

type rCon struct {
	con *amqp.Connection
	ch  *amqp.Channel
}

func (r *rCon) connect(url string) error {
	con, err := amqp.Dial(url)
	if err != nil {
		return err
	}

	ch, err := con.Channel()
	if err != nil {
		return err
	}

	r.ch = ch
	r.con = con

	return nil
}

func fromRest(rMsg restMsg) mqMsg {
	return mqMsg{
		ID:          rMsg.ID,
		Timestamp:   rMsg.Timestamp,
		Provider:    fmt.Sprintf("%s/%s", rMsg.Provider, rMsg.Dataset),
		Rawdata:     rMsg.Payload,
		ContentType: rMsg.ContentType,
		Query:       flattenQuery(rMsg.Query),
	}
}

func InitRabbitMq(msgQ <-chan restMsg) {
	r := new(rCon)
	conErr := make(chan *amqp.Error) // when the connection drops, we get a message on this channel

	go func() {
		for {
			select {
			// connect to rabbitmq
			case e := <-conErr:
				if e != nil {
					slog.Error("Rabbit connection dropped.", "closeErr", e)
				}
				retry := 0
				for {
					err := r.connect(Config.RabbitURL)
					if err != nil {
						retry++
						slog.Error("Error establishing Rabbitmq connection", "err", err)
						if retry < 5 {
							time.Sleep(time.Second * 5)
						} else {
							slog.Error("Exhausted connection retries. aborting")
							panic("Unable to connect to rabbitmq")
						}
					} else {
						slog.Info("Connection to rabbitmq established")
						conErr = make(chan *amqp.Error)
						r.ch.NotifyClose(conErr)
						break
					}
				}

			// Handle incoming message
			case rMsg := <-msgQ:
				msg := fromRest(rMsg)
				payload, err := json.Marshal(msg)
				if err != nil {
					slog.Error("Error marshalling message to json", "err", err, "UID", msg.ID)
				}

				err = func() error {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					return r.ch.PublishWithContext(ctx,
						"ingress",    // exchange
						msg.Provider, // routing key
						false,        // mandatory
						false,        // immediate
						amqp.Publishing{
							ContentType: "application/json",
							Body:        payload,
							Headers:     amqp.Table{"provider": msg.Provider},
						})
				}()
				if err != nil {
					slog.Error("Error sending amqp msg", "err", err, "UID", rMsg.ID)
					rMsg.Response <- false
				} else {
					rMsg.Response <- true
				}
			}
		}
	}()

	conErr <- nil // force initial connect with rabbitmq
}
