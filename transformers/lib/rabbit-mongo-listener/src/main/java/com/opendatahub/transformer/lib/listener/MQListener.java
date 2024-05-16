package com.opendatahub.transformer.lib.listener;

import org.springframework.amqp.rabbit.connection.CachingConnectionFactory;
import org.springframework.amqp.rabbit.connection.ConnectionFactory;
import org.springframework.amqp.rabbit.listener.SimpleMessageListenerContainer;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.PropertySource;

@Configuration
@PropertySource("classpath:rabbit-mongo-listener.properties")
public class MQListener {
    
    @Bean
    public SimpleMessageListenerContainer mqContainer(ConnectionFactory connectionFactory) {
        var container = new SimpleMessageListenerContainer(connectionFactory);
        container.setMissingQueuesFatal(false);
        container.setMismatchedQueuesFatal(false);
        return container;
    }
    
    @Bean
    public CachingConnectionFactory mqFactory( @Value("${mq.listen.uri}") String uri) throws Exception {
        var fac = new CachingConnectionFactory(); 
        fac.getRabbitConnectionFactory().setUri(uri);
        return fac;
    }
}
