// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Window struct {
	scode string
	start time.Time
}

type RangeCounter struct {
	rdb       *redis.Client
	ctx       context.Context
	vkPrefix  string
	aggWindow time.Duration
}

func (ea *RangeCounter) flush(scode string, ts time.Time) error {
	startKey := ea.vkPrefix + ":" + scode + ":winstart"

	newWindow := ts.Truncate(ea.aggWindow)
	lastWindow, err := ea.rdb.Get(ea.ctx, startKey).Time()
	if err == redis.Nil {
		lastWindow = newWindow
	} else if err != nil {
		return err
	}

	for lastWindow.Before(newWindow) {
		lastWindow = lastWindow.Add(ea.aggWindow)
	}
	return nil
}

func (ea *RangeCounter) HandleRec(scode string, ts time.Time) error {
	if err := ea.flush(scode, ts); err != nil {
		return err
	}
	if err := ea.add(scode, ts); err != nil {
		return err
	}
	return nil
}

func (ea *RangeCounter) add(scode string, ts time.Time) error {

	countKey := ea.vkPrefix + ":" + scode + ":count"

	_, err := ea.rdb.Incr(ea.ctx, countKey).Result()
	if err != nil {
		return err
	}

	return nil
}
