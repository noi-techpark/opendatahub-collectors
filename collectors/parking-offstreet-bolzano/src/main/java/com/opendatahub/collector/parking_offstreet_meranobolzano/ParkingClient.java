// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package com.opendatahub.collector.parking_offstreet_meranobolzano;

import java.net.URL;
import java.util.Arrays;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import org.apache.xmlrpc.XmlRpcException;
import org.apache.xmlrpc.client.XmlRpcClient;
import org.apache.xmlrpc.client.XmlRpcClientConfigImpl;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

@Service
public class ParkingClient {
    private static final String PROTOCOLL = "http://";
    private static final String P_GUIDE_GET_PARKING_METADATA = "pGuide.getCaratteristicheParcheggio";
    private String defaultServerHost;
    private String defaultServerPort;
    private String defaultSiteName;
    private static final int XMLRPCREPLYTIMEOUT = 10000;
    private static final int XMLRPCCONNECTIONTIMEOUT = 8000;
    private static final String P_GUIDE_GET_POSTI_LIBERI_PARCHEGGIO_EXT = "pGuide.getPostiLiberiParcheggioExt";
    private XmlRpcClient client;

    @Autowired
    public ParkingClient(@Value("${xmlrpc.host}") String defaultServerHost,
            @Value("${xmlrpc.port}") String defaultServerPort,
            @Value("${xmlrpc.sitename}") String defaultSiteName) {
        this.defaultServerHost = defaultServerHost;
        this.defaultServerPort = defaultServerPort;
        this.defaultSiteName = defaultSiteName;
    }

    public void connect(String serverHost, String serverPort, String siteName) throws Exception {
        if (serverHost == null)
            serverHost = defaultServerHost;
        if (serverPort == null)
            serverPort = defaultServerPort;
        if (siteName == null)
            siteName = defaultSiteName;
        if (client == null) {
            XmlRpcClientConfigImpl config = new XmlRpcClientConfigImpl();
            config.setServerURL(new URL(PROTOCOLL + serverHost + ":" + serverPort + siteName));
            config.setEnabledForExtensions(true);
            config.setReplyTimeout(XMLRPCREPLYTIMEOUT);
            config.setConnectionTimeout(XMLRPCCONNECTIONTIMEOUT);
            client = new XmlRpcClient();
            client.setConfig(config);
        }
    }

    public void connect() throws Exception {
        connect(null, null, null);
    }

    private List<Object> getParkingMetaData(Integer identifier) throws Exception {
        return Arrays.asList(getArray(P_GUIDE_GET_PARKING_METADATA, List.of(identifier)));
    }

    private Integer[] getIdentifiersOfParkingPlaces() throws XmlRpcException {
        return getArrayOfInteger("pGuide.getElencoIdentificativiParcheggi");
    }

    private Object[] getArray(String method, List<Object> pParams) throws XmlRpcException {
        return (Object[]) client.execute(method, pParams);
    }

    private Integer[] getArrayOfInteger(String method) throws XmlRpcException {
        Object[] ar = (Object[]) client.execute(method, (Object[]) null) ;
        return Arrays.copyOf(ar, ar.length, Integer[].class);
    }

    private List<Object> getData(Integer identifier) throws Exception {
        Object[] params = new Object[] { identifier };
        Object object = client.execute(P_GUIDE_GET_POSTI_LIBERI_PARCHEGGIO_EXT, params);
        return Arrays.asList((Object[]) object);
    }

    public Map<Integer, Object> getAllData() throws Exception {
        Map<Integer, Object> ret = new HashMap<>();
        for (Integer identifier : getIdentifiersOfParkingPlaces()){
            ret.put(identifier, Map.of(
                "metadata", getParkingMetaData(identifier), 
                "data", getData(identifier)
            ));
        }
        return ret;
    }
}
