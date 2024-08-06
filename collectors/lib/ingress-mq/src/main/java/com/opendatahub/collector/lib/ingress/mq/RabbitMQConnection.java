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
    
    static final String RABBITMQ_INGRESS_EXCHANGE = "ingress";
    private static final String CONNECTION_FACTORY = "odh-ingress";

    @Bean(CONNECTION_FACTORY)
    public ConnectionFactory createConnectionFactory() throws Exception {
        final CachingConnectionFactory fac = new CachingConnectionFactory();
        fac.setConnectionNameStrategy(_f -> clientname + ": " + System.getenv("HOSTNAME"));
        fac.getRabbitConnectionFactory().setUri(rabbitUri);
        return fac;
    }
    
    public String getRabbitMQIngressTo() {
        return String.format("spring-rabbitmq:%s?connectionFactory=#bean:%s&exchangePattern=InOnly&exchangeType=fanout&acknowledgeMode=AUTO",
                RABBITMQ_INGRESS_EXCHANGE,
                CONNECTION_FACTORY);
    }
}
