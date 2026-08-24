@rcavaliere  i now get what `current-station` means and the fact that cars are "stationary" in a CarSharingStations reframes the situation.

I will add `future-availability-180` timeseries to meet the bussiness requirement.
For all the "CarSharingStations related" timeseries, all the proposed timeseries are expressible with 10-15 line code in the client.
I will provide the snipets and i strongly suggest to propose this solution to the partner since it is feasible, easy and fast.

## 1. `current-vehicles` — all stations

```python
import requests
from datetime import datetime, timedelta, timezone
from collections import defaultdict

FRESH = (datetime.now(timezone.utc) - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")  # ignore retired cars
URL = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar/current-station,availability/latest"
Q = {"where": f"and(sorigin.eq.AlpsGo,mvalidtime.gt.{FRESH})", "select": "scode,tname,mvalue", "limit": -1}
rows = requests.get(URL, params=Q).json()["data"]

cars = defaultdict(dict)
for r in rows: cars[r["scode"]][r["tname"]] = r["mvalue"]           # one entry per car

current_vehicles = defaultdict(list)                                 # station -> available car ids
for code, m in cars.items():
    if m.get("availability") == 1 and m.get("current-station", "0") != "0":
        current_vehicles[m["current-station"]].append(code)
```

```js
const FRESH = new Date(Date.now() - 3600e3).toISOString().slice(0, 19) + "Z";  // ignore retired cars
const URL = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar/current-station,availability/latest"
          + `?where=and(sorigin.eq.AlpsGo,mvalidtime.gt.${FRESH})&select=scode,tname,mvalue&limit=-1`;
const rows = (await (await fetch(URL)).json()).data;

const cars = {};                                                     // one entry per car
for (const r of rows) (cars[r.scode] ??= {})[r.tname] = r.mvalue;

const currentVehicles = {};                                          // station -> available car ids
for (const [code, m] of Object.entries(cars))
  if (m.availability === 1 && m["current-station"] !== "0")
    (currentVehicles[m["current-station"]] ??= []).push(code);
```

## 2. `current-vehicles-3hours` — all stations

```python
import requests
from datetime import datetime, timedelta, timezone
from collections import defaultdict

FRESH = (datetime.now(timezone.utc) - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")  # ignore retired cars
URL = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar/current-station,future-availability-180/latest"
Q = {"where": f"and(sorigin.eq.AlpsGo,mvalidtime.gt.{FRESH})", "select": "scode,tname,mvalue", "limit": -1}
rows = requests.get(URL, params=Q).json()["data"]

cars = defaultdict(dict)
for r in rows: cars[r["scode"]][r["tname"]] = r["mvalue"]           # one entry per car

current_vehicles_3h = defaultdict(list)                              # station -> car ids free in 3h
for code, m in cars.items():
    if m.get("future-availability-180") == 1 and m.get("current-station", "0") != "0":
        current_vehicles_3h[m["current-station"]].append(code)
```

```js
const FRESH = new Date(Date.now() - 3600e3).toISOString().slice(0, 19) + "Z";  // ignore retired cars
const URL = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar/current-station,future-availability-180/latest"
          + `?where=and(sorigin.eq.AlpsGo,mvalidtime.gt.${FRESH})&select=scode,tname,mvalue&limit=-1`;
const rows = (await (await fetch(URL)).json()).data;

const cars = {};                                                     // one entry per car
for (const r of rows) (cars[r.scode] ??= {})[r.tname] = r.mvalue;

const currentVehicles3h = {};                                        // station -> car ids free in 3h
for (const [code, m] of Object.entries(cars))
  if (m["future-availability-180"] === 1 && m["current-station"] !== "0")
    (currentVehicles3h[m["current-station"]] ??= []).push(code);
```

## 3. `number-available-3hours` — all stations

```python
import requests
from datetime import datetime, timedelta, timezone
from collections import defaultdict

FRESH = (datetime.now(timezone.utc) - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")  # ignore retired cars
URL = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar/current-station,future-availability-180/latest"
Q = {"where": f"and(sorigin.eq.AlpsGo,mvalidtime.gt.{FRESH})", "select": "scode,tname,mvalue", "limit": -1}
rows = requests.get(URL, params=Q).json()["data"]

cars = defaultdict(dict)
for r in rows: cars[r["scode"]][r["tname"]] = r["mvalue"]           # one entry per car

number_available_3h = defaultdict(int)                               # station -> how many free in 3h
for m in cars.values():
    if m.get("future-availability-180") == 1 and m.get("current-station", "0") != "0":
        number_available_3h[m["current-station"]] += 1
```

```js
const FRESH = new Date(Date.now() - 3600e3).toISOString().slice(0, 19) + "Z";  // ignore retired cars
const URL = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar/current-station,future-availability-180/latest"
          + `?where=and(sorigin.eq.AlpsGo,mvalidtime.gt.${FRESH})&select=scode,tname,mvalue&limit=-1`;
const rows = (await (await fetch(URL)).json()).data;

const cars = {};                                                     // one entry per car
for (const r of rows) (cars[r.scode] ??= {})[r.tname] = r.mvalue;

const numberAvailable3h = {};                                        // station -> how many free in 3h
for (const m of Object.values(cars))
  if (m["future-availability-180"] === 1 && m["current-station"] !== "0")
    numberAvailable3h[m["current-station"]] = (numberAvailable3h[m["current-station"]] ?? 0) + 1;
```

## 4. `current-vehicles` — one station

