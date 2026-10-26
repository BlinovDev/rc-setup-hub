package setups

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// These are public schema-v1 inputs/outputs, independent of normalization helpers.
func TestManufacturerNormalizationContract(t *testing.T) {
	id := "abcdefab-cdef-4abc-8def-abcdefabcdef"
	for _, tc := range []struct{ pattern, field string }{
		{`{"rear_differential":{"manufacturer_id":%s}}`, "rear differential"},
		{`{"front_steering_knuckle":{"manufacturer_id":%s}}`, "front steering knuckle"},
		{`{"suspension":{"front":{"lower_arm_mount":{"manufacturer_id":%s}}}}`, "lower arm mount"},
		{`{"suspension":{"rear":{"lower_arm_mount":{"manufacturer_id":%s}}}}`, "lower arm mount"},
		{`{"running_gear":{"front":{"wheel":{"manufacturer_id":%s}}}}`, "running gear"},
		{`{"running_gear":{"front":{"tire":{"manufacturer_id":%s}}}}`, "running gear"},
		{`{"running_gear":{"rear":{"wheel":{"manufacturer_id":%s}}}}`, "running gear"},
		{`{"running_gear":{"rear":{"tire":{"manufacturer_id":%s}}}}`, "running gear"},
	} {
		for _, value := range []string{`"` + strings.ToUpper(id) + `"`, `null`, `""`, `"invalid"`} {
			raw := fmt.Sprintf(tc.pattern, value)
			t.Run(raw, func(t *testing.T) {
				var data DataV1
				if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
					t.Fatal(err)
				}
				err := normalizeData(&data)
				if value == `""` || value == `"invalid"` {
					if err == nil || err.Error() != tc.field+" manufacturer_id must be a UUID" {
						t.Fatalf("error changed: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf(tc.pattern, `"`+id+`"`)
				if value == `null` {
					want = `{}`
				}
				if string(encoded) != want {
					t.Fatalf("got %s, want %s", encoded, want)
				}
			})
		}
	}
}

func TestManufacturerReferenceSlots(t *testing.T) {
	var data DataV1
	data.RearDifferential = &RearDifferential{ManufacturerID: stringPtr("differential")}
	data.Suspension = &Suspension{
		Front: &AxleSuspension{LowerArmMount: &LowerArmMount{ManufacturerID: stringPtr("front mount")}},
		Rear:  &AxleSuspension{LowerArmMount: &LowerArmMount{ManufacturerID: stringPtr("rear mount")}},
	}
	data.FrontSteeringKnuckle = &SteeringKnuckle{ManufacturerID: stringPtr("knuckle")}
	data.RunningGear = &RunningGear{
		Front: &AxleRunningGear{Wheel: &Wheel{ManufacturerID: stringPtr("front wheel")}, Tire: &Tire{ManufacturerID: stringPtr("front tire")}},
		Rear:  &AxleRunningGear{Wheel: &Wheel{ManufacturerID: stringPtr("rear wheel")}, Tire: &Tire{ManufacturerID: stringPtr("rear tire")}},
	}
	want := []string{"differential", "front mount", "rear mount", "knuckle", "front wheel", "front tire", "rear wheel", "rear tire"}
	labels := []string{"rear differential", "front lower arm mount", "rear lower arm mount", "front steering knuckle", "front wheel", "front tire", "rear wheel", "rear tire"}
	for i, ref := range manufacturerReferences(data) {
		if ref.component != labels[i] || ref.id == nil || *ref.id != want[i] {
			t.Fatalf("slot %d: %+v", i, ref)
		}
	}
	// Missing earlier parts must not shift later historical IDs into different slots.
	data.RearDifferential = nil
	data.Suspension.Front = nil
	data.FrontSteeringKnuckle = nil
	data.RunningGear.Front.Wheel = nil
	for i, ref := range manufacturerReferences(data) {
		missing := i == 0 || i == 1 || i == 3 || i == 4
		if ref.component != labels[i] || (ref.id == nil) != missing {
			t.Fatalf("removed slot %d: %+v", i, ref)
		}
		if !missing && *ref.id != want[i] {
			t.Fatalf("shifted historical reference %d", i)
		}
	}
	for _, ref := range manufacturerReferences(DataV1{}) {
		if ref.id != nil {
			t.Fatal("empty data has manufacturer IDs")
		}
	}
}
