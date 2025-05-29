// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"log/slog"
	"net/http"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
	"golang.org/x/net/html/charset"
)

var env struct {
	dc.Env
	CRON    string
	RPC_URL string
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

		req := XmlRpcRequest{MethodName: "pGuide.getElencoIdentificativiParcheggi"}
		reqBytes, _ := xml.Marshal(req)

		httpRes, err := http.Post(env.RPC_URL, "text/xml", bytes.NewReader(reqBytes))
		ms.FailOnError(ctx, err, "failed getting list of parking areas")
		if httpRes.StatusCode != http.StatusOK {
			slog.Error("got non-OK http status code", "res", httpRes)
		}

		var res XmlRpcResponse

		dec := xml.NewDecoder(httpRes.Body)
		dec.CharsetReader = charset.NewReaderLabel
		err = dec.Decode(&res)
		ms.FailOnError(ctx, err, "failed decoding http response")

		if err := c.Publish(ctx, &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   res,
		}); err != nil {
			ms.FailOnError(ctx, err, "failed publishing to MQ")
		}
	})
	c.Run()
}

type XmlRpcRequest struct {
	XMLName    xml.Name      `xml:"methodCall"`
	MethodName string        `xml:"methodName"`
	Params     []XmlRpcValue `xml:"params>param>value"`
}

type XmlRpcValue struct {
	Int      int64         `xml:"int,omitempty"`
	I4       int64         `xml:"i4,omitempty"`
	Boolean  uint8         `xml:"boolean,omitempty"`
	String   string        `xml:"string,omitempty"`
	DateTime string        `xml:"dateTime.iso8601,omitempty"`
	Base64   []byte        `xml:"base64,omitempty"`
	Struct   *XmlRpcStruct `xml:"struct,omitempty"`
	Array    *XmlRpcArray  `xml:"array,omitempty"`
}

type XmlRpcStruct struct {
	XmlName xml.Name `xml:"struct"`
	members []XmlRpcStructMember
}
type XmlRpcStructMember struct {
	Name  string
	Value XmlRpcValue
}

type XmlRpcArray struct {
	XmlName xml.Name      `xml:"array"`
	Data    []XmlRpcValue `xml:"data>value"`
}

type XmlRpcResponse struct {
	XMLName xml.Name    `xml:"methodResponse"`
	Param   XmlRpcValue `xml:"params>param>value"`
	Fault   XmlRpcValue `xml:"fault>value"`
}
