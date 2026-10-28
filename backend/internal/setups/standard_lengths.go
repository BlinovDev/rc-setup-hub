package setups

// Standard measurements have fixed identities; custom/legacy names are not keys
// for new standard values. Never infer or destructively migrate an old document.
func validateStandardLengths(axle *AxleSuspension, front bool) error {
	if !front && axle.SteeringTieRodLengthMM != nil {
		return invalid("steering_tie_rod_length_mm is front-only")
	}
	for _, field := range []struct {
		name, legacy string
		value        *float64
	}{
		{"upper_arm_length_mm", "upper_arm", axle.UpperArmLengthMM},
		{"lower_arm_length_mm", "lower_arm", axle.LowerArmLengthMM},
		{"steering_tie_rod_length_mm", "steering_tie_rod", axle.SteeringTieRodLengthMM},
	} {
		if field.value == nil {
			continue
		}
		if !finite(*field.value) || *field.value <= 0 {
			return invalid(field.name + " must be a finite number greater than zero")
		}
		for _, link := range axle.LinkLengths {
			if link.Name == field.legacy {
				return invalid("remove the converted legacy " + field.legacy + " link before setting " + field.name)
			}
		}
	}
	return nil
}
