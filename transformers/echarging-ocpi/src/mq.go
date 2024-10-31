// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type mqMsg struct {
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
	Rawdata   any       `json:"rawdata"`
}

type RabbitC struct {
	Con *amqp.Connection
	Ch  *amqp.Channel
}

func (r *RabbitC) Close() {
	if r.Ch != nil && !r.Ch.IsClosed() {
		_ = r.Ch.Close()
	}
	if r.Con != nil && !r.Con.IsClosed() {
		_ = r.Con.Close()
	}
}

func (r *RabbitC) OnClose(handler func(amqp.Error)) {
	r.Con.NotifyClose(func() chan *amqp.Error {
		notifyClose := make(chan *amqp.Error)
		go func() {
			err := <-notifyClose
			handler(*err)
		}()
		return notifyClose
	}())
}

func RabbitConnect(url string) (RabbitC, error) {
	r := RabbitC{}
	con, err := amqp.Dial(url)
	if err != nil {
		return r, err
	}

	ch, err := con.Channel()
	if err != nil {
		return r, err
	}

	r.Ch = ch
	r.Con = con

	return r, nil
}

func (r *RabbitC) Publish(msg mqMsg, exchange string) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshalling message to json: %w", err)
	}

	err = r.Ch.Publish(
		exchange,     // exchange
		msg.Provider, // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
			Headers:     amqp.Table{"provider": msg.Provider},
		})

	if err != nil {
		return fmt.Errorf("error sending amqp msg: %w", err)
	}
	return nil
}

func (r *RabbitC) Consume(exchange string, queue string, routingkey string, consumer string) (<-chan amqp.Delivery, error) {
	q, err := r.Ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue %s: %w", queue, err)
	}
	err = r.Ch.QueueBind(q.Name, routingkey, exchange, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed binding queue %s: %w", queue, err)
	}
	mq, err := r.Ch.Consume(q.Name, consumer, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed registering consumer for queue %s: %w", queue, err)
	}
	return mq, nil
}
