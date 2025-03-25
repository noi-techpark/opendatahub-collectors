module opendatahub.com/rest-poller

go 1.23.1

require (
	github.com/hashicorp/go-retryablehttp v0.7.7
	github.com/joho/godotenv v1.5.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/noi-techpark/go-opendatahub-discoverswiss v0.0.0-20250325093525-f1c5e788ef87
	github.com/robfig/cron/v3 v3.0.1
)

require github.com/hashicorp/go-cleanhttp v0.5.2 // indirect

require (
	github.com/noi-techpark/go-opendatahub-ingest v1.3.1
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
)
