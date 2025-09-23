// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/go-timeseries-client/where"
	"github.com/noi-techpark/opendatahub-go-sdk/elab"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
)

// the timestamp of the aggregated record is the end of the AGG_PERIOD long window
func windowTs(ts time.Time) time.Time {
	return ts.Truncate(time.Second * AGGR_PERIOD).Add(time.Second * AGGR_PERIOD)
}

func aggregate(ctx context.Context, b bdplib.Bdp, n odhts.C) error {
	e := elab.NewElaboration(&n, &b)
	e.StationTypes = append(e.StationTypes, STATIONTYPE)
	e.Filter = where.Eq("sorigin", env.BDP_ORIGIN)
	e.BaseTypes = append(e.BaseTypes, elab.BaseDataType{Name: dtIn.Name, Period: BASE_PERIOD})
	e.BaseTypes = append(e.BaseTypes, elab.BaseDataType{Name: dtOut.Name, Period: BASE_PERIOD})
	e.ElaboratedTypes = append(e.ElaboratedTypes, elab.ElaboratedDataType{Name: dtCount.Name, Period: AGGR_PERIOD, DontSync: true})
	e.StartingPoint = time.Date(2025, 07, 31, 0, 0, 0, 0, time.UTC) // first records came in that day in testing

	is, err := e.RequestState()
	ms.FailOnError(ctx, err, "failed requesting initial elaboration state")

	res := []elab.ElabResult{}

	for scode, st := range is[STATIONTYPE].Stations {
		start := st.Datatypes[dtCount.Name].Periods[AGGR_PERIOD]
		if start.IsZero() {
			start = e.StartingPoint
		}
		end := time.Now().Add(-AGGR_LAG)

		measures, err := e.RequestHistory([]string{STATIONTYPE}, e.StationTypes, []string{dtIn.Name, dtOut.Name}, []elab.Period{BASE_PERIOD}, start, end)
		if err != nil {
			return fmt.Errorf("failed requesting history for count elaboration station %s from %s to %s: %w", scode, start.String(), end.String(), err)
		}

		// Create contiguous AGGR_PERIOD sized windows, then count the records for each window
		// Windows may also be empty, we still have to count them as 0
		idx := 0
		curWin := start
		for {
			curWin = curWin.Add(time.Second * AGGR_PERIOD)
			cnt := 0
			for idx < len(measures) {
				meas := measures[idx]
				win := windowTs(meas.Timestamp.Time)
				if win.Equal(curWin) {
					cnt += 1
					idx += 1
				} else if win.After(curWin) {
					// measurement belongs to one of the next windows
					break
				} else {
					return fmt.Errorf("tried to elaborate record at %s before current window %s. This should not be possible", win.String(), curWin.String())
				}
			}
			// To be sure that the data for a window is complete, the end date lags behind Now().
			// But, if we see that there are still measurements left to elaborate for the following windows,
			// we can also assume the window is complete and don't have to respect the lag
			if curWin.After(end) && idx >= len(measures) {
				break
			}
			res = append(res, elab.ElabResult{StationType: STATIONTYPE, StationCode: scode, Timestamp: start, Period: AGGR_PERIOD, DataType: dtCount.Name, Value: cnt})
		}
	}
	if err := e.PushResults(STATIONTYPE, res); err != nil {
		return fmt.Errorf("failed pushing elaboration results: %w", err)
	}
	return nil
}
