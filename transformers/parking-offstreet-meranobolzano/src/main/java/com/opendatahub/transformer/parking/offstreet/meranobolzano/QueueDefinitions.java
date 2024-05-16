package com.opendatahub.transformer.parking.offstreet.meranobolzano;

import java.util.Map;

import org.springframework.amqp.core.Binding;
import org.springframework.amqp.core.BindingBuilder;
import org.springframework.amqp.core.Exchange;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.TopicExchange;
import org.springframework.amqp.support.converter.Jackson2JsonMessageConverter;
import org.springframework.amqp.support.converter.MessageConverter;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class QueueDefinitions {
    @Bean
    public Exchange routed(@Value("${mq.listen.exchange}") String exchange) {
        return new TopicExchange(exchange);
    }

    @Value("${mq.listen.acktimeout}")
    private int queueAcktimeout;

    private Map<String, Object> defaultQueueArgs = Map.of("x-consumer-timeout", queueAcktimeout);

    private Queue queue(String name) {
        return new Queue(name, true, false, false, defaultQueueArgs);
    }

    @Bean
    public Queue meta(@Value("${mq.meta.queue}") String name) {
        return queue(name);
    }

    @Bean
    public Queue merano(@Value("${mq.merano.queue}") String name) {
        return queue(name);
    }

    @Bean
    public Queue bolzano(@Value("${mq.bolzano.queue}") String name) {
        return queue(name);
    }

    @Bean
    public Binding metaBinding(Exchange routed, Queue meta, @Value("${mq.meta.key}") String key) {
        return BindingBuilder.bind(meta).to(routed).with(key).noargs();
    }

    @Bean
    public Binding meranoBinding(Exchange routed, Queue merano, @Value("${mq.merano.key}") String key) {
        return BindingBuilder.bind(merano).to(routed).with(key).noargs();
    }

    @Bean
    public Binding bolzanoBinding(Exchange routed, Queue bolzano, @Value("${mq.bolzano.key}") String key) {
        return BindingBuilder.bind(bolzano).to(routed).with(key).noargs();
    }

    @Bean
    public MessageConverter jsonMessageConverter() {
        return new Jackson2JsonMessageConverter();
    }
}
