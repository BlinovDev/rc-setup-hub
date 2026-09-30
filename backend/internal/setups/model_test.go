package setups

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const realisticJSON = `{
  "suspension":{"front":{"camber_deg":-5.5,"caster_deg":7,"toe_deg":1,"link_lengths":[{"name":"camber_link","length_mm":42.5}]},"rear":{"camber_deg":-2,"caster_deg":null,"toe_deg":0}},
  "shocks":{"front":{"manufacturer":"Yokomo","model":"Big Bore","spring":{"manufacturer":"Yokomo","color":"Purple"},"oil_cst":250},"rear":{"manufacturer":"Yokomo","model":"Big Bore","spring":{"manufacturer":"Yokomo","color":"Blue"},"oil_cst":300}},
  "electronics":{"motor":"Acuvance Fledge 10.5T","esc":"Acuvance Xarvis XX","servo":"Reve D RS-ST","gyro":"Yokomo DP-302 V4","radio":"Futaba T10PX"}
}`

func realisticData(t *testing.T) *DataV1 {
	t.Helper()
	var data DataV1
	if err := decodeStrict(strings.NewReader(realisticJSON), &data); err != nil {
		t.Fatal(err)
	}
	return &data
}
func stringPtr(value string) *string  { return &value }
func floatPtr(value float64) *float64 { return &value }
func TestSchemaRoundTrip(t *testing.T) {
	data := realisticData(t)
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var result DataV1
	if err := decodeStrict(bytes.NewReader(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Suspension.Rear.ToeDeg == nil || *result.Suspension.Rear.ToeDeg != 0 || result.Suspension.Rear.CasterDeg != nil || result.Shocks.Front.OilCST == nil || result.Electronics.Motor != data.Electronics.Motor {
		t.Fatal("v1 round trip lost values", string(raw))
	}
	var omitted DataV1
	if err := decodeStrict(strings.NewReader(`{"suspension":{"rear":{}}}`), &omitted); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(omitted)
	if err != nil {
		t.Fatal(err)
	}
	if omitted.Suspension.Rear.ToeDeg != nil || strings.Contains(string(raw), "toe_deg") {
		t.Fatal("omitted toe became zero", string(raw))
	}
}
func TestPatchPresence(t *testing.T) {
	for _, tc := range []struct {
		body    string
		present bool
		null    bool
	}{
		{`{}`, false, true}, {`{"chassis_model_id":null,"notes":null}`, true, true}, {`{"chassis_model_id":"model","notes":"note"}`, true, false},
	} {
		var p PatchInput
		if err := decodeStrict(strings.NewReader(tc.body), &p); err != nil {
			t.Fatal(err)
		}
		if p.ChassisModelID.Present != tc.present || (p.ChassisModelID.Value == nil) != tc.null || p.Notes.Present != tc.present {
			t.Fatal("presence lost", p)
		}
	}
	var p PatchInput
	if err := decodeStrict(strings.NewReader(`{"data":{"electronics":{"unknown":true}}}`), &p); err == nil {
		t.Fatal("unknown nested PATCH data accepted")
	}
}
func TestSetupValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*Setup)
		valid  bool
	}{
		{"realistic", func(*Setup) {}, true},
		{"unusual angles", func(s *Setup) { s.Data.Suspension.Front.CamberDeg = floatPtr(-1000) }, true},
		{"empty title", func(s *Setup) { s.Title = "  " }, false},
		{"long title", func(s *Setup) { s.Title = strings.Repeat("a", 151) }, false},
		{"long notes", func(s *Setup) { s.Notes = stringPtr(strings.Repeat("a", 10001)) }, false},
		{"multiline notes", func(s *Setup) { s.Notes = stringPtr("\nNormal free-form notes\n") }, true},
		{"visibility", func(s *Setup) { s.Visibility = "other" }, false},
		{"long technical", func(s *Setup) { s.Data.Electronics.Motor = strings.Repeat("a", 201) }, false},
		{"empty link name", func(s *Setup) { s.Data.Suspension.Front.LinkLengths[0].Name = " " }, false},
		{"zero link", func(s *Setup) { s.Data.Suspension.Front.LinkLengths[0].LengthMM = 0 }, false},
		{"negative link", func(s *Setup) { s.Data.Suspension.Front.LinkLengths[0].LengthMM = -1 }, false},
		{"zero oil", func(s *Setup) { s.Data.Shocks.Front.OilCST = floatPtr(0) }, false},
		{"negative oil", func(s *Setup) { s.Data.Shocks.Front.OilCST = floatPtr(-1) }, false},
		{"nonfinite", func(s *Setup) { s.Data.Suspension.Front.ToeDeg = floatPtr(math.Inf(1)) }, false},
		{"null byte", func(s *Setup) { s.Data.Electronics.Motor = "bad\x00value" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Setup{Title: " Title ", Visibility: Private, Data: *realisticData(t)}
			tc.modify(&s)
			err := normalize(&s)
			if (err == nil) != tc.valid {
				t.Fatal(err)
			}
			if tc.valid && s.Title != "Title" {
				t.Fatal("title not trimmed")
			}
		})
	}
}
