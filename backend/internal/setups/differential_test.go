package setups

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRearDifferentialNormalization(t *testing.T) {
	id := "abcdefab-cdef-4abc-8def-abcdefabcdef"
	for _, tc := range []struct{ name, raw, want string }{
		{"legacy", realisticJSON, realisticJSON},
		{"omitted", `{}`, `{}`},
		{"null", `{"rear_differential":null}`, `{}`},
		{"empty", `{"rear_differential":{}}`, `{}`},
		{"null fields", `{"rear_differential":{"type":null,"manufacturer_id":null,"model":null,"tuning_notes":null}}`, `{}`},
		{"blank text", `{"rear_differential":{"model":" \t ","tuning_notes":"\n "}}`, `{}`},
		{"manufacturer only", `{"rear_differential":{"manufacturer_id":"` + strings.ToUpper(id) + `"}}`, `{"rear_differential":{"manufacturer_id":"` + id + `"}}`},
		{"model only", `{"rear_differential":{"model":" Model "}}`, `{"rear_differential":{"model":"Model"}}`},
		{"notes only", `{"rear_differential":{"tuning_notes":" Oil 10000\nTwo gears "}}`, `{"rear_differential":{"tuning_notes":"Oil 10000\nTwo gears"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got, want DataV1
			if err := decodeStrict(strings.NewReader(tc.raw), &got); err != nil {
				t.Fatal(err)
			}
			if err := decodeStrict(strings.NewReader(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if err := normalizeData(&got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v want %+v", got, want)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var roundtrip DataV1
			if err := decodeStrict(bytes.NewReader(raw), &roundtrip); err != nil || !reflect.DeepEqual(got, roundtrip) {
				t.Fatal("round trip", string(raw), err)
			}
			if tc.name == "legacy" && (strings.Contains(string(raw), "rear_differential") || got.Suspension.Rear.ToeDeg == nil || *got.Suspension.Rear.ToeDeg != 0) {
				t.Fatal("legacy data/zero changed", string(raw))
			}
		})
	}
	for _, kind := range []DifferentialType{Gear, LSD, Ball, Spool} {
		t.Run(string(kind), func(t *testing.T) {
			data := realisticData(t)
			data.RearDifferential = &RearDifferential{Type: &kind, ManufacturerID: &id, Model: strings.Repeat("界", 200), TuningNotes: strings.Repeat("界", 10000)}
			if err := normalizeData(data); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			var got DataV1
			if err := decodeStrict(bytes.NewReader(raw), &got); err != nil || !reflect.DeepEqual(*data, got) {
				t.Fatal("full differential round trip", err)
			}
		})
	}
}

func TestRearDifferentialInvalidInput(t *testing.T) {
	for _, bad := range []string{
		`{"type":"other"}`, `{"type":"spul"}`, `{"type":""}`, `{"type":"Gear"}`, `{"type":" gear "}`,
		`{"type":1}`, `{"type":true}`, `{"manufacturer_id":""}`, `{"manufacturer_id":"not-a-uuid"}`,
		`{"manufacturer_id":1}`, `{"manufacturer_id":{}}`, `{"model":1}`, `{"tuning_notes":[]}`,
		`{"model":"bad\u0000text"}`, `{"tuning_notes":"bad\u0000text"}`,
		`{"model":"` + strings.Repeat("界", 201) + `"}`, `{"tuning_notes":"` + strings.Repeat("界", 10001) + `"}`,
		`{"front":{}}`, `{"oil_cst":1000}`, `[]`, `"gear"`,
	} {
		t.Run(bad[:min(len(bad), 80)], func(t *testing.T) {
			var data DataV1
			err := decodeStrict(strings.NewReader(`{"rear_differential":`+bad+`}`), &data)
			if err == nil {
				err = normalizeData(&data)
				if err != nil {
					var validation *ValidationError
					if !errors.As(err, &validation) {
						t.Fatal("not a validation error", err)
					}
				}
			}
			if err == nil {
				t.Fatal("invalid differential accepted")
			}
		})
	}
	var data DataV1
	if err := decodeStrict(strings.NewReader(`{"front_differential":{"type":"gear"}}`), &data); err == nil {
		t.Fatal("front differential accepted")
	}
}
