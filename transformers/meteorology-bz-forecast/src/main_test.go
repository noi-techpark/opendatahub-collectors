// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

func TestTransformation(t *testing.T) {
	var in = Forecast{}
	err := bdpmock.LoadInputData(&in, "../testdata/input/SMOS_MCPL-WX_EXP_SIAG.json")
	require.Nil(t, err)

	var out = bdpmock.BdpMockCalls{}
	err = bdpmock.LoadOutput(&out, "../testdata/output/SMOS_MCPL-WX_EXP_SIAG--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	raw := dto.Raw[Forecast]{
		Rawdata: in,
	}

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	assert.DeepEqual(t, mock.Requests(), out)
}
func TestDatatypes(t *testing.T) {
	var out = bdpmock.BdpMockCalls{}
	err := bdpmock.LoadOutput(&out, "../testdata/output/DATATYPES--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	dataTypeList := bdplib.NewDataTypeList(nil)
	err = dataTypeList.Load("datatypes.json")
	require.Nil(t, err)

	b.SyncDataTypes(OriginStationType, dataTypeList.All())

	mock := b.(*bdpmock.BdpMock)

	assert.DeepEqual(t, mock.Requests(), out)
}
