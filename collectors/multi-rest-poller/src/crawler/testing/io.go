package crawler_testing

import (
	"encoding/json"
	"io"
	"os"
)

func LoadInputData[P any](in *P, file_path string) error {
	file, err := os.Open(file_path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the file content
	byteValue, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Unmarshal JSON into struct
	err = json.Unmarshal(byteValue, in)
	if err != nil {
		return err
	}
	return nil
}

func LoadOutput[P any](in *P, file_path string) error {
	file, err := os.Open(file_path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the file content
	byteValue, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Unmarshal JSON into struct
	err = json.Unmarshal(byteValue, in)
	if err != nil {
		return err
	}
	return nil
}

func WriteOutput(out interface{}, file_path string) error {
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(file_path, data, 0777)
	return err
}
