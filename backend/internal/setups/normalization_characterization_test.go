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
