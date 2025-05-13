// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentMapper

import (
	"fmt"
	"strconv"

	odhContentModel "opendatahub.com/tr-discoverswiss-lodging/odh-content-model"
)

func MapStarRatingToCategory(starRating float64) string {
	value := starRating
	if value >= 1 {
		if value == float64(int32(value)) {
			return fmt.Sprintf("%dstars", int32(value))
		} else {
			return fmt.Sprintf("%dsstars", int32(value))
		}
	} else {
		return "Not categorized"
	}
}

func MapAdditionalTypeToAccoTypeId(value string) string {
	if value == "Hotel" || value == "Pension" {
		return "HotelPension"
	} else if value == "" {
		return "Notdefined"
	} else if value == "ServicedApartments" || value == "HolidayApartment" || value == "GroupAccommodation" {
		return "Apartment"
	} else if value == "BedAndBreakfast" || value == "HolidayHouse" || value == "GuestHouse" || value == "PrivateRoom" {
		return "BedBreakfast"
	} else if value == "Hostel" {
		return "Youth"
	} else if value == "Campground" {
		return "Camping"
	} else if value == "Mountainhut" {
		return "Mountain"
	}
	return value
}

func MapLodgingBusinessToAccommodation(lb odhContentModel.LodgingBusiness) odhContentModel.Accommodation {
	acco := odhContentModel.Accommodation{
		Source:    "discoverswiss",
		Active:    true,
		Shortname: lb.Name,
	}

	acco.Mapping.DiscoverSwiss.Id = lb.Identifier
	acco.LicenseInfo.Author = ""
	acco.LicenseInfo.License = lb.License
	acco.LicenseInfo.ClosedData = false
	acco.LicenseInfo.LicenseHolder = "www.discover.swiss"

	acco.GpsInfo = []struct {
		Gpstype               string  `json:"Gpstype"`
		Latitude              float64 `json:"Latitude"`
		Longitude             float64 `json:"Longitude"`
		Altitude              float64 `json:"Altitude"`
		AltitudeUnitofMeasure string  `json:"AltitudeUnitofMeasure"`
	}{
		{
			Gpstype:               "position",
			Latitude:              lb.Geo.Latitude,
			Longitude:             lb.Geo.Longitude,
			Altitude:              lb.Geo.Elevation,
			AltitudeUnitofMeasure: "m",
		},
	}

	//apparently the publishedOn does not reflects the provider
	// publishedOn := strings.Replace(lb.DataGovernance.Provider.Link[0].Url, "https://www.", "", 1)
	// publishedOn = strings.Replace(publishedOn, "/de", "", 1)
	// publishedOn = strings.Replace(publishedOn, "/", "", 1)
	// acco.PublishedOn = append(acco.PublishedOn, publishedOn)

	//NotWorking
	acco.LocationInfo.RegionInfo.Name.De = lb.Address.AddressRegion
	acco.LocationInfo.RegionInfo.Name.It = lb.Address.AddressRegion
	acco.LocationInfo.RegionInfo.Name.En = lb.Address.AddressRegion
	acco.LocationInfo.RegionInfo.Name.Fr = lb.Address.AddressRegion
	acco.LocationInfo.RegionInfo.Id = fmt.Sprintf("%s-%s", lb.Address.AddressCountry, lb.Address.AddressRegion)

	acco.LocationInfo.MunicipalityInfo.Name.De = lb.Address.AddressLocality
	acco.LocationInfo.MunicipalityInfo.Name.It = lb.Address.AddressLocality
	acco.LocationInfo.MunicipalityInfo.Name.En = lb.Address.AddressLocality
	acco.LocationInfo.MunicipalityInfo.Name.Fr = lb.Address.AddressLocality
	acco.LocationInfo.MunicipalityInfo.Id = fmt.Sprintf("%s-%s", lb.Address.AddressCountry, lb.Address.AddressLocality)

	acco.HasLanguage = append(acco.HasLanguage, "de")
	acco.HasLanguage = append(acco.HasLanguage, "it")
	acco.HasLanguage = append(acco.HasLanguage, "en")
	acco.HasLanguage = append(acco.HasLanguage, "fr")

	acco.AccoDetail.LanguageDe = odhContentModel.AccoDetailLanguage{
		Fax:         lb.FaxNumber,
		Name:        lb.Name,
		Street:      lb.Address.StreetAddress,
		Zip:         lb.Address.PostalCode,
		City:        lb.Address.AddressLocality,
		CountryCode: lb.Address.AddressCountry,
		Email:       lb.Address.Email,
		Phone:       lb.Address.Telephone,
	}

	acco.AccoDetail.LanguageEn = acco.AccoDetail.LanguageDe
	acco.AccoDetail.LanguageIt = acco.AccoDetail.LanguageDe

	for _, room := range lb.NumberOfRooms {
		value, err := strconv.Atoi(room.Value)
		if err != nil {
			fmt.Println("Error converting room value to int")
			continue
		}

		switch room.PropertyID {
		case "total":
			acco.AccoOverview.TotalRooms = &value
		case "single":
			acco.AccoOverview.SingleRooms = &value
		case "double":
			acco.AccoOverview.DoubleRooms = &value
		case "triple":
			acco.AccoOverview.TripleRooms = &value
		}
	}

	acco.AccoOverview.CheckInFrom = lb.CheckinTime
	acco.AccoOverview.CheckInTo = lb.CheckinTimeTo
	acco.AccoOverview.CheckOutFrom = lb.CheckoutTimeFrom
	acco.AccoOverview.CheckOutTo = lb.CheckoutTime
	acco.AccoOverview.MaxPersons = lb.NumberOfBeds

	for _, photo := range lb.Photo {
		acco.ImageGallery = append(acco.ImageGallery, odhContentModel.ImageGalleryItem{
			ImageUrl: photo.ContentUrl, CopyRight: photo.CopyrightNotice,
			ImageDesc:   odhContentModel.LanguageMap{DE: photo.Name, EN: photo.Name, IT: photo.Name, FR: photo.Name},
			ImageName:   &photo.Identifier,
			ImageSource: &photo.DataGovernance.Source.Name,
		})
	}

	acco.AccoTypeId = MapAdditionalTypeToAccoTypeId(lb.AdditionalType)

	acco.AccoCategoryId = MapStarRatingToCategory(lb.StarRating.RatingValue)

	return acco
}
