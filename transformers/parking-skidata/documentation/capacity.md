<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Capacity

Where capacity enters the transformer, and what each source is used for.

## Sources

| Source | Used for | Stored |
| --- | --- | --- |
| event, category 3 | station `capacity`, facility capacity sum, `free` | yes, cached per carpark |
| event, categories 1 & 2 | `free_short_stay` / `free_subscribers` only | no |
| counting categories, category 3 | boot fallback until the first event arrives | yes |
| counting categories, categories 1 & 2 | nothing | no |

A category capacity from an event is read once, to compute
`free_<category> = capacity - level`, and then discarded. Only the category-3
value survives the message: it becomes the station's `capacity` and feeds the
facility sum.

In the counting-categories table, `TotalCapacity` reads category-3 rows only.
The category-1 and category-2 rows are never read for their capacity. They
contribute nothing but their `carparkId` to the topology — and since every
carpark has a category-3 row, even that is redundant.

Per-category capacity is deliberately never stored. For at least two carparks it
is a live quota rather than a fact: at `0607242/0` the short-stay and subscriber
capacities repartition a fixed total of 245 continuously, and at `0609008/1` the
same happens on a pool of 42.

## Open: is per-category `free` worth publishing?

At `0607242/0` the subscriber capacity is adjusted to keep a constant headroom:

```
cap 85 / level 65  ->  free_subscribers = 20
cap 86 / level 66  ->  free_subscribers = 20
cap 87 / level 67  ->  free_subscribers = 20
cap 88 / level 68  ->  free_subscribers = 20
cap 89 / level 69  ->  free_subscribers = 20
```

`free_subscribers` is pinned at 20 by construction. It is not availability, it
is the headroom policy restated. `occupied_subscribers` is a real count of cars.

If that pattern holds across the estate, the meaningful per-category output is
`occupied_*` only, with `free` published on the total alone. It is confirmed on
two carparks so far; measuring the rest needs a query against the raw data lake.
