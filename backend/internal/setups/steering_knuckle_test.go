package setups

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSteeringKnuckleNormalization(t *testing.T) {
	id := "abcdefab-cdef-4abc-8def-abcdefabcdef"
	for _, tc := range []struct{ raw, want string }{
		{`{}`, `{}`}, {`{"front_steering_knuckle":null}`, `{}`}, {`{"front_steering_knuckle":{}}`, `{}`},
		{`{"front_steering_knuckle":{"manufacturer_id":null,"description":null}}`, `{}`},
		{`{"front_steering_knuckle":{"description":" \t "}}`, `{}`},
		{`{"front_steering_knuckle":{"manufacturer_id":"` + strings.ToUpper(id) + `"}}`, `{"front_steering_knuckle":{"manufacturer_id":"` + id + `"}}`},
		{`{"front_steering_knuckle":{"description":" Part #42\nAluminum "}}`, `{"front_steering_knuckle":{"description":"Part #42\nAluminum"}}`},
		{realisticJSON, realisticJSON},
	} {
		t.Run(tc.raw[:min(len(tc.raw), 100)], func(t *testing.T) {
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
			if err := decodeStrict(strings.NewReader(string(raw)), &roundtrip); err != nil || !reflect.DeepEqual(got, roundtrip) {
				t.Fatal(string(raw), err)
			}
		})
	}
	data := realisticData(t)
	data.FrontSteeringKnuckle = &SteeringKnuckle{ManufacturerID: &id, Description: strings.Repeat("😀", 200)}
	if err := normalizeData(data); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var got DataV1
	if err := decodeStrict(strings.NewReader(string(raw)), &got); err != nil || !reflect.DeepEqual(*data, got) {
		t.Fatal("full document round trip", err)
	}
}

func TestSteeringKnuckleInvalidInput(t *testing.T) {
	for _, raw := range []string{
		`{"front_steering_knuckle":{"manufacturer_id":""}}`, `{"front_steering_knuckle":{"manufacturer_id":"bad"}}`,
		`{"front_steering_knuckle":{"manufacturer_id":42}}`, `{"front_steering_knuckle":{"description":true}}`,
		`{"front_steering_knuckle":{"description":"bad\u0000text"}}`,
		`{"front_steering_knuckle":{"description":"` + strings.Repeat("😀", 201) + `"}}`,
		`{"front_steering_knuckle":{"rear":{}}}`, `{"front_steering_knuckle":[]}`, `{"rear_steering_knuckle":{}}`,
	} {
		var data DataV1
		err := decodeStrict(strings.NewReader(raw), &data)
		if err == nil {
			err = normalizeData(&data)
		}
		if err == nil {
			t.Fatal("invalid accepted", raw)
		}
	}
}
