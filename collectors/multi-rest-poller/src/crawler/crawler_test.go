package crawler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crawler_testing "opendatahub.com/multi-rest-poller/crawler/testing"
)

func TestExampleSingle(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities":                   "testdata/crawler/example_single/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_single/facility_id_2.json",
	})

	craw := NewApiCrawler("testing/example_single.yaml")
	craw.SetClientRoundTripper(mockTransport)

	err := craw.Run()
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example_single/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestExample2(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities":                    "testdata/crawler/example2/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2":  "testdata/crawler/example2/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=s3": "testdata/crawler/example2/facility_id_s3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=s4": "testdata/crawler/example2/facility_id_s4.json",
		"https://www.onecenter.info/api/DAZ/Locations/l1":                     "testdata/crawler/example2/location_id_l1.json",
		"https://www.onecenter.info/api/DAZ/Locations/l2":                     "testdata/crawler/example2/location_id_l2.json",
		"https://www.onecenter.info/api/DAZ/Locations/l3":                     "testdata/crawler/example2/location_id_l3.json",
	})

	craw := NewApiCrawler("testing/example2.yaml")
	craw.SetClientRoundTripper(mockTransport)

	err := craw.Run()
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example2/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedIncrement(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=0": "testdata/crawler/paginated_increment/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=1": "testdata/crawler/paginated_increment/facilities_2.json",
	})

	craw := NewApiCrawler("testing/example_pagination_increment.yaml")
	craw.SetClientRoundTripper(mockTransport)

	err := craw.Run()
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/paginated_increment/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedIncrementNested(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=0":          "testdata/crawler/paginated_increment_stream/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=1":          "testdata/crawler/paginated_increment_stream/facilities_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/paginated_increment_stream/facility_id_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/paginated_increment_stream/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=3": "testdata/crawler/paginated_increment_stream/facility_id_3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=4": "testdata/crawler/paginated_increment_stream/facility_id_4.json",
	})

	craw := NewApiCrawler("testing/example_pagination_increment_nested.yaml")
	craw.SetClientRoundTripper(mockTransport)

	err := craw.Run()
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/paginated_increment_stream/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedIncrementStream(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=0":          "testdata/crawler/paginated_increment_stream/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=1":          "testdata/crawler/paginated_increment_stream/facilities_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/paginated_increment_stream/facility_id_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/paginated_increment_stream/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=3": "testdata/crawler/paginated_increment_stream/facility_id_3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=4": "testdata/crawler/paginated_increment_stream/facility_id_4.json",
	})

	craw := NewApiCrawler("testing/example_pagination_increment_stream.yaml")
	craw.SetClientRoundTripper(mockTransport)

	stream := craw.GetDataStream()
	defer close(stream)
	data := make([]interface{}, 0)

	go func() {
		for d := range stream {
			data = append(data, d)
		}
	}()

	err := craw.Run()
	require.Nil(t, err)

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/paginated_increment_stream/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}