```python
import requests
from datetime import datetime, timedelta, timezone

API = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar"
STATION = "1092945197"
FRESH = (datetime.now(timezone.utc) - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")  # ignore retired cars

codes = [r["scode"] for r in requests.get(f"{API}/current-station/latest",                        # cars based here
    params={"where": f'and(sorigin.eq.AlpsGo,mvalue.eq."{STATION}",mvalidtime.gt.{FRESH})',
            "select": "scode", "limit": -1}).json()["data"]]

quoted = ",".join(f'"{c}"' for c in codes)                                                        # scode is a string column
rows = requests.get(f"{API}/availability/latest",
    params={"where": f"and(sorigin.eq.AlpsGo,scode.in.({quoted}),mvalidtime.gt.{FRESH})",
            "select": "scode,mvalue", "limit": -1}).json()["data"]

current_vehicles = [r["scode"] for r in rows if r["mvalue"] == 1]                                  # keep the available ones
```

```js
const API = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar", STATION = "1092945197";
const FRESH = new Date(Date.now() - 3600e3).toISOString().slice(0, 19) + "Z";  // ignore retired cars
const get = async (u) => (await (await fetch(u)).json()).data;

const codes = (await get(`${API}/current-station/latest`                                          // cars based here
  + `?where=and(sorigin.eq.AlpsGo,mvalue.eq.${encodeURIComponent(`"${STATION}"`)},mvalidtime.gt.${FRESH})`
  + `&select=scode&limit=-1`)).map(r => r.scode);

const quoted = encodeURIComponent(codes.map(c => `"${c}"`).join(","));                            // scode is a string column
const rows = await get(`${API}/availability/latest`
  + `?where=and(sorigin.eq.AlpsGo,scode.in.(${quoted}),mvalidtime.gt.${FRESH})&select=scode,mvalue&limit=-1`);

const currentVehicles = rows.filter(r => r.mvalue === 1).map(r => r.scode);                        // keep the available ones
```

## 5. `current-vehicles-3hours` — one station

```python
import requests
from datetime import datetime, timedelta, timezone

API = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar"
STATION = "1092945197"
FRESH = (datetime.now(timezone.utc) - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")  # ignore retired cars

codes = [r["scode"] for r in requests.get(f"{API}/current-station/latest",                        # cars based here
    params={"where": f'and(sorigin.eq.AlpsGo,mvalue.eq."{STATION}",mvalidtime.gt.{FRESH})',
            "select": "scode", "limit": -1}).json()["data"]]

quoted = ",".join(f'"{c}"' for c in codes)                                                        # scode is a string column
rows = requests.get(f"{API}/future-availability-180/latest",
    params={"where": f"and(sorigin.eq.AlpsGo,scode.in.({quoted}),mvalidtime.gt.{FRESH})",
            "select": "scode,mvalue", "limit": -1}).json()["data"]

current_vehicles_3h = [r["scode"] for r in rows if r["mvalue"] == 1]                               # keep the ones free in 3h
```

```js
const API = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar", STATION = "1092945197";
const FRESH = new Date(Date.now() - 3600e3).toISOString().slice(0, 19) + "Z";  // ignore retired cars
const get = async (u) => (await (await fetch(u)).json()).data;

const codes = (await get(`${API}/current-station/latest`                                          // cars based here
  + `?where=and(sorigin.eq.AlpsGo,mvalue.eq.${encodeURIComponent(`"${STATION}"`)},mvalidtime.gt.${FRESH})`
  + `&select=scode&limit=-1`)).map(r => r.scode);

const quoted = encodeURIComponent(codes.map(c => `"${c}"`).join(","));                            // scode is a string column
const rows = await get(`${API}/future-availability-180/latest`
  + `?where=and(sorigin.eq.AlpsGo,scode.in.(${quoted}),mvalidtime.gt.${FRESH})&select=scode,mvalue&limit=-1`);

const currentVehicles3h = rows.filter(r => r.mvalue === 1).map(r => r.scode);                      // keep the ones free in 3h
```

## 6. `number-available-3hours` — one station

```python
import requests
from datetime import datetime, timedelta, timezone

API = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar"
STATION = "1092945197"
FRESH = (datetime.now(timezone.utc) - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")  # ignore retired cars

codes = [r["scode"] for r in requests.get(f"{API}/current-station/latest",                        # cars based here
    params={"where": f'and(sorigin.eq.AlpsGo,mvalue.eq."{STATION}",mvalidtime.gt.{FRESH})',
            "select": "scode", "limit": -1}).json()["data"]]

quoted = ",".join(f'"{c}"' for c in codes)                                                        # scode is a string column
rows = requests.get(f"{API}/future-availability-180/latest",
    params={"where": f"and(sorigin.eq.AlpsGo,scode.in.({quoted}),mvalidtime.gt.{FRESH})",
            "select": "scode,mvalue", "limit": -1}).json()["data"]

number_available_3h = sum(1 for r in rows if r["mvalue"] == 1)                                     # just the count
```

```js
const API = "https://mobility.api.opendatahub.com/v2/flat,node/CarsharingCar", STATION = "1092945197";
const FRESH = new Date(Date.now() - 3600e3).toISOString().slice(0, 19) + "Z";  // ignore retired cars
const get = async (u) => (await (await fetch(u)).json()).data;

const codes = (await get(`${API}/current-station/latest`                                          // cars based here
  + `?where=and(sorigin.eq.AlpsGo,mvalue.eq.${encodeURIComponent(`"${STATION}"`)},mvalidtime.gt.${FRESH})`
  + `&select=scode&limit=-1`)).map(r => r.scode);

const quoted = encodeURIComponent(codes.map(c => `"${c}"`).join(","));                            // scode is a string column
const rows = await get(`${API}/future-availability-180/latest`
  + `?where=and(sorigin.eq.AlpsGo,scode.in.(${quoted}),mvalidtime.gt.${FRESH})&select=scode,mvalue&limit=-1`);

const numberAvailable3h = rows.filter(r => r.mvalue === 1).length;                                 // just the count
```
