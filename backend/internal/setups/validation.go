package setups

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func invalid(message string) error       { return &ValidationError{Message: message} }
func validText(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsRune(value, 0)
}
func normalize(s *Setup) error {
	s.Title = strings.TrimSpace(s.Title)
	if s.Title == "" || !validText(s.Title, 150) {
		return invalid("title must contain 1 to 150 characters")
	}
	if s.Notes != nil && !validText(*s.Notes, 10000) {
		return invalid("notes must contain at most 10000 characters without null bytes")
	}
	if s.Visibility != Public && s.Visibility != Friends && s.Visibility != Private {
		return invalid("visibility must be public, friends or private")
	}
	return normalizeData(&s.Data)
}
func technical(value *string) error {
	*value = strings.TrimSpace(*value)
	if !validText(*value, 200) {
		return invalid("technical strings must contain at most 200 characters without null bytes")
	}
	return nil
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func normalizeData(data *DataV1) error {
	if err := normalizeRunningGear(data); err != nil {
		return err
	}
	if k := data.FrontSteeringKnuckle; k != nil {
		id, err := canonicalUUID(k.ManufacturerID, "front steering knuckle manufacturer_id")
		if err != nil {
			return err
		}
		k.ManufacturerID = id
		if err := technical(&k.Description); err != nil {
			return err
		}
		if k.ManufacturerID == nil && k.Description == "" {
			data.FrontSteeringKnuckle = nil
		}
	}
	if d := data.RearDifferential; d != nil {
		if d.Type != nil && *d.Type != Gear && *d.Type != LSD && *d.Type != Ball && *d.Type != Spool {
			return invalid("rear differential type must be gear, lsd, ball or spool")
		}
		id, err := canonicalUUID(d.ManufacturerID, "rear differential manufacturer_id")
		if err != nil {
			return err
		}
		d.ManufacturerID = id
		if err := technical(&d.Model); err != nil {
			return err
		}
		d.TuningNotes = strings.TrimSpace(d.TuningNotes)
		if !validText(d.TuningNotes, 10000) {
			return invalid("rear differential tuning_notes must contain at most 10000 characters without null bytes")
		}
		if d.Type == nil && d.ManufacturerID == nil && d.Model == "" && d.TuningNotes == "" {
			data.RearDifferential = nil
		}
	}
	if err := normalizeSuspension(data); err != nil {
		return err
	}
	if err := normalizeShocks(data); err != nil {
		return err
	}
	if data.Electronics != nil {
		e := data.Electronics
		for _, text := range []*string{&e.Motor, &e.ESC, &e.Servo, &e.Gyro, &e.Radio} {
			if err := technical(text); err != nil {
				return err
			}
		}
	}
	return nil
}
func normalizeShockHardware(shock *Shock) error {
	if p := shock.Piston; p != nil {
		if p.Holes != nil && *p.Holes <= 0 {
			return invalid("piston holes must be a positive integer")
		}
		if p.HoleDiameterMM != nil && (!finite(*p.HoleDiameterMM) || *p.HoleDiameterMM <= 0) {
			return invalid("piston hole_diameter_mm must be a finite number greater than zero")
		}
		p.Notes = strings.TrimSpace(p.Notes)
		if !validText(p.Notes, 10000) {
			return invalid("piston notes must contain at most 10000 characters without null bytes")
		}
		if p.Holes == nil && p.HoleDiameterMM == nil && p.Notes == "" {
			shock.Piston = nil
		}
	}
	if seal := shock.Seal; seal != nil {
		if seal.Type != nil && *seal.Type != ORing && *seal.Type != XRing {
			return invalid("seal type must be o-ring or x-ring")
		}
		if err := technical(&seal.Description); err != nil {
			return err
		}
		if seal.Type == nil && seal.Description == "" {
			shock.Seal = nil
		}
	}
	return nil
}

func requiredPatch(field string, value *string) error {
	if value == nil {
		return invalid(fmt.Sprintf("%s cannot be null", field))
	}
	return nil
}

func normalizeSuspension(data *DataV1) error {
	if data.Suspension == nil {
		return nil
	}
	removedEmptyAxle := false
	for _, axleSlot := range []**AxleSuspension{&data.Suspension.Front, &data.Suspension.Rear} {
		axle := *axleSlot
		if axle == nil {
			continue
		}
		hadOptionalBlock := axle.TopArmSpacing != nil || axle.LowerArmSpacing != nil || axle.LowerArmMount != nil
		if mount := axle.LowerArmMount; mount != nil {
			id, err := canonicalUUID(mount.ManufacturerID, "lower arm mount manufacturer_id")
			if err != nil {
				return err
			}
			mount.ManufacturerID = id
			if err := technical(&mount.Model); err != nil {
				return err
			}
			mount.ConfigurationNotes = strings.TrimSpace(mount.ConfigurationNotes)
			if !validText(mount.ConfigurationNotes, 10000) {
				return invalid("lower arm mount configuration_notes must contain at most 10000 characters without null bytes")
			}
			if mount.ManufacturerID == nil && mount.Model == "" && mount.ConfigurationNotes == "" {
				axle.LowerArmMount = nil
			}
		}
		for _, spacing := range []**ArmSpacing{&axle.TopArmSpacing, &axle.LowerArmSpacing} {
			if *spacing == nil {
				continue
			}
			for _, value := range []*float64{(*spacing).FrontMM, (*spacing).RearMM} {
				if value != nil && (!finite(*value) || *value < 0) {
					return invalid("arm spacer totals must be finite numbers greater than or equal to zero")
				}
			}
			if (*spacing).FrontMM == nil && (*spacing).RearMM == nil {
				*spacing = nil
			}
		}
		// Prune parents introduced solely by empty optional blocks; leave legacy documents alone.
		if hadOptionalBlock && axle.LowerArmMount == nil && axle.TopArmSpacing == nil && axle.LowerArmSpacing == nil &&
			axle.CamberDeg == nil && axle.CasterDeg == nil && axle.ToeDeg == nil && len(axle.LinkLengths) == 0 {
			*axleSlot = nil
			removedEmptyAxle = true
			continue
		}
		for _, degree := range []*float64{axle.CamberDeg, axle.CasterDeg, axle.ToeDeg} {
			if degree != nil && !finite(*degree) {
				return invalid("suspension angles must be finite numbers")
			}
		}
		if len(axle.LinkLengths) > 100 {
			return invalid("each axle may have at most 100 link lengths")
		}
		for i := range axle.LinkLengths {
			link := &axle.LinkLengths[i]
			link.Name = strings.TrimSpace(link.Name)
			if link.Name == "" || !validText(link.Name, 200) || (link.LengthMM != nil && (!finite(*link.LengthMM) || *link.LengthMM <= 0)) {
				return invalid("link lengths require a name and any supplied length_mm must be greater than zero")
			}
		}
	}
	if removedEmptyAxle && data.Suspension.Front == nil && data.Suspension.Rear == nil {
		data.Suspension = nil
	}

	return nil
}

func normalizeShocks(data *DataV1) error {
	if data.Shocks == nil {
		return nil
	}
	removedEmptyHardware := false
	for _, slot := range []**Shock{&data.Shocks.Front, &data.Shocks.Rear} {
		shock := *slot
		if shock == nil {
			continue
		}
		for _, text := range []*string{&shock.Manufacturer, &shock.Model} {
			if err := technical(text); err != nil {
				return err
			}
		}
		if shock.Spring != nil {
			for _, text := range []*string{&shock.Spring.Manufacturer, &shock.Spring.Color} {
				if err := technical(text); err != nil {
					return err
				}
			}
		}
		if shock.OilCST != nil && (!finite(*shock.OilCST) || *shock.OilCST <= 0) {
			return invalid("oil_cst must be greater than zero")
		}
		hadHardware := shock.Piston != nil || shock.Seal != nil
		if err := normalizeShockHardware(shock); err != nil {
			return err
		}
		if hadHardware && shock.Piston == nil && shock.Seal == nil && shock.Manufacturer == "" && shock.Model == "" && shock.Spring == nil && shock.OilCST == nil {
			*slot = nil
			removedEmptyHardware = true
		}
	}
	if removedEmptyHardware && data.Shocks.Front == nil && data.Shocks.Rear == nil {
		data.Shocks = nil
	}

	return nil
}

// canonicalUUID validates nullable catalog/query IDs without changing input storage.
func canonicalUUID(value *string, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	var uuid pgtype.UUID
	if err := uuid.Scan(*value); err != nil || !uuid.Valid {
		return nil, invalid(field + " must be a UUID")
	}
	canonical := uuid.String()
	return &canonical, nil
}
