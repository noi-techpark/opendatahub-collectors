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
	Meta []any
	Data []any
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		ctx, c := collector.StartCollection(context.Background())
		defer c.End(ctx)

		rawRecs := []rawRec{}

		req := xmlrpc.XmlRpcRequest{MethodName: METHOD_STATION_LIST}
		res, err := rpc(req)
		ms.FailOnError(ctx, err, "cannot get list of stations. aborting")

		for _, id := range res.Param.Array.Data {
			rec := rawRec{Id: *id.Int}

			req := xmlrpc.XmlRpcRequest{MethodName: METHOD_STATION_METADATA}
			req.Params.Values = append(req.Params.Values, xmlrpc.XmlRpcValue{Int: id.Int})
			res, err = rpc(req)
			if err != nil {
				slog.Error("error getting station metadata", "err", err, "id", id.Int)
			}
			rec.Meta = ar2list(*res.Param.Array)

			req = xmlrpc.XmlRpcRequest{MethodName: METHOD_OCCUPANCY}
			req.Params.Values = append(req.Params.Values, xmlrpc.XmlRpcValue{Int: id.Int})
			res, err = rpc(req)
			if err != nil {
				slog.Error("error getting station occupancy", "err", err, "id", id.Int)
			}
			rec.Data = ar2list(*res.Param.Array)

			rawRecs = append(rawRecs, rec)
		}

		if err := c.Publish(ctx, &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   rawRecs,
		}); err != nil {
			ms.FailOnError(ctx, err, "failed publishing to MQ")
		}
	})
	c.Run()
}

func anyVal(v xmlrpc.XmlRpcValue) any {
	if v.Int != nil {
		return *v.Int
	}
	if v.I4 != nil {
		return *v.I4
	}
	if v.Double != nil {
		return *v.Double
	}
	if v.Boolean != nil {
		return *v.Boolean
	}
	if v.String != nil {
		return *v.String
	}
	if v.DateTime != nil {
		return *v.DateTime
	}
	if v.Base64 != nil {
		return *v.Base64
	}
	if v.Array != nil {
		return *v.Array
	}
	if v.Struct != nil {
		return *v.Struct
	}
	if v.StringRaw != nil {
		return *v.StringRaw
	}
	return nil
}

func ar2list(ar xmlrpc.XmlRpcArray) (ret []any) {
	for _, e := range ar.Data {
		ret = append(ret, anyVal(e))
	}
	return ret
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
