// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
	"opendatahub.com/dc-parking-offstreet-famas/xmlrpc"
)

var env struct {
	dc.Env
	CRON    string
	RPC_URL string
}

const METHOD_STATION_METADATA = "pGuide.getCaratteristicheParcheggio"
const METHOD_STATION_LIST = "pGuide.getElencoIdentificativiParcheggi"
const METHOD_OCCUPANCY = "pGuide.getPostiLiberiParcheggioExt"

type rawRec struct {
	Id   int
	Meta *xmlrpc.XmlRpcValue
	Data *xmlrpc.XmlRpcValue
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Job start")
		ctx, c := collector.StartCollection(context.Background())
		defer c.End(ctx)

		rawRecs := []rawRec{}

		req := xmlrpc.XmlRpcRequest{MethodName: METHOD_STATION_LIST}
		res, err := rpc(req)
		ms.FailOnError(ctx, err, "cannot get list of stations. aborting")

		for _, id := range res.Param.Array.Data {
			rec := rawRec{Id: *id.GetInt()}

			req := xmlrpc.XmlRpcRequest{MethodName: METHOD_STATION_METADATA}
			req.Params.Values = append(req.Params.Values, xmlrpc.XmlRpcValue{Int: id.GetInt()})
			res, err = rpc(req)
			if err != nil {
				slog.Error("error getting station metadata", "err", err, "id", id.GetInt())
			}
			if res.Fault != nil {
				rec.Meta = res.Fault
			} else {
				rec.Meta = res.Param
			}

			req = xmlrpc.XmlRpcRequest{MethodName: METHOD_OCCUPANCY}
			req.Params.Values = append(req.Params.Values, xmlrpc.XmlRpcValue{Int: id.GetInt()})
			res, err = rpc(req)
			if err != nil {
				slog.Error("error getting station occupancy", "err", err, "id", id.GetInt())
			}
			if res.Fault != nil {
				rec.Data = res.Fault
			} else {
				rec.Data = res.Param
			}

			rawRecs = append(rawRecs, rec)
		}

		if err := c.Publish(ctx, &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   rawRecs,
		}); err != nil {
			ms.FailOnError(ctx, err, "failed publishing to MQ")
		}
		slog.Info("Job complete", "records_pushed", len(rawRecs))
	})
	c.Run()
}

func rpc(req xmlrpc.XmlRpcRequest) (res xmlrpc.XmlRpcResponse, err error) {
	res, err = xmlrpc.XmlRpc(env.RPC_URL, req)
	if err != nil {
		return res, fmt.Errorf("error calling remote service: %w", err)
	}
	if res.Fault != nil {
		x, _ := xml.Marshal(res)
		return res, fmt.Errorf("xmlrpc request returned fault: %s", x)
	}
	return res, nil
}
