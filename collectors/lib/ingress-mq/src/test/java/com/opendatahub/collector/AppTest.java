package com.opendatahub.collector;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import java.util.Map;

import org.junit.Test;

import com.opendatahub.collector.lib.ingress.mq.WrapperProcessor;

/**
 * Unit test for simple App.
 */
public class AppTest 
{
    /**
     * Rigorous Test :-)
     */
    @Test
    public void shouldAnswerWithTrue()
    {
        assertTrue( true );
    }
    
    @Test
    public void testQueryStringSplitter(){
        String query = "test=2&test2=value&key=somevalue";
        Map<String, String> expectedResult = Map.of(
            "test", "2", 
            "test2", "value",
            "key", "somevalue"
        );
        
        assertEquals(expectedResult, WrapperProcessor.queryStringToMap(query));
    }
}
