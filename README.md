# Open Data Hub - Data Collectors 

This repo is part of the [Open Data Hub](https://opendatahub.com/) project

It contains microservices used for data ingestion and transformation.

### Data Collectors
Get raw data from a data provider and put it onto a message queue as is. The Open Data Hub then stores this raw data in a db for further processing and notifies transformers that new data is present

### Transformers
Listen on the message queue for new data events. They fetch the raw data from the db, transform it and push the result to the Open Data Hub main database.
 
### Infrastructure
For more details on the infrastructure, including source code and configuration for the event system, see [our infrastructure repo](https://github.com/noi-techpark/infrastructure-v2)

### Migrated code
Data collectors are initially migrated from [bdp-commons](https://github.com/noi-techpark/bdp-commons)