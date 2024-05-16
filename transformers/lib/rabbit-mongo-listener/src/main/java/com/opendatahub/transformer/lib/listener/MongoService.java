package com.opendatahub.transformer.lib.listener;

import java.util.Map;

import org.bson.Document;
import org.bson.types.ObjectId;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.stereotype.Service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.mongodb.client.MongoClient;
import com.mongodb.client.MongoClients;
import com.mongodb.client.model.Filters;

@Service
public class MongoService {
    @Autowired
    private MongoClient client;
    @Autowired
    private ObjectMapper om;

    @Bean
    public static MongoClient mongoClient(@Value("${mongo.connectionString}") String connectionString) {
        return MongoClients.create(connectionString);
    }

    @Deprecated()
    /** Use getRaw() instead, as you probably need the timestamp anyway */
    public String getRawPayload(String database, String collection, String id) throws Exception {
        return client
            .getDatabase(database)
            .getCollection(collection)
            .find(Filters.eq("_id", new ObjectId(id)))
            .first()
            .getString("rawdata");
    }
    
    public RawDto getRaw(String database, String collection, String id) throws Exception {
        Document doc = client
                .getDatabase(database)
                .getCollection(collection)
                .find(Filters.eq("_id", new ObjectId(id)))
                .first();
        return om.convertValue(doc, RawDto.class);
    }
}
