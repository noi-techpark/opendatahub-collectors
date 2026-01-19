// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package xmlrpc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/net/html/charset"
)

type XmlRpcRequest struct {
	XMLName    xml.Name            `xml:"methodCall"`
	MethodName string              `xml:"methodName"`
	Params     XmlRpcRequestParams `xml:"params"`
}

type XmlRpcRequestParams struct {
	Values []XmlRpcValue `xml:"param>value,omitempty"`
}

// Ensure empty "params" tag always present
func (r XmlRpcRequestParams) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	params := xml.StartElement{Name: xml.Name{Local: "params"}}
	e.EncodeToken(params)
	for _, v := range r.Values {
		param := xml.StartElement{Name: xml.Name{Local: "param"}}
		e.EncodeToken(param)
		value := xml.StartElement{Name: xml.Name{Local: "value"}}

		if err := e.EncodeElement(v, value); err != nil {
			return err
		}

		e.EncodeToken(param.End())
	}

	return e.EncodeToken(params.End())
}

func (r XmlRpcRequest) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if err := e.EncodeToken(xml.ProcInst{Target: "xml", Inst: []byte(`version="1.0" encoding="UTF-8"`)}); err != nil {
		return err
	}
	type W XmlRpcRequest
	return e.Encode(W(r))
}

type XmlRpcValue struct {
	Int       *int          `xml:"int,omitempty" json:",omitempty"`
	I4        *int          `xml:"i4,omitempty" json:",omitempty"`
	Double    *float64      `xml:"double,omitempty" json:",omitempty"`
	Boolean   *uint8        `xml:"boolean,omitempty" json:",omitempty"`
	String    *string       `xml:"string,omitempty" json:",omitempty"`
	DateTime  *string       `xml:"dateTime.iso8601,omitempty" json:",omitempty"`
	Base64    *[]byte       `xml:"base64,omitempty" json:",omitempty"`
	Struct    *XmlRpcStruct `xml:"struct,omitempty" json:",omitempty"`
	Array     *XmlRpcArray  `xml:"array,omitempty" json:",omitempty"`
	StringRaw *string       `xml:",chardata" json:",omitempty"`
}

func (v *XmlRpcValue) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type Alias XmlRpcValue
	aux := (*Alias)(v)

	if err := d.DecodeElement(aux, &start); err != nil {
		return err
	}

	// Only set StringRaw if no other field was populated
	if v.Int == nil && v.I4 == nil && v.Double == nil &&
		v.Boolean == nil && v.String == nil && v.DateTime == nil &&
		v.Base64 == nil && v.Struct == nil && v.Array == nil &&
		*v.StringRaw != "" {
		raw := strings.TrimSpace(*v.StringRaw)
		v.StringRaw = &raw
	} else {
		v.StringRaw = nil
	}

	return nil
}

type XmlRpcStruct struct {
	Members []XmlRpcStructMember `xml:"member"`
}
type XmlRpcStructMember struct {
	Name  string      `xml:"name"`
	Value XmlRpcValue `xml:"value"`
}

type XmlRpcArray struct {
	Data []XmlRpcValue `xml:"data>value"`
}

type XmlRpcResponse struct {
	XMLName xml.Name     `xml:"methodResponse"`
	Param   *XmlRpcValue `xml:"params>param>value"`
	Fault   *XmlRpcValue `xml:"fault>value"`
}

func XmlRpc(url string, req XmlRpcRequest) (ret XmlRpcResponse, err error) {
	reqBytes, err := xml.Marshal(req)
	if err != nil {
		return
	}
	slog.Debug("Dumping request xml", "req", string(reqBytes))
	httpRes, err := http.Post(url, "text/xml", bytes.NewReader(reqBytes))
	if err != nil {
		return
	}
	if httpRes.StatusCode != http.StatusOK {
		return ret, fmt.Errorf("non-ok http status: %d", httpRes.StatusCode)
	}
	respBody, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return
	}
	slog.Debug("Dumping response xml", "body", string(respBody))
	dec := xml.NewDecoder(bytes.NewReader(respBody))
	dec.CharsetReader = charset.NewReaderLabel
	err = dec.Decode(&ret)
	return
}

func Pt[T any](t T) *T {
	return &t
}

// ints can either be called Int or I4
func (v XmlRpcValue) GetInt() *int {
	if v.Int != nil {
		return v.Int
	}
	return v.I4
}

// strings can be raw or within a value tag
func (v XmlRpcValue) GetString() *string {
	if v.String != nil {
		return v.String
	}
	return v.StringRaw
}
