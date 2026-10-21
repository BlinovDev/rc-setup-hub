package setups

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const SchemaVersion = 1

type Visibility string

const (
	Public  Visibility = "public"
	Friends Visibility = "friends"
	Private Visibility = "private"
)

var (
	ErrNotFound          = errors.New("setup not found")
	ErrConflict          = errors.New("setup changed; reload and retry")
	ErrInvalidChassis    = errors.New("chassis model must exist and both model and brand must be active")
	ErrUnsupportedSchema = errors.New("unsupported setup schema version")
)

// DifferentialType identifies a rear differential without prescribing its tuning controls.
type DifferentialType string

const (
	Gear  DifferentialType = "gear"
	LSD   DifferentialType = "lsd"
	Ball  DifferentialType = "ball"
	Spool DifferentialType = "spool"
)

type RearDifferential struct {
	Type           *DifferentialType `json:"type,omitempty"`
	ManufacturerID *string           `json:"manufacturer_id,omitempty"`
	Model          string            `json:"model,omitempty"`
	TuningNotes    string            `json:"tuning_notes,omitempty"`
}

type DataV1 struct {
	RearDifferential *RearDifferential `json:"rear_differential,omitempty"`
	Suspension       *Suspension       `json:"suspension,omitempty"`
	Shocks           *Shocks           `json:"shocks,omitempty"`
	Electronics      *Electronics      `json:"electronics,omitempty"`
}
type Suspension struct {
	Front *AxleSuspension `json:"front,omitempty"`
	Rear  *AxleSuspension `json:"rear,omitempty"`
}

// ArmSpacing stores symmetric spacer totals ahead of and behind an arm, in mm.
type ArmSpacing struct {
	FrontMM *float64 `json:"front_mm,omitempty"`
	RearMM  *float64 `json:"rear_mm,omitempty"`
}

// LowerArmMount is one descriptive mount configuration per axle, not an insert/geometry model.
type LowerArmMount struct {
	ManufacturerID     *string `json:"manufacturer_id,omitempty"`
	Model              string  `json:"model,omitempty"`
	ConfigurationNotes string  `json:"configuration_notes,omitempty"`
}

type AxleSuspension struct {
	LowerArmMount   *LowerArmMount `json:"lower_arm_mount,omitempty"`
	TopArmSpacing   *ArmSpacing    `json:"top_arm_spacing,omitempty"`
	LowerArmSpacing *ArmSpacing    `json:"lower_arm_spacing,omitempty"`
	CamberDeg       *float64       `json:"camber_deg,omitempty"`
	CasterDeg       *float64       `json:"caster_deg,omitempty"`
	ToeDeg          *float64       `json:"toe_deg,omitempty"`
	LinkLengths     []LinkLength   `json:"link_lengths,omitempty"`
}

// UnmarshalJSON tracks explicit null blocks so normalization can omit their empty parents.
// The alias retains strict unknown-field decoding, including nested spacing fields.
func (a *AxleSuspension) UnmarshalJSON(raw []byte) error {
	type axle AxleSuspension
	var decoded axle
	if err := decodeStrict(bytes.NewReader(raw), &decoded); err != nil {
		return err
	}
	var presence struct {
		Top   json.RawMessage `json:"top_arm_spacing"`
		Lower json.RawMessage `json:"lower_arm_spacing"`
		Mount json.RawMessage `json:"lower_arm_mount"`
	}
	if err := json.Unmarshal(raw, &presence); err != nil {
		return err
	}
	*a = AxleSuspension(decoded)
	if bytes.Equal(bytes.TrimSpace(presence.Top), []byte("null")) {
		a.TopArmSpacing = &ArmSpacing{}
	}
	if bytes.Equal(bytes.TrimSpace(presence.Lower), []byte("null")) {
		a.LowerArmSpacing = &ArmSpacing{}
	}
	if bytes.Equal(bytes.TrimSpace(presence.Mount), []byte("null")) {
		a.LowerArmMount = &LowerArmMount{}
	}
	return nil
}

type LinkLength struct {
	Name     string  `json:"name"`
	LengthMM float64 `json:"length_mm"`
}
type Shocks struct {
	Front *Shock `json:"front,omitempty"`
	Rear  *Shock `json:"rear,omitempty"`
}
type SealType string

const (
	ORing SealType = "o-ring"
	XRing SealType = "x-ring"
)

