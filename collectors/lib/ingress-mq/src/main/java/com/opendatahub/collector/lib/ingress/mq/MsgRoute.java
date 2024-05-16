package com.opendatahub.collector.lib.ingress.mq;

import org.apache.camel.builder.RouteBuilder;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

@Component
public class MsgRoute extends RouteBuilder {
    private String from = "mq";

    public MsgRoute(String from) {
        this.from = from;
    }

    public String getRouteUri() {
        return "direct:" + from;
    }

    public MsgRoute() {
    }

    @Value("${ingress.provider:#{null}}")
    String provider;

    @Autowired
    RabbitMQConnection rabbitMQConfig;
    
    public static final String HEADER_PROVIDER = "provider";
    
    @Override
    public void configure() throws Exception {
        from("direct:" + from)
            .routeId("to-odh-ingress-route")
            // Possibility to set providers from header, overriding the configured one. Use in case you need more than one provider
            .process(exchange -> WrapperProcessor.process(exchange, exchange.getMessage().getHeader(HEADER_PROVIDER, provider, String.class)))
            // .to("file:bar?doneFileName=done")
            .choice()
            // forward to fastline
            .when(header("fastline").isEqualTo(true))
            // we handle the request as invalid and forward the encapsulated payload to
            // whatever mechanism we want to use to store malformed data
            .to(rabbitMQConfig.getRabbitMQFastlineConnectionString())
            .end()
            .choice()
            // if the payload is not a valid json
            .when(header("failed").isEqualTo(true))
            // we handle the request as invalid and forward the encapsulated payload to
            // whatever mechanism we want to use to store malformed data
            .to(rabbitMQConfig.getRabbitMQIngressDeadletterTo())
            .otherwise()
            // otherwise we forward the encapsulated message to the
            // internal queue waiting to be written in rawDataTable
            .to(rabbitMQConfig.getRabbitMQIngressTo());
    }
}
