package desktop

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"testing"
)

func TestStandaloneMetadataMatchesAppConfiguration(t *testing.T) {
	b, err := os.ReadFile("Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	var plist struct {
		Dict struct {
			Entries []struct {
				XMLName xml.Name
				Text    string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal(b, &plist); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for i, entry := range plist.Dict.Entries {
		if entry.XMLName.Local == "key" && i+1 < len(plist.Dict.Entries) {
			values[entry.Text] = plist.Dict.Entries[i+1].Text
		}
	}
	b, err = os.ReadFile("../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Name, Identifier, Version string
		MacOS                     struct{ MinimumSystemVersion string }
	}
	if err := json.Unmarshal(b, &config); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"CFBundleName": config.Name, "CFBundleIdentifier": config.Identifier, "CFBundleVersion": config.Version, "LSMinimumSystemVersion": config.MacOS.MinimumSystemVersion} {
		if want == "" || values[key] != want {
			t.Fatalf("%s: embedded=%q app=%q", key, values[key], want)
		}
	}
}
