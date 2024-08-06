// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type mqMsg struct {
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
	Rawdata   []byte    `json:"rawdata"`
	ID        string    `json:"id"`
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
		ID:        rMsg.ID,
		Timestamp: rMsg.Timestamp,
		Provider:  fmt.Sprintf("%s/%s", rMsg.Provider, rMsg.Dataset),
		Rawdata:   rMsg.Payload,
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
