package setups

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestLowerArmMountNormalization(t *testing.T) {
	id := "abcdefab-cdef-4abc-8def-abcdefabcdef"
	for _, side := range []string{"front", "rear"} {
		wrap := func(block string) string {
			return fmt.Sprintf(`{"suspension":{"%s":{"lower_arm_mount":%s}}}`, side, block)
		}
		for _, tc := range []struct{ name, raw, want string }{
			{"legacy", realisticJSON, realisticJSON},
			{"legacy empty axle", fmt.Sprintf(`{"suspension":{"%s":{}}}`, side), fmt.Sprintf(`{"suspension":{"%s":{}}}`, side)},
			{"omitted", `{}`, `{}`}, {"null", wrap(`null`), `{}`}, {"empty", wrap(`{}`), `{}`},
			{"null fields", wrap(`{"manufacturer_id":null,"model":null,"configuration_notes":null}`), `{}`},
			{"blank fields", wrap(`{"model":" \t ","configuration_notes":"\n "}`), `{}`},
			{"manufacturer only", wrap(`{"manufacturer_id":"` + strings.ToUpper(id) + `"}`), wrap(`{"manufacturer_id":"` + id + `"}`)},
			{"model only", wrap(`{"model":" Part #42 "}`), wrap(`{"model":"Part #42"}`)},
			{"notes only", wrap(`{"configuration_notes":" Inserts A\nMount B "}`), wrap(`{"configuration_notes":"Inserts A\nMount B"}`)},
			{"preserve zero", fmt.Sprintf(`{"suspension":{"%s":{"lower_arm_mount":null,"toe_deg":0,"top_arm_spacing":{"front_mm":0,"rear_mm":5.23456789}}}}`, side), fmt.Sprintf(`{"suspension":{"%s":{"toe_deg":0,"top_arm_spacing":{"front_mm":0,"rear_mm":5.23456789}}}}`, side)},
			{"empty spacing retains mount", fmt.Sprintf(`{"suspension":{"%s":{"lower_arm_mount":{"model":"Part"},"top_arm_spacing":null,"lower_arm_spacing":{}}}}`, side), wrap(`{"model":"Part"}`)},
		} {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
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
				if err := normalizeData(&got); err != nil {
					t.Fatal(err)
				}
				again, err := json.Marshal(got)
				if err != nil || !bytes.Equal(raw, again) {
					t.Fatal("normalization not idempotent", err)
				}
				var read DataV1
				if err := decodeStrict(bytes.NewReader(raw), &read); err != nil || !reflect.DeepEqual(got, read) {
					t.Fatal("round trip", err)
				}
			})
		}
	}
	data := realisticData(t)
	data.Suspension.Front.LowerArmMount = &LowerArmMount{ManufacturerID: &id, Model: strings.Repeat("😀", 200), ConfigurationNotes: strings.Repeat("界", 10000)}
	data.Suspension.Rear.LowerArmMount = &LowerArmMount{Model: "Rear part", ConfigurationNotes: "Rear notes"}
	kind := Gear
	data.RearDifferential = &RearDifferential{Type: &kind, Model: "Diff"}
	holes := int32(6)
	diameter := 1.23456789
	seal := XRing
	data.Shocks.Front.Piston = &ShockPiston{Holes: &holes, HoleDiameterMM: &diameter, Notes: "Piston notes"}
	data.Shocks.Rear.Seal = &ShockSeal{Type: &seal, Description: "Seal product"}
	if err := normalizeData(data); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var read DataV1
	if err := decodeStrict(bytes.NewReader(raw), &read); err != nil || !reflect.DeepEqual(*data, read) {
		t.Fatal("complete data round trip", err)
	}
}

func TestLowerArmMountInvalidInput(t *testing.T) {
	for _, side := range []string{"front", "rear"} {
		for _, block := range []string{
			`[]`, `42`, `"Part"`, `{"manufacturer_id":""}`, `{"manufacturer_id":"bad"}`, `{"manufacturer_id":42}`,
			`{"model":true}`, `{"configuration_notes":[]}`, `{"model":"bad\u0000text"}`, `{"configuration_notes":"bad\u0000text"}`,
			`{"model":"` + strings.Repeat("😀", 201) + `"}`, `{"configuration_notes":"` + strings.Repeat("界", 10001) + `"}`,
			`{"mounts":[]}`, `{"inserts":[]}`, `{"geometry":{"toe":2}}`,
		} {
			var data DataV1
			raw := fmt.Sprintf(`{"suspension":{"%s":{"lower_arm_mount":%s}}}`, side, block)
			err := decodeStrict(strings.NewReader(raw), &data)
			if err == nil {
				err = normalizeData(&data)
			}
			if err == nil {
				t.Fatal("invalid accepted", raw)
			}
		}
	}
}
