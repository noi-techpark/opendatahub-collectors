package com.opendatahub.transformer.lib.listener;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

import org.springframework.amqp.rabbit.annotation.Exchange;
import org.springframework.amqp.rabbit.annotation.Queue;
import org.springframework.amqp.rabbit.annotation.QueueBinding;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.amqp.rabbit.annotation.Argument;

@Target({ElementType.TYPE, ElementType.METHOD, ElementType.ANNOTATION_TYPE})
@Retention(RetentionPolicy.RUNTIME)
@RabbitListener(bindings = @QueueBinding(
        value = @Queue(value = "${mq.listen.queue}",  
            durable = "true", 
            autoDelete = "false",
            declare = "true",
            arguments = @Argument( 
              name = "x-consumer-timeout", 
              value = "${mq.listen.acktimeout}")), 
        exchange = @Exchange(
                value = "${mq.listen.exchange}", 
                declare = "false"),
        key = "${mq.listen.key}")
)
public @interface TransformerListener {
}
