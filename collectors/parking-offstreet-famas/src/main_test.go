// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/xml"
	"testing"

	"gotest.tools/v3/assert"
)

func Test_xmlReq(t *testing.T) {
	r := XmlRpcRequest{MethodName: "testmethod"}
	r.Params = append(r.Params, XmlRpcValue{String: "stringtest"})
	r.Params = append(r.Params, XmlRpcValue{Int: 4})
	r.Params = append(r.Params, XmlRpcValue{Boolean: 1})
	r.Params = append(r.Params, XmlRpcValue{DateTime: "test1234"})
	r.Params = append(r.Params, XmlRpcValue{Base64: []byte("bytetest")})

	x, err := xml.MarshalIndent(r, "", "  ")
	assert.NilError(t, err, "error unmarshalling")

	t.Fatalf("%s", string(x))
}
