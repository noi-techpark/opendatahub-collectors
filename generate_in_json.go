package main

import (
	"encoding/json"
	"os"
)

func main() {
	xmlBytes, _ := os.ReadFile("transformers/webcam-feratel/testdata/raw.xml")
	b, _ := json.Marshal(string(xmlBytes))
	os.WriteFile("transformers/webcam-feratel/testdata/in.json", b, 0644)
}
