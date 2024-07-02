// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.collector.lib.ingress.mq;

import org.springframework.amqp.rabbit.connection.CachingConnectionFactory;
import org.springframework.amqp.rabbit.connection.ConnectionFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.stereotype.Component;

@Component
public class RabbitMQConnection {
    @Value("${ingress.rabbitmq.uri}")
    String rabbitUri;

    @Value("${ingress.rabbitmq.clientname}")
    String clientname;
    
    static final String RABBITMQ_INGRESS_QUEUE = "ingress-q";
    static final String RABBITMQ_INGRESS_EXCHANGE = "ingress";
    static final String RABBITMQ_INGRESS_DEADLETTER_QUEUE = "ingress-dl-q";
    static final String RABBITMQ_INGRESS_DEADLETTER_EXCHANGE = "ingress-dl";
    static final String RABBITMQ_FASTLINE_EXCHANGE = "fastline";
    
    private static final String CONNECTION_FACTORY = "odh-ingress";

    @Bean(CONNECTION_FACTORY)
    public ConnectionFactory createConnectionFactory() throws Exception {
        final CachingConnectionFactory fac = new CachingConnectionFactory();
        fac.setConnectionNameStrategy(_f -> clientname + ": " + System.getenv("HOSTNAME"));
        fac.getRabbitConnectionFactory().setUri(rabbitUri);
        return fac;
    }
    
    public String getRabbitMQIngressTo() {
        return String.format("spring-rabbitmq:%s?connectionFactory=#bean:%s&queues=%s&exchangePattern=InOnly&exchangeType=fanout&acknowledgeMode=AUTO",
                RABBITMQ_INGRESS_EXCHANGE,
                CONNECTION_FACTORY,
                RABBITMQ_INGRESS_QUEUE);
    }

    public String getRabbitMQIngressDeadletterTo() {
        return String.format("spring-rabbitmq:%s?queues=%s&exchangePattern=InOnly&exchangeType=fanout&acknowledgeMode=AUTO",
                RABBITMQ_INGRESS_DEADLETTER_EXCHANGE,
                RABBITMQ_INGRESS_DEADLETTER_QUEUE);
    }

    public String getRabbitMQFastlineConnectionString() {
        final StringBuilder uri = new StringBuilder(String.format("spring-rabbitmq:%s?exchangePattern=InOnly&exchangeType=topic&acknowledgeMode=AUTO",
                RABBITMQ_FASTLINE_EXCHANGE));
        return uri.toString();
    }
}
