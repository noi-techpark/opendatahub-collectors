// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package testutils provides reusable test utilities for all transformers.
// It includes a mock BDP client for testing without production infrastructure.
package testutils

import (
	"fmt"
	"sync"

	"github.com/noi-techpark/go-bdp-client/bdplib"
)

// MockBDP is a mock implementation of bdplib.Bdp for testing transformers
// without requiring production BDP infrastructure.
type MockBDP struct {
	mu             sync.Mutex
	origin         string
	provenance     string
	SyncedStations map[string][]bdplib.Station // key: stationType
	PushedData     map[string]int              // key: stationType -> count of data points
	Errors         []error
	DataMaps       []*bdplib.DataMap
}

// NewMockBDP creates a new mock BDP instance for testing
func NewMockBDP(origin, provenance string) *MockBDP {
	return &MockBDP{
		origin:         origin,
		provenance:     provenance,
		SyncedStations: make(map[string][]bdplib.Station),
		PushedData:     make(map[string]int),
		Errors:         make([]error, 0),
		DataMaps:       make([]*bdplib.DataMap, 0),
	}
}

// GetOrigin returns the configured origin
func (m *MockBDP) GetOrigin() string {
	return m.origin
}

// GetProvenance returns the configured provenance
func (m *MockBDP) GetProvenance() string {
	return m.provenance
}

// CreateDataMap creates a new data map
func (m *MockBDP) CreateDataMap() bdplib.DataMap {
	dm := bdplib.DataMap{
		Name:   "Mock Data Map",
		Branch: make(map[string]bdplib.DataMap),
		Data:   make([]bdplib.Record, 0),
	}
	m.mu.Lock()
	m.DataMaps = append(m.DataMaps, &dm)
	m.mu.Unlock()
	return dm
}

// SyncStations syncs stations with the mock BDP
func (m *MockBDP) SyncStations(stationType string, stations []bdplib.Station, sync bool, onlyActivation bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.SyncedStations[stationType] == nil {
		m.SyncedStations[stationType] = make([]bdplib.Station, 0)
	}
	m.SyncedStations[stationType] = append(m.SyncedStations[stationType], stations...)
	return nil
}

// SyncDataTypes syncs data types (mock implementation)
func (m *MockBDP) SyncDataTypes(dataTypes []bdplib.DataType) error {
	return nil
}

// PushData pushes data to the mock BDP
func (m *MockBDP) PushData(stationType string, dataMap bdplib.DataMap) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	count := m.countDataPoints(dataMap)
	m.PushedData[stationType] = m.PushedData[stationType] + count
	
	return nil
}

func (m *MockBDP) countDataPoints(dm bdplib.DataMap) int {
	count := len(dm.Data)
	for _, branch := range dm.Branch {
		count += m.countDataPoints(branch)
	}
	return count
}

// TransformMetrics contains comprehensive test metrics for transformer validation
type TransformMetrics struct {
	// Input data counts
	TotalProviders int
	TotalStations  int
	TotalVehicles  int
	
	// Sync operation counts
	StationsByType map[string]int
	VehiclesByType map[string]int
	
	// Data points
	DataPointsByType map[string]int
	TotalDataPoints  int
	
	// Warnings
	GenericTypeStations int
	OrphanedStations    int
	GenericTypeVehicles int
	
	// Errors
	Errors []error
}

// GetMetrics extracts comprehensive metrics from the mock BDP
func (m *MockBDP) GetMetrics() TransformMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	metrics := TransformMetrics{
		StationsByType:   make(map[string]int),
		VehiclesByType:   make(map[string]int),
		DataPointsByType: make(map[string]int),
		Errors:           m.Errors,
	}
	
	for stationType, stations := range m.SyncedStations {
		count := len(stations)
		metrics.StationsByType[stationType] = count
		
		if stationType == "SharingMobilityStation" || stationType == "SharingMobilityService" {
			metrics.GenericTypeStations += count
		}
		
		if IsVehicleType(stationType) {
			metrics.VehiclesByType[stationType] = count
			metrics.TotalVehicles += count
		} else {
			metrics.TotalStations += count
		}
	}
	
	for stationType, count := range m.PushedData {
		metrics.DataPointsByType[stationType] = count
		metrics.TotalDataPoints += count
	}
	
	return metrics
}

// VehicleTypes contains known vehicle station types
var VehicleTypes = []string{
	"Bicycle",
	"ScooterSharingVehicle",
	"CarsharingCar",
	"SharingMobilityVehicle",
}

// IsVehicleType returns true if the station type represents a vehicle
func IsVehicleType(stationType string) bool {
	for _, vt := range VehicleTypes {
		if stationType == vt {
			return true
		}
	}
	return false
}

// PrintReport prints a comprehensive test report to stdout
func (m *TransformMetrics) PrintReport() {
	separator := repeatString("=", 70)
	
	fmt.Println("\n" + separator)
	fmt.Println("          TRANSFORMER TEST REPORT")
	fmt.Println(separator)
	
	fmt.Println("\n📊 INPUT DATA FROM COLLECTOR")
	fmt.Printf("   Providers: %d\n", m.TotalProviders)
	fmt.Printf("   Physical Stations: %d\n", m.TotalStations)
	fmt.Printf("   Vehicles: %d\n", m.TotalVehicles)
	
	fmt.Println("\n✅ STATIONS SYNCED TO BDP")
	for stationType, count := range m.StationsByType {
		if !IsVehicleType(stationType) {
			fmt.Printf("   %s: %d\n", stationType, count)
		}
	}
	
	fmt.Println("\n✅ VEHICLES SYNCED TO BDP")
	for stationType, count := range m.VehiclesByType {
		fmt.Printf("   %s: %d\n", stationType, count)
	}
	
	fmt.Println("\n📈 DATA POINTS PUSHED")
	fmt.Printf("   Total: %d measurements\n", m.TotalDataPoints)
	for stationType, count := range m.DataPointsByType {
		fmt.Printf("   %s: %d\n", stationType, count)
	}
	
	if m.GenericTypeStations > 0 || m.OrphanedStations > 0 || m.GenericTypeVehicles > 0 {
		fmt.Println("\n⚠️  WARNINGS")
		if m.GenericTypeStations > 0 {
			fmt.Printf("   %d stations using generic type\n", m.GenericTypeStations)
		}
		if m.OrphanedStations > 0 {
			fmt.Printf("   %d orphaned stations (no parent region)\n", m.OrphanedStations)
		}
		if m.GenericTypeVehicles > 0 {
			fmt.Printf("   %d vehicles using generic type\n", m.GenericTypeVehicles)
		}
	}
	
	if len(m.Errors) > 0 {
		fmt.Println("\n❌ ERRORS")
		for i, err := range m.Errors {
			fmt.Printf("   %d: %v\n", i+1, err)
		}
	} else {
		fmt.Println("\n✅ NO ERRORS")
	}
	
	fmt.Println("\n" + separator)
}

func repeatString(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
