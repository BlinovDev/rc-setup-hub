package setups

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestNamedLinksMayHaveUnknownLengths(t *testing.T) {
	for _, value := range []string{`{"name":"custom link"}`, `{"name":"custom link","length_mm":null}`} {
		for _, axle := range []string{"front", "rear"} {
			t.Run(axle+value, func(t *testing.T) {
				var data DataV1
				raw := `{"suspension":{"` + axle + `":{"link_lengths":[` + value + `]}}}`
				if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
					t.Fatal(err)
				}
				if err := normalizeData(&data); err != nil {
					t.Fatalf("named unknown length rejected: %v", err)
				}
				encoded, err := json.Marshal(data)
				if err != nil || strings.Contains(string(encoded), "length_mm") || !strings.Contains(string(encoded), `"name":"custom link"`) {
					t.Fatalf("missing length became a number: %s %v", encoded, err)
				}
			})
		}
	}
}

func TestLinkLengthValidationAndNoDefaults(t *testing.T) {
	for _, axle := range []string{"front", "rear"} {
		for _, tc := range []struct {
			name, entries string
			valid         bool
		}{
			{"standard and custom", `[{"name":"upper_arm","length_mm":16.2123456789},{"name":"lower_arm"},{"name":"steering_tie_rod","length_mm":3.5},{"name":"Custom","length_mm":null}]`, true},
			{"legacy duplicate labels", `[{"name":"Old upper label","length_mm":10},{"name":"Old upper label","length_mm":12}]`, true},
			{"empty", `[]`, true}, {"null", `null`, true},
			{"zero", `[{"name":"upper_arm","length_mm":0}]`, false}, {"negative", `[{"name":"upper_arm","length_mm":-0.1}]`, false},
			{"no name", `[{"length_mm":12}]`, false}, {"blank name", `[{"name":"  "}]`, false},
			{"nul name", `[{"name":"bad\u0000name"}]`, false},
			{"long name", `[{"name":"` + strings.Repeat("界", 201) + `"}]`, false},
			{"unicode name", `[{"name":"` + strings.Repeat("界", 200) + `","length_mm":0.1}]`, true},
			{"string", `[{"name":"x","length_mm":"16.2"}]`, false},
			{"unknown field", `[{"name":"x","length_mm":16.2,"kind":"arm"}]`, false},
		} {
			t.Run(axle+"/"+tc.name, func(t *testing.T) {
				var data DataV1
				err := decodeStrict(strings.NewReader(`{"suspension":{"`+axle+`":{"camber_deg":0,"link_lengths":`+tc.entries+`}}}`), &data)
				if err == nil {
					err = normalizeData(&data)
				}
				if (err == nil) != tc.valid {
					t.Fatalf("valid=%v error=%v", tc.valid, err)
				}
			})
		}
	}
	for _, raw := range []string{`{}`, `{"suspension":{"front":{"camber_deg":0},"rear":{"toe_deg":-1.5}}}`} {
		var data DataV1
		if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&data); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(data)
		if err != nil || strings.Contains(string(encoded), "link_lengths") {
			t.Fatal("defaults injected", string(encoded), err)
		}
	}
}

func TestLinkLengthCountAndFiniteRules(t *testing.T) {
	for _, count := range []int{100, 101} {
		links := make([]LinkLength, count)
		for i := range links {
			links[i] = LinkLength{Name: "custom", LengthMM: floatPtr(1.23456789)}
		}
		for _, axle := range []string{"front", "rear"} {
			data := DataV1{Suspension: &Suspension{}}
			value := &AxleSuspension{LinkLengths: links}
			if axle == "front" {
				data.Suspension.Front = value
			} else {
				data.Suspension.Rear = value
			}
			if err := normalizeData(&data); (err == nil) != (count == 100) {
				t.Fatal(count, axle, err)
			}
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		data := DataV1{Suspension: &Suspension{Front: &AxleSuspension{LinkLengths: []LinkLength{{Name: "x", LengthMM: &value}}}}}
		if err := normalizeData(&data); err == nil {
			t.Fatal("nonfinite link accepted", value)
		}
	}
	// Reads of an old numeric zero retain its presence, although new writes reject it.
	var old LinkLength
	if err := json.Unmarshal([]byte(`{"name":"old zero","length_mm":0}`), &old); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(old)
	if err != nil || old.LengthMM == nil || *old.LengthMM != 0 || !strings.Contains(string(encoded), `"length_mm":0`) {
		t.Fatal("legacy numeric presence lost", string(encoded), err)
	}
}