// ShockPiston describes independently optional piston measurements and tuning.
type ShockPiston struct {
	Holes          *int32   `json:"holes,omitempty"`
	HoleDiameterMM *float64 `json:"hole_diameter_mm,omitempty"`
	Notes          string   `json:"notes,omitempty"`
}

type ShockSeal struct {
	Type        *SealType `json:"type,omitempty"`
	Description string    `json:"description,omitempty"`
}

type Shock struct {
	Piston       *ShockPiston `json:"piston,omitempty"`
	Seal         *ShockSeal   `json:"seal,omitempty"`
	Manufacturer string       `json:"manufacturer,omitempty"`
	Model        string       `json:"model,omitempty"`
	Spring       *Spring      `json:"spring,omitempty"`
	OilCST       *float64     `json:"oil_cst,omitempty"`
}

// UnmarshalJSON retains explicit null hardware blocks until normalization can prune
// their newly empty parents, without changing legacy empty shock documents.
func (s *Shock) UnmarshalJSON(raw []byte) error {
	type shock Shock
	var decoded shock
	if err := decodeStrict(bytes.NewReader(raw), &decoded); err != nil {
		return err
	}
	var presence struct {
		Piston json.RawMessage `json:"piston"`
		Seal   json.RawMessage `json:"seal"`
	}
	if err := json.Unmarshal(raw, &presence); err != nil {
		return err
	}
	*s = Shock(decoded)
	if bytes.Equal(bytes.TrimSpace(presence.Piston), []byte("null")) {
		s.Piston = &ShockPiston{}
	}
	if bytes.Equal(bytes.TrimSpace(presence.Seal), []byte("null")) {
		s.Seal = &ShockSeal{}
	}
	return nil
}

type Spring struct {
	Manufacturer string `json:"manufacturer,omitempty"`
	Color        string `json:"color,omitempty"`
}
type Electronics struct {
	Motor string `json:"motor,omitempty"`
	ESC   string `json:"esc,omitempty"`
	Servo string `json:"servo,omitempty"`
	Gyro  string `json:"gyro,omitempty"`
	Radio string `json:"radio,omitempty"`
}

// DifferentialManufacturer is the shared historical brand display shape, including inactive brands.
type DifferentialManufacturer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LowerArmMountManufacturers contains joined historical names, never client input.
type LowerArmMountManufacturers struct {
	Front *DifferentialManufacturer `json:"front"`
	Rear  *DifferentialManufacturer `json:"rear"`
}

type Setup struct {
	LowerArmMountManufacturers   *LowerArmMountManufacturers `json:"lower_arm_mount_manufacturers"`
	RearDifferentialManufacturer *DifferentialManufacturer   `json:"rear_differential_manufacturer"`
	CoverPhoto                   *Photo                      `json:"cover_photo"`
	ID                           string                      `json:"id"`
	OwnerID                      string                      `json:"owner_id"`
	ChassisModelID               *string                     `json:"chassis_model_id"`
	Chassis                      *SearchChassis              `json:"chassis"`
	Title                        string                      `json:"title"`
	Visibility                   Visibility                  `json:"visibility"`
	Data                         DataV1                      `json:"data"`
	Notes                        *string                     `json:"notes"`
	SchemaVersion                int                         `json:"schema_version"`
	CreatedAt                    time.Time                   `json:"created_at"`
	UpdatedAt                    time.Time                   `json:"updated_at"`
}
type CreateInput struct {
	Title          string     `json:"title"`
	ChassisModelID *string    `json:"chassis_model_id"`
	Visibility     Visibility `json:"visibility"`
	Notes          *string    `json:"notes"`
	Data           *DataV1    `json:"data"`
}

// NullableString distinguishes omission from a supplied value, including explicit null.
type NullableString struct {
	Present bool
	Value   *string
}

func (f *NullableString) UnmarshalJSON(raw []byte) error {
	f.Present = true
	return json.Unmarshal(raw, &f.Value)
}

type OptionalData struct {
	Present bool
	Value   *DataV1
}

func (f *OptionalData) UnmarshalJSON(raw []byte) error {
	f.Present = true
	return decodeStrict(bytes.NewReader(raw), &f.Value)
}

type PatchInput struct {
	Title          NullableString `json:"title"`
	ChassisModelID NullableString `json:"chassis_model_id"`
	Visibility     NullableString `json:"visibility"`
	Notes          NullableString `json:"notes"`
	Data           OptionalData   `json:"data"`
}

func decodeStrict(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return errors.New("expected one JSON object")
	}
	return nil
}
