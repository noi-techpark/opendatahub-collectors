-- SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
--
-- SPDX-License-Identifier: CC0-1.0

-- parking-ch: remove the BikeParking stations that were created with an exponential
-- station code ("SBB:8.506303e+06", float64 formatted with %v) and their metadata.
-- The correct stations ("SBB:8506303") exist already; the old ones have no timeseries.
--
-- Only origin 'SBB' stations with an exponential code AND active = false are selected.
-- station.meta_data_id and metadata.station_id reference each other, so the link on
-- the station side is cleared first, then the metadata, then the stations.
--
-- Everything runs in one transaction that is NOT committed by this script:
-- review the output and then type COMMIT (or ROLLBACK).
--   psql -f cleanup-exponential-station-codes.sql

set search_path = intimev2, public;

begin;

-- 1. stations to delete --------------------------------------------------------------
create temporary table tmp_old_station on commit drop as
select o.id as old_id, n.id as new_id, o.stationcode as old_code, n.stationcode as new_code,
       o.stationtype, o.active as old_active, n.active as new_active
from station o
left join station n
  on  n.origin      = o.origin
  and n.stationtype = o.stationtype
  and n.id         <> o.id
  and n.stationcode = 'SBB:' || (substr(o.stationcode, 5)::numeric::bigint)::text
where o.origin = 'SBB'
  and o.stationcode ~ '^SBB:[0-9](\.[0-9]+)?e\+[0-9]+$'
  and o.active = false;   -- NULL or true is never selected, hence never deleted

-- 2. preview / sanity checks ---------------------------------------------------------
select stationtype, count(*) as to_delete, count(new_id) as with_new_station,
       count(*) filter (where new_active) as new_station_active
from tmp_old_station group by 1;

-- exponential stations NOT selected because they are not inactive (expected: none)
select id, stationcode, stationtype, active from station
where origin = 'SBB' and stationcode ~ '^SBB:[0-9](\.[0-9]+)?e\+[0-9]+$'
  and active is distinct from false;

-- what else hangs off the selected stations (expected: 0 for everything except metadata)
select 'timeseries' as what, count(*) from timeseries t join tmp_old_station s on t.station_id = s.old_id
union all select 'edges', count(*) from edge e join tmp_old_station s on s.old_id in (e.origin_id, e.destination_id, e.edge_data_id)
union all select 'child stations', count(*) from station c join tmp_old_station s on c.parent_id = s.old_id
union all select 'metadata rows', count(*) from metadata m join tmp_old_station s on m.station_id = s.old_id
union all select 'metadata used by other stations', count(*) from station o
  join metadata m on m.id = o.meta_data_id join tmp_old_station s on m.station_id = s.old_id
  where o.id <> s.old_id
union all select 'metadata used by events', count(*) from event ev
  join metadata m on m.id = ev.meta_data_id join tmp_old_station s on m.station_id = s.old_id;

-- 3. abort unless the selection is clean -------------------------------------------------
DO $$
DECLARE
  v_count int;
BEGIN
  SELECT count(*) INTO v_count FROM tmp_old_station WHERE new_id IS NULL OR new_active IS NOT TRUE;
  IF v_count > 0 THEN
    RAISE EXCEPTION 'abort: % old station(s) have no active station with the integer code', v_count;
  END IF;

  SELECT count(*) INTO v_count FROM timeseries t JOIN tmp_old_station s ON t.station_id = s.old_id;
  IF v_count > 0 THEN
    RAISE EXCEPTION 'abort: % timeseries exist on the old stations, they would be orphaned', v_count;
  END IF;

  RAISE NOTICE 'safety checks passed';
END
$$;

-- 4. delete -----------------------------------------------------------------------------------
-- break the reciprocal reference: station.meta_data_id -> metadata, metadata.station_id -> station
update station st set meta_data_id = null
from tmp_old_station s where st.id = s.old_id;

delete from metadata m using tmp_old_station s where m.station_id = s.old_id;

delete from station st using tmp_old_station s where st.id = s.old_id;

-- expected: 0 / 0
select (select count(*) from station st join tmp_old_station s on s.old_id = st.id) as remaining_stations,
       (select count(*) from metadata m join tmp_old_station s on m.station_id = s.old_id) as remaining_metadata;

-- review the output above, then COMMIT; (or ROLLBACK;)
