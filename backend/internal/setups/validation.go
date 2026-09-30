package setups

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
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
	if data.Suspension != nil {
		for _, axle := range []*AxleSuspension{data.Suspension.Front, data.Suspension.Rear} {
			if axle == nil {
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
				if link.Name == "" || !validText(link.Name, 200) || !finite(link.LengthMM) || link.LengthMM <= 0 {
					return invalid("link lengths require a name and length_mm greater than zero")
				}
			}
		}
	}
	if data.Shocks != nil {
		for _, shock := range []*Shock{data.Shocks.Front, data.Shocks.Rear} {
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
		}
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
func requiredPatch(field string, value *string) error {
	if value == nil {
		return invalid(fmt.Sprintf("%s cannot be null", field))
	}
	return nil
}
