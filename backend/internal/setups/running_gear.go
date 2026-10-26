package setups

func normalizeRunningGear(data *DataV1) error {
	if data.RunningGear == nil {
		return nil
	}
	for _, slot := range []**AxleRunningGear{&data.RunningGear.Front, &data.RunningGear.Rear} {
		axle := *slot
		if axle == nil {
			continue
		}
		if h := axle.Hex; h != nil {
			if h.OffsetMM != nil && !finite(*h.OffsetMM) {
				return invalid("hex offset_mm must be finite")
			}
			if h.OffsetMM == nil {
				axle.Hex = nil
			}
		}
		if w := axle.Wheel; w != nil {
			if err := normalizeRunningGearPart(&w.ManufacturerID, &w.Model); err != nil {
				return err
			}
			if w.ManufacturerID == nil && w.Model == "" && w.OffsetMM == nil {
				axle.Wheel = nil
			}
		}
		if tire := axle.Tire; tire != nil {
			if err := normalizeRunningGearPart(&tire.ManufacturerID, &tire.Model); err != nil {
				return err
			}
			if tire.ManufacturerID == nil && tire.Model == "" {
				axle.Tire = nil
			}
		}
		if axle.Hex == nil && axle.Wheel == nil && axle.Tire == nil {
			*slot = nil
		}
	}
	if data.RunningGear.Front == nil && data.RunningGear.Rear == nil {
		data.RunningGear = nil
	}
	return nil
}
func normalizeRunningGearPart(manufacturer **string, model *string) error {
	id, err := canonicalUUID(*manufacturer, "running gear manufacturer_id")
	if err != nil {
		return err
	}
	*manufacturer = id
	return technical(model)
}
