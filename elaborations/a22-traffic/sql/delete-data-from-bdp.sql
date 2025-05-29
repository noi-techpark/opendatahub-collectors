UPDATE measurement m
SET timestamp = '2024-07-09 23:59:59'
FROM station s, "type" t
WHERE m.station_id = s.id
  AND m.type_id = t.id
  AND m.timestamp > '2024-07-10'
  AND m.period = 600
  AND s.stationtype = 'TrafficSensor'
  AND s.origin = 'A22'
  AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  );

---
select m.*, t.*, s.*
from station s
join measurement m on s.id = m.station_id
join type t on t.id = m.type_id
AND m.period = 600
  AND s.stationtype = 'TrafficSensor'
  AND s.origin = 'A22'
  AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  )
order by m.timestamp desc
limit 1000;

----------- Delete procedure for measurement history TO EXECUTE IN A SCRIPT SHELL
DO $$
DECLARE
    station_rec RECORD;
    station_count INT := 0;
    rows_deleted INT := 0;
    total_deleted INT := 0;
BEGIN
    FOR station_rec IN
        SELECT s.id AS station_id
        FROM station s
        WHERE s.stationtype = 'TrafficSensor'
          AND s.origin = 'A22'
    LOOP
        station_count := station_count + 1;
        RAISE INFO 'Deleting for station %', station_rec.station_id;

        DELETE FROM measurementhistory m
        USING "type" t
        WHERE m.station_id = station_rec.station_id
          AND m.type_id = t.id
          AND m.timestamp > '2024-07-10'
          AND m.period = 600
          AND t.cname IN (
            'Nr. Light Vehicles', 
            'Nr. Heavy Vehicles', 
            'Nr. Buses', 
            'Nr. Equivalent Vehicles', 
            'Average Speed Light Vehicles', 
            'Average Speed Heavy Vehicles', 
            'Average Speed Buses', 
            'Variance Speed Light Vehicles', 
            'Variance Speed Heavy Vehicles', 
            'Variance Speed Buses', 
            'Average Gap', 
            'Average Headway', 
            'Average Density', 
            'Average Flow', 
            'Euro Emission Standard', 
            'Vehicle Count by Nationality'
          );

        GET DIAGNOSTICS rows_deleted = ROW_COUNT;
        total_deleted := total_deleted + rows_deleted;

        RAISE INFO 'Station % (%): deleted % rows', station_rec.station_id, station_count, rows_deleted;

        COMMIT AND CHAIN;
    END LOOP;

    RAISE INFO 'Finished. % stations processed, % rows deleted total.', station_count, total_deleted;
END;
$$;
------------

select m.*, t.*, s.*
from station s
join measurementhistory m on s.id = m.station_id
join type t on t.id = m.type_id
AND m.period = 600
  AND s.stationtype = 'TrafficSensor'
  AND s.origin = 'A22'
  AND m.timestamp > '2025-05-20'
  ;

------------------------------------------- JSON
UPDATE measurementjson m
SET timestamp = '2024-07-09 23:59:59'
FROM station s, "type" t
WHERE m.station_id = s.id
  AND m.type_id = t.id
  AND m.timestamp > '2024-07-10'
  AND m.period = 600
  AND s.stationtype = 'TrafficSensor'
  AND s.origin = 'A22'
  AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  );

---
select m.*, t.*, s.*
from station s
join measurementjson m on s.id = m.station_id
join type t on t.id = m.type_id
AND m.period = 600
  AND s.stationtype = 'TrafficSensor'
  AND s.origin = 'A22'
  AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  )
order by m.timestamp desc
limit 1000;

DELETE FROM measurementjsonhistory m
USING station s, "type" t
WHERE m.station_id = s.id
  AND m.type_id = t.id
  AND m.timestamp > '2024-07-10'
  AND s.stationtype = 'TrafficSensor'
  AND s.origin = 'A22'
  and m."period" = 600
  AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  );

---------------------------------------------
select count(*)
from station s
join measurementhistory m on s.id = m.station_id
join type t on t.id = m.type_id
where stationtype = 'TrafficSensor'
and s.origin = 'A22'
AND m.timestamp > '2024-07-10'
and m."period" = 600
AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  );

select count(*)
from station s
join measurementjsonhistory m on s.id = m.station_id
join type t on t.id = m.type_id
where stationtype = 'TrafficSensor'
and s.origin = 'A22'
AND m.timestamp > '2024-07-10'
and m."period" = 600
AND t.cname IN (
    'Nr. Light Vehicles', 
    'Nr. Heavy Vehicles', 
    'Nr. Buses', 
    'Nr. Equivalent Vehicles', 
    'Average Speed Light Vehicles', 
    'Average Speed Heavy Vehicles', 
    'Average Speed Buses', 
    'Variance Speed Light Vehicles', 
    'Variance Speed Heavy Vehicles', 
    'Variance Speed Buses', 
    'Average Gap', 
    'Average Headway', 
    'Average Density', 
    'Average Flow', 
    'Euro Emission Standard', 
    'Vehicle Count by Nationality'
  );

