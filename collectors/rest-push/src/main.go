package main

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

var Config struct {
	LogLevel     string `default:"INFO"`
	RabbitURL    string `default:"amqp://rabbitmq:5672"`
	SwaggerURL   string `default:"http://localhost:8081"`
	AuthURL      string `default:"https://auth.opendatahub.testingmachine.eu/auth/"`
	AuthRealm    string `default:"noi"`
	AuthClientId string `default:"opendatahub-push-testing"`
}

func initConfig() {
	err := envconfig.Process("APP", &Config)
	if err != nil {
		log.Fatal(err.Error())
	}
}

func main() {
	initConfig()
	initLogging()

	q := make(chan restMsg)
	InitRabbitMq(q)
	serve(q)
}
