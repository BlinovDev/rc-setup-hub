package setups

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestArmSpacingNormalization(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"legacy", `{"suspension":{"front":{}}}`, `{"suspension":{"front":{}}}`},
		{"example", `{"suspension":{"front":{"top_arm_spacing":{"front_mm":0,"rear_mm":5.2}}}}`, `{"suspension":{"front":{"top_arm_spacing":{"front_mm":0,"rear_mm":5.2}}}}`},
		{"both zero", `{"suspension":{"rear":{"lower_arm_spacing":{"front_mm":0,"rear_mm":0}}}}`, `{"suspension":{"rear":{"lower_arm_spacing":{"front_mm":0,"rear_mm":0}}}}`},
		{"partial", `{"suspension":{"rear":{"top_arm_spacing":{"front_mm":null,"rear_mm":0.123456789}}}}`, `{"suspension":{"rear":{"top_arm_spacing":{"rear_mm":0.123456789}}}}`},
		{"empty", `{"suspension":{"front":{"top_arm_spacing":{},"lower_arm_spacing":{"front_mm":null,"rear_mm":null}}}}`, `{}`},
		{"null", `{"suspension":{"rear":{"top_arm_spacing":null,"lower_arm_spacing":null}}}`, `{}`},
		{"keep sibling", `{"suspension":{"front":{"toe_deg":0,"top_arm_spacing":{},"lower_arm_spacing":{"front_mm":1.2}}}}`, `{"suspension":{"front":{"lower_arm_spacing":{"front_mm":1.2},"toe_deg":0}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data DataV1
			if err := decodeStrict(strings.NewReader(tc.input), &data); err != nil {
				t.Fatal(err)
			}
			if err := normalizeData(&data); err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
			if err := normalizeData(&data); err != nil {
				t.Fatal(err)
			}
			again, _ := json.Marshal(data)
			if string(again) != tc.want {
				t.Fatal("not idempotent", string(again))
			}
		})
	}
}

func TestArmSpacingValidation(t *testing.T) {
	for _, rearAxle := range []bool{false, true} {
		for _, lowerArm := range []bool{false, true} {
			for _, rearValue := range []bool{false, true} {
				for _, value := range []float64{0, 5.2, 0.123456789, -0.1, math.Inf(1), math.Inf(-1), math.NaN()} {
					spacing := &ArmSpacing{}
					if rearValue {
						spacing.RearMM = &value
					} else {
						spacing.FrontMM = &value
					}
					axle := &AxleSuspension{}
					if lowerArm {
						axle.LowerArmSpacing = spacing
					} else {
						axle.TopArmSpacing = spacing
					}
					data := DataV1{Suspension: &Suspension{}}
					if rearAxle {
						data.Suspension.Rear = axle
					} else {
						data.Suspension.Front = axle
					}
					if err := normalizeData(&data); (err == nil) != (finite(value) && value >= 0) {
						t.Fatalf("value %v: %v", value, err)
					}
				}
			}
		}
	}
}

func TestArmSpacingStrictDecode(t *testing.T) {
	for _, raw := range []string{`{"front_mm":"5.2"}`, `{"front_mm":true}`, `{"front_mm":[]}`, `{"front_mm":1e999}`, `{"front_mm":NaN}`, `{"front_mm":Infinity}`, `{"unknown":1}`, `[]`, `5`} {
		body := `{"suspension":{"front":{"top_arm_spacing":` + raw + `}}}`
		var data DataV1
		if err := decodeStrict(strings.NewReader(body), &data); err == nil {
			t.Fatalf("accepted %s", body)
		}
		var patch PatchInput
		if err := decodeStrict(strings.NewReader(`{"data":`+body+`}`), &patch); err == nil {
			t.Fatalf("PATCH accepted %s", body)
		}
	}
	var data DataV1
	if err := decodeStrict(strings.NewReader(`{"suspension":{"front":{"unknown":1}}}`), &data); err == nil {
		t.Fatal("unknown axle field accepted")
	}
}
