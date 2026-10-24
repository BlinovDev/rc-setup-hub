package setups

import "github.com/jackc/pgx/v5/pgtype"

// Fixed slots keep historical-reference comparisons scoped to the same axle and component.
func runningGearManufacturerIDs(data DataV1) [4]*string {
	var ids [4]*string
	if data.RunningGear == nil {
		return ids
	}
	for i, axle := range []*AxleRunningGear{data.RunningGear.Front, data.RunningGear.Rear} {
		if axle == nil {
			continue
		}
		if axle.Wheel != nil {
			ids[i*2] = axle.Wheel.ManufacturerID
		}
		if axle.Tire != nil {
			ids[i*2+1] = axle.Tire.ManufacturerID
		}
	}
	return ids
}

var runningGearComponents = [4]string{"front wheel", "front tire", "rear wheel", "rear tire"}

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
	if *manufacturer != nil {
		var id pgtype.UUID
		if err := id.Scan(**manufacturer); err != nil || !id.Valid {
			return invalid("running gear manufacturer_id must be a UUID")
		}
		canonical := id.String()
		*manufacturer = &canonical
	}
	return technical(model)
}
