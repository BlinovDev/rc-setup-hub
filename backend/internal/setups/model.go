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

type DataV1 struct {
	Suspension  *Suspension  `json:"suspension,omitempty"`
	Shocks      *Shocks      `json:"shocks,omitempty"`
	Electronics *Electronics `json:"electronics,omitempty"`
}
type Suspension struct {
	Front *AxleSuspension `json:"front,omitempty"`
	Rear  *AxleSuspension `json:"rear,omitempty"`
}
type AxleSuspension struct {
	CamberDeg   *float64     `json:"camber_deg,omitempty"`
	CasterDeg   *float64     `json:"caster_deg,omitempty"`
	ToeDeg      *float64     `json:"toe_deg,omitempty"`
	LinkLengths []LinkLength `json:"link_lengths,omitempty"`
}
type LinkLength struct {
	Name     string  `json:"name"`
	LengthMM float64 `json:"length_mm"`
}
type Shocks struct {
	Front *Shock `json:"front,omitempty"`
	Rear  *Shock `json:"rear,omitempty"`
}
type Shock struct {
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	Spring       *Spring  `json:"spring,omitempty"`
	OilCST       *float64 `json:"oil_cst,omitempty"`
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
type Setup struct {
	ID             string     `json:"id"`
	OwnerID        string     `json:"-"`
	ChassisModelID *string    `json:"chassis_model_id"`
	Title          string     `json:"title"`
	Visibility     Visibility `json:"visibility"`
	Data           DataV1     `json:"data"`
	Notes          *string    `json:"notes"`
	SchemaVersion  int        `json:"schema_version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
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
