package setups

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestShockHardwareNormalization(t *testing.T) {
	for _, axle := range []string{"front", "rear"} {
		t.Run(axle, func(t *testing.T) {
			for _, tc := range []struct{ name, input, want string }{
				{"empty piston", `{"piston":{}}`, `{}`},
				{"null piston", `{"piston":null}`, `{}`},
				{"null piston values", `{"piston":{"holes":null,"hole_diameter_mm":null,"notes":null}}`, `{}`},
				{"empty seal", `{"seal":{}}`, `{}`},
				{"null seal", `{"seal":null}`, `{}`},
				{"null seal values", `{"seal":{"type":null,"description":null}}`, `{}`},
				{"blank hardware", `{"piston":{"notes":" \n "},"seal":{"description":" \t "}}`, `{}`},
				{"legacy empty shock", `{}`, `{"shocks":{"` + axle + `":{}}}`},
				{"legacy spring preserved", `{"piston":null,"seal":{},"spring":{}}`, `{"shocks":{"` + axle + `":{"spring":{}}}}`},
				{"hole count only", `{"piston":{"holes":4}}`, `{"shocks":{"` + axle + `":{"piston":{"holes":4}}}}`},
				{"diameter only", `{"piston":{"hole_diameter_mm":0.123456789}}`, `{"shocks":{"` + axle + `":{"piston":{"hole_diameter_mm":0.123456789}}}}`},
				{"notes only", `{"piston":{"notes":" Drilled piston\nThree holes "}}`, `{"shocks":{"` + axle + `":{"piston":{"notes":"Drilled piston\nThree holes"}}}}`},
				{"o ring", `{"seal":{"type":"o-ring"}}`, `{"shocks":{"` + axle + `":{"seal":{"type":"o-ring"}}}}`},
				{"x ring", `{"seal":{"type":"x-ring"}}`, `{"shocks":{"` + axle + `":{"seal":{"type":"x-ring"}}}}`},
				{"description only", `{"seal":{"description":" Product model "}}`, `{"shocks":{"` + axle + `":{"seal":{"description":"Product model"}}}}`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var got, want DataV1
					input := `{"shocks":{"` + axle + `":` + tc.input + `}}`
					if err := decodeStrict(strings.NewReader(input), &got); err != nil {
						t.Fatal(err)
					}
					if err := decodeStrict(strings.NewReader(tc.want), &want); err != nil {
						t.Fatal(err)
					}
					if err := normalizeData(&got); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("normalized %s got %+v want %+v", input, got, want)
					}
					raw, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					var roundtrip DataV1
					if err := decodeStrict(bytes.NewReader(raw), &roundtrip); err != nil || !reflect.DeepEqual(roundtrip, got) {
						t.Fatal("round trip", string(raw), err)
					}
				})
			}
		})
	}
	// Empty blocks on one side must not prune the other side or any legacy technical values.
	data := realisticData(t)
	before, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	data.Shocks.Front.Piston = &ShockPiston{}
	data.Shocks.Rear.Seal = &ShockSeal{}
	if err := normalizeData(data); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(data)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("legacy shock/spring/oil/zero data changed", string(after), err)
	}
}

func TestShockHardwareBounds(t *testing.T) {
	max := int32(math.MaxInt32)
	typ := XRing
	data := realisticData(t)
	data.Shocks.Front.Piston = &ShockPiston{Holes: &max, HoleDiameterMM: floatPtr(0.123456789), Notes: strings.Repeat("界", 10000)}
	data.Shocks.Rear.Seal = &ShockSeal{Type: &typ, Description: strings.Repeat("界", 200)}
	if err := normalizeData(data); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var got DataV1
	if err := decodeStrict(bytes.NewReader(raw), &got); err != nil || !reflect.DeepEqual(*data, got) {
		t.Fatal("boundaries/precision", err)
	}
	for _, diameter := range []float64{0, -1, math.Inf(1), math.Inf(-1), math.NaN()} {
		d := DataV1{Shocks: &Shocks{Rear: &Shock{Piston: &ShockPiston{HoleDiameterMM: &diameter}}}}
		if err := normalizeData(&d); err == nil {
			t.Fatal("invalid diameter", diameter)
		}
	}
}

func TestShockHardwareInvalidInput(t *testing.T) {
	for _, axle := range []string{"front", "rear"} {
		for i, shock := range []string{
			`{"piston":{"holes":0}}`, `{"piston":{"holes":-1}}`, `{"piston":{"holes":1.5}}`, `{"piston":{"holes":"4"}}`, `{"piston":{"holes":true}}`, `{"piston":{"holes":2147483648}}`,
			`{"piston":{"hole_diameter_mm":0}}`, `{"piston":{"hole_diameter_mm":-1}}`, `{"piston":{"hole_diameter_mm":"1.2"}}`, `{"piston":{"hole_diameter_mm":1e999}}`, `{"piston":{"hole_diameter_mm":NaN}}`, `{"piston":{"hole_diameter_mm":Infinity}}`,
			`{"piston":{"notes":"bad\u0000text"}}`, `{"piston":{"notes":"` + strings.Repeat("界", 10001) + `"}}`,
			`{"piston":{"unknown":1}}`, `{"piston":[]}`,
			`{"seal":{"type":"other"}}`, `{"seal":{"type":""}}`, `{"seal":{"type":"O-ring"}}`, `{"seal":{"type":1}}`,
			`{"seal":{"description":"bad\u0000text"}}`, `{"seal":{"description":"` + strings.Repeat("界", 201) + `"}}`, `{"seal":{"description":42}}`, `{"seal":{"unknown":1}}`, `{"seal":[]}`,
		} {
			t.Run(fmt.Sprintf("%s/%d", axle, i), func(t *testing.T) {
				var data DataV1
				err := decodeStrict(strings.NewReader(`{"shocks":{"`+axle+`":`+shock+`}}`), &data)
				if err == nil {
					err = normalizeData(&data)
				}
				if err == nil {
					t.Fatal("invalid piston/seal accepted")
				}
			})
		}
	}
}
