// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// RawData is the payload published by the api-crawler collector
// (infrastructure/crawler-config/traffic-famas-prov-bz.silky.yaml): one station
// list fetched from FAMAS's AnagrafichePostazioni, with each station
// enriched (via a forEach step) with its own windowed
// DatiAggregatiSuPostazioni results.
//
// The legacy collector also synced bluetooth passage data
// (DatiPassaggiSuPostazioni) into a BluetoothStation station type -
// deliberately left out here, see traffic-famas-prov-bz.silky.yaml.
type RawData struct {
	Stations []StationRaw `json:"stations"`
}

type StationRaw struct {
	// FAMAS station numeric id. NOT used as the BDP station code - Nome is
	// (see createStations), matching the legacy Java collector so existing
	// BDP station codes are preserved across the migration.
	ID             int                `json:"id"`
	Nome           string             `json:"nome"`
	GeoInfo        GeoInfoRaw         `json:"geoInfo"`
	StradaInfo     StradaInfoRaw      `json:"stradaInfo"`
	NumeroCorsie   int                `json:"numeroCorsie"`
	CorsieInfo     []LaneRaw          `json:"corsieInfo"`
	AggregatedData []AggregatedRecord `json:"aggregatedData"`
}

type GeoInfoRaw struct {
	Latitudine  float64 `json:"latitudine"`
	Longitudine float64 `json:"longitudine"`
	Regione     string  `json:"regione"`
	Comune      string  `json:"comune"`
}

type StradaInfoRaw struct {
	Nome         string  `json:"nome"`
	Chilometrica float64 `json:"chilometrica"`
}

type LaneRaw struct {
	ID            int    `json:"id"`
	Descrizione   string `json:"descrizione"`
	SensoDiMarcia string `json:"sensoDiMarcia"`
}

// AggregatedRecord is one row of FAMAS's DatiAggregatiSuPostazioni response
// (traffic sensor data, one row per lane/direction/5-minute bucket).
type AggregatedRecord struct {
	// "yyyy-MM-ddTHH:mm:ss", optionally with a trailing "Z" - FAMAS is
	// inconsistent about it, always UTC regardless (see parseFamasTime).
	Data string `json:"data"`
	// 0-indexed; lane.ID in CorsieInfo is 1-indexed (see stationForLane).
	Corsia                      int                `json:"corsia"`
	Direzione                   string             `json:"direzione"`
	TotaleVeicoli               *float64           `json:"totaleVeicoli"`
	TotaliPerClasseVeicolare    map[string]float64 `json:"totaliPerClasseVeicolare"`
	MediaArmonicaVelocita       *float64           `json:"mediaArmonicaVelocita"`
	HeadwayMedioSecondi         *float64           `json:"headwayMedioSecondi"`
	VarianzaHeadwayMedioSecondi *float64           `json:"varianzaHeadwayMedioSecondi"`
	GapMedioSecondi             *float64           `json:"gapMedioSecondi"`
	VarianzaGapMedioSecondi     *float64           `json:"varianzaGapMedioSecondi"`
}
