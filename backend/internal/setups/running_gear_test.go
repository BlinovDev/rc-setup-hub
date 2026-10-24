package setups

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestRunningGearNormalization(t *testing.T) {
	for _, side := range []string{"front", "rear"} {
		for _, tc := range []struct {
			part, block string
			valid       bool
		}{
			{"hex", `{"offset_mm":0}`, true}, {"hex", `{"offset_mm":-5.23456789}`, true},
			{"wheel", `{"offset_mm":0}`, true}, {"wheel", `{"offset_mm":-2147483648}`, true},
			{"wheel", `{"offset_mm":2147483647}`, true}, {"wheel", `{"offset_mm":1.5}`, false},
			{"wheel", `{"offset_mm":2147483648}`, false}, {"wheel", `{"offset_mm":"1"}`, false},
			{"hex", `{"width_mm":5}`, false}, {"hex", `{"offset_mm":1e400}`, false},
			{"wheel", `{"model":" Part "}`, true}, {"tire", `{"model":" Part "}`, true},
			{"tire", `{"manufacturer_id":"bad"}`, false}, {"wheel", `{"manufacturer_id":"bad"}`, false},
			{"tire", `{"model":"bad\u0000value"}`, false},
			{"wheel", `{"model":"` + strings.Repeat("界", 200) + `"}`, true},
			{"tire", `{"model":"` + strings.Repeat("界", 201) + `"}`, false},
			{"tire", `{"offset_mm":1}`, false}, {"wheel", `[]`, false},
		} {
			raw := fmt.Sprintf(`{"running_gear":{"%s":{"%s":%s}}}`, side, tc.part, tc.block)
			var data DataV1
			err := decodeStrict(strings.NewReader(raw), &data)
			if err == nil {
				err = normalizeData(&data)
			}
			if (err == nil) != tc.valid {
				t.Fatalf("%s: %v", raw, err)
			}
			if err == nil {
				encoded, _ := json.Marshal(data)
				var again DataV1
				if err := decodeStrict(strings.NewReader(string(encoded)), &again); err != nil || !reflect.DeepEqual(data, again) {
					t.Fatalf("roundtrip %s: %v", encoded, err)
				}
			}
		}
	}
	for _, raw := range []string{`{"running_gear":null}`, `{"running_gear":{}}`, `{"running_gear":{"front":{"hex":{"offset_mm":null},"wheel":{"model":" "},"tire":{}},"rear":null}}`} {
		var data DataV1
		if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&data); err != nil || data.RunningGear != nil {
			t.Fatalf("empty %s: %+v %v", raw, data, err)
		}
	}
	zero := int32(0)
	preserved := DataV1{RunningGear: &RunningGear{Front: &AxleRunningGear{Hex: &Hex{OffsetMM: floatPtr(0)}, Wheel: &Wheel{OffsetMM: &zero}}}}
	if err := normalizeData(&preserved); err != nil || preserved.RunningGear == nil || preserved.RunningGear.Front.Hex == nil || preserved.RunningGear.Front.Wheel == nil {
		t.Fatal("explicit zero lost", err)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		data := DataV1{RunningGear: &RunningGear{Front: &AxleRunningGear{Hex: &Hex{OffsetMM: &value}}}}
		if normalizeData(&data) == nil {
			t.Fatal("nonfinite accepted")
		}
	}
	data := realisticData(t)
	data.RunningGear = &RunningGear{Front: &AxleRunningGear{Hex: &Hex{OffsetMM: floatPtr(0)}, Wheel: &Wheel{Model: "Wheel"}}, Rear: &AxleRunningGear{Tire: &Tire{Model: "Tire"}}}
	raw, _ := json.Marshal(data)
	var again DataV1
	if err := decodeStrict(strings.NewReader(string(raw)), &again); err != nil || !reflect.DeepEqual(*data, again) {
		t.Fatal("legacy sibling loss", err)
	}
}
