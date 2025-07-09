// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/xml"
	"os"
	"testing"

	"golang.org/x/net/html/charset"
	"gotest.tools/v3/assert"
	"opendatahub.com/dc-parking-offstreet-famas/xmlrpc"
)

func Test_xmlReq(t *testing.T) {
	r := xmlrpc.XmlRpcRequest{MethodName: METHOD_STATION_LIST}

	x, err := xml.Marshal(r)
	assert.NilError(t, err, "error marshalling")

	f, _ := os.ReadFile("./testdata/listrequest.xml")
	assert.Equal(t, string(f), string(x))

	r = xmlrpc.XmlRpcRequest{MethodName: METHOD_STATION_METADATA}
	r.Params.Values = append(r.Params.Values, xmlrpc.XmlRpcValue{Int: xmlrpc.Pt(105)})

	x, err = xml.Marshal(r)
	assert.NilError(t, err, "error marshalling")

	f, _ = os.ReadFile("./testdata/metarequest.xml")
	assert.Equal(t, string(f), string(x))
}

func Test_Unmarshal(t *testing.T) {
	// test input obtained by using calls.http

	res := unmarshal(t, "./testdata/listresponse.xml")
	assert.Equal(t, *res.Param.Array.Data[0].Int, 103)
	assert.Equal(t, *res.Param.Array.Data[1].Int, 104)
	assert.Equal(t, len(res.Param.Array.Data), 8)

	res = unmarshal(t, "./testdata/metaresponse.xml")
	assert.Equal(t, *res.Param.Array.Data[0].Int, 105)
	assert.Equal(t, *res.Param.Array.Data[1].StringRaw, "P05 - Laurin")
	assert.Equal(t, *res.Param.Array.Data[2].Int, 90)

	res = unmarshal(t, "./testdata/occupancyresponse.xml")
	assert.Equal(t, *res.Param.Array.Data[0].StringRaw, "P05 - Laurin")
	assert.Equal(t, *res.Param.Array.Data[1].Int, 0)
	assert.Equal(t, len(res.Param.Array.Data), 15)

	res = unmarshal(t, "./testdata/faultresponse.xml")
	assert.Equal(t, *res.Fault.Struct.Members[0].Value.I4, 0)
}

func unmarshal(t *testing.T, input string) (res xmlrpc.XmlRpcResponse) {
	f, err := os.ReadFile(input)
	assert.NilError(t, err, "failed reading input file")
	dec := xml.NewDecoder(bytes.NewReader(f))
	dec.CharsetReader = charset.NewReaderLabel
	err = dec.Decode(&res)
	assert.NilError(t, err, "failed unmarshalling")
	return res
}
