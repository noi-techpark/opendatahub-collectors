# Data collector structure

Three independent tasks
- pushMetadata (cron scheduler.stations)
- pushData (cron scheduler.slots)
- syncDataTypes (once at startup)

There is also some kind of forecasting service functionality, but it's not used.

Two different data providers
- Bolzano: uses XMLRPC
    - get list of parking areas
    - get metadata of parking area
    - get occupation data of parking area
- Merano: uses REST/JSON
    - get a list of parking areas, including metadata and occupation

Station data is enriched with metadata coming from a Google spreadsheet.  
This metadata is just translated names in en/it/de and a "standard name"  
Stations missing in the Google spreadsheet are added as new entries

Migration strategy:
1 rest poller for Merano
1 custom client for Bolzano
1 generic spreadsheet collector for sheet


Transformer:
1 route for Bolzano stations
1 route for Merano stations

Independent spreadsheet route, like an elaboration.
Gets all stations from ninja, enriches them, (adds new entries to spreadsheet?)
Gets triggered by new spreadsheets, but also after completed pushes (internal SEDA)


