package setups

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestFixedStandardLengthsContract(t *testing.T) {
	raw := `{"suspension":{"front":{"upper_arm_length_mm":42.123456789,"lower_arm_length_mm":null,"steering_tie_rod_length_mm":16.2,"top_arm_spacing":null},"rear":{"upper_arm_length_mm":50,"lower_arm_length_mm":55.5}}}`
	var data DataV1
	if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
		t.Fatal(err)
	}
	if err := normalizeData(&data); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(data)
	if err != nil || !strings.Contains(string(encoded), `"upper_arm_length_mm":42.123456789`) || strings.Contains(string(encoded), "link_lengths") || strings.Contains(string(encoded), "null") {
		t.Fatal("fixed identities/unknown values lost", string(encoded), err)
	}
}

func TestStandardLengthValidation(t *testing.T) {
	for _, axle := range []string{"front", "rear"} {
		for _, field := range []string{"upper_arm_length_mm", "lower_arm_length_mm", "steering_tie_rod_length_mm"} {
			for _, value := range []string{"null", "16.2123456789", "0", "-1", `"16.2"`} {
				expected := value == "null" || (value == "16.2123456789" && (axle == "front" || field != "steering_tie_rod_length_mm"))
				var data DataV1
				err := decodeStrict(strings.NewReader(`{"suspension":{"`+axle+`":{"`+field+`":`+value+`}}}`), &data)
				if err == nil {
					err = normalizeData(&data)
				}
				if (err == nil) != expected {
					t.Fatal(axle, field, value, err)
				}
			}
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for i := 0; i < 3; i++ {
			axle := &AxleSuspension{}
			fields := []**float64{&axle.UpperArmLengthMM, &axle.LowerArmLengthMM, &axle.SteeringTieRodLengthMM}
			*fields[i] = &value
			if err := validateStandardLengths(axle, true); err == nil {
				t.Fatal("nonfinite standard length accepted")
			}
		}
	}
	for _, tc := range []struct{ field, key string }{{"upper_arm_length_mm", "upper_arm"}, {"lower_arm_length_mm", "lower_arm"}, {"steering_tie_rod_length_mm", "steering_tie_rod"}} {
		var data DataV1
		raw := `{"suspension":{"front":{"` + tc.field + `":42,"link_lengths":[{"name":"` + tc.key + `","length_mm":42}]}}}`
		if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&data); err == nil {
			t.Fatal("duplicate canonical representations accepted", raw)
		}
	}
	// Both identical and conflicting/duplicate legacy labels remain untouched when
	// no typed value was explicitly set; never guess which custom entry to promote.
	for _, raw := range []string{`{"suspension":{"front":{"link_lengths":[{"name":"upper_arm","length_mm":42},{"name":"upper_arm","length_mm":43}]}}}`, `{"suspension":{"rear":{"link_lengths":[{"name":"steering_tie_rod","length_mm":16.2}]}}}`} {
		var data DataV1
		if err := decodeStrict(strings.NewReader(raw), &data); err != nil {
			t.Fatal(err)
		}
		if err := normalizeData(&data); err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(data)
		if err != nil || string(body) != raw {
			t.Fatal("legacy reinterpretation", string(body), err)
		}
	}
}
