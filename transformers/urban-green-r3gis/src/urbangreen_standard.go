// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Standards is a wrapper that holds all loaded standard versions
type Standards struct {
	versions map[string]*Standard
}

type Standard struct {
	Version   string
	Codes     map[string]*GreenCode
	MainTypes map[string]*MainType
	SubTypes  map[string]*SubType
}

type GreenCode struct {
	ID    string            `json:"id"`
	Names map[string]string `json:"names"` // key: language code (de, en, it, etc.)
}

type MainType struct {
	ID    string            `json:"id"`
	TagID string            `json:"tagId"`
	Names map[string]string `json:"names"`
}

type SubType struct {
	ID    string            `json:"id"`
	TagID string            `json:"tagId"`
	Names map[string]string `json:"names"`
}

// ParsedCode represents the parsed components of an urban green code
// e.g., P103108 -> Geometry: P, MainType: 1, SubType: 03, Element: 108
type ParsedCode struct {
	Geometry string // S (Surface), L (Line), P (Point)
	MainType string // 1 digit
	SubType  string // 2 digits
	Element  string // 3 digits
}

// ParseCode parses an encoded type string into its 3 components plus geometry
// Format: [S|L|P][MainType:1digit][SubType:2digits][Element:3digits]
// Example: P103108 -> Geometry=P, MainType=1, SubType=03, Element=108
func ParseCode(code string) (*ParsedCode, error) {
	if len(code) != 7 {
		return nil, fmt.Errorf("invalid code length: expected 7 characters, got %d", len(code))
	}

	geometry := string(code[0])
	if geometry != "S" && geometry != "L" && geometry != "P" {
		return nil, fmt.Errorf("invalid geometry type: expected S, L, or P, got %s", geometry)
	}

	return &ParsedCode{
		Geometry: geometry,
		MainType: string(code[1]),
		SubType:  code[2:4],
		Element:  code[4:7],
	}, nil
}

// LoadStandards loads all urban green standards from the given resources directory
// Each subdirectory is treated as a version (e.g., "2.1")
func LoadStandards(resourcesDir string) (*Standards, error) {
	standards := &Standards{
		versions: make(map[string]*Standard),
	}

	entries, err := os.ReadDir(resourcesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read resources directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		versionPath := filepath.Join(resourcesDir, entry.Name())
		standard, err := loadStandardVersion(versionPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load standard version %s: %w", entry.Name(), err)
		}

		standards.versions[standard.Version] = standard
	}

	return standards, nil
}

// GetVersion returns a Standard by version, or nil if not found
func (s *Standards) GetVersion(version string) *Standard {
	return s.versions[version]
}

// AllVersions returns all loaded standards
func (s *Standards) AllVersions() []*Standard {
	result := make([]*Standard, 0, len(s.versions))
	for _, std := range s.versions {
		result = append(result, std)
	}
	return result
}

// loadStandardVersion loads a single standard version from a folder
func loadStandardVersion(versionPath string) (*Standard, error) {
	version := filepath.Base(versionPath)

	standard := &Standard{
		Version:   version,
		Codes:     make(map[string]*GreenCode),
		MainTypes: make(map[string]*MainType),
		SubTypes:  make(map[string]*SubType),
	}

	entries, err := os.ReadDir(versionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read version directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}

		filePath := filepath.Join(versionPath, entry.Name())
		name := entry.Name()

		switch {
		case strings.HasPrefix(name, "green_codes"):
			if err := loadGreenCodes(filePath, standard); err != nil {
				return nil, fmt.Errorf("failed to load green codes: %w", err)
			}
		case strings.HasPrefix(name, "main_types"):
			if err := loadMainTypes(filePath, standard); err != nil {
				return nil, fmt.Errorf("failed to load main types: %w", err)
			}
		case strings.HasPrefix(name, "secondary_types"):
			if err := loadSubTypes(filePath, standard); err != nil {
				return nil, fmt.Errorf("failed to load secondary types: %w", err)
			}
		}
	}

	return standard, nil
}

func parseLabels(labelStr string) (map[string]string, error) {
	var labels map[string]string
	if err := json.Unmarshal([]byte(labelStr), &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// toKebabCase converts a string to lowercase kebab-case, removing special characters
// and normalizing unicode (e.g., accented characters become their base form)
func toKebabCase(s string) string {
	// Normalize unicode and remove diacritics
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, s)

	// Convert to lowercase
	normalized = strings.ToLower(normalized)

	// Replace spaces and underscores with hyphens
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")

	// Remove all characters that are not alphanumeric or hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	normalized = reg.ReplaceAllString(normalized, "")

	// Replace multiple consecutive hyphens with single hyphen
	reg = regexp.MustCompile(`-+`)
	normalized = reg.ReplaceAllString(normalized, "-")

	// Trim leading/trailing hyphens
	normalized = strings.Trim(normalized, "-")

	return normalized
}

func loadGreenCodes(filePath string, standard *Standard) error {
	records, err := readCSV(filePath)
	if err != nil {
		return err
	}

	for _, record := range records[1:] { // Skip header
		if len(record) < 2 {
			continue
		}

		code := record[0]
		names, err := parseLabels(record[1])
		if err != nil {
			return fmt.Errorf("failed to parse label JSON for code %s: %w", code, err)
		}

		standard.Codes[code] = &GreenCode{
			ID:    code,
			Names: names,
		}
	}

	return nil
}

func loadMainTypes(filePath string, standard *Standard) error {
	records, err := readCSV(filePath)
	if err != nil {
		return err
	}

	for _, record := range records[1:] { // Skip header
		if len(record) < 2 {
			continue
		}

		code := record[0]
		names, err := parseLabels(record[1])
		if err != nil {
			return fmt.Errorf("failed to parse label JSON for main type %s: %w", code, err)
		}

		tagID := toKebabCase(names["en"])

		standard.MainTypes[code] = &MainType{
			ID:    code,
			TagID: fmt.Sprintf("%s:%s", TagSource, tagID),
			Names: names,
		}
	}

	return nil
}

func loadSubTypes(filePath string, standard *Standard) error {
	records, err := readCSV(filePath)
	if err != nil {
		return err
	}

	for _, record := range records[1:] { // Skip header
		if len(record) < 2 {
			continue
		}

		code := record[0]
		names, err := parseLabels(record[1])
		if err != nil {
			return fmt.Errorf("failed to parse label JSON for sub type %s: %w", code, err)
		}

		tagID := toKebabCase(names["en"])

		standard.SubTypes[code] = &SubType{
			ID:    code,
			TagID: fmt.Sprintf("%s:%s", TagSource, tagID),
			Names: names,
		}
	}

	return nil
}

func readCSV(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	return records, nil
}

// Lookup methods for Standard

// LookupCode looks up a GreenCode by its full code (e.g., P103108)
func (s *Standard) LookupCode(code string) *GreenCode {
	return s.Codes[code]
}

// LookupMainType looks up a MainType by its code
func (s *Standard) LookupMainType(code string) *MainType {
	return s.MainTypes[code]
}

// LookupSubType looks up a SubType by its code
func (s *Standard) LookupSubType(code string) *SubType {
	return s.SubTypes[code]
}

// LookupParsed parses a code and returns all related lookups
func (s *Standard) LookupParsed(code string) (*ParsedCode, *GreenCode, *MainType, *SubType, error) {
	parsed, err := ParseCode(code)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	greenCode := s.LookupCode(code)
	mainType := s.LookupMainType(parsed.MainType)
	subType := s.LookupSubType(parsed.SubType)

	return parsed, greenCode, mainType, subType, nil
}
