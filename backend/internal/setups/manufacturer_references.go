package setups

// Fixed slots preserve per-component historical references and validation order.
// A removed component retains its slot with a nil ID.
type manufacturerReference struct {
	component string
	id        *string
}

func manufacturerReferences(data DataV1) [8]manufacturerReference {
	refs := [8]manufacturerReference{
		{component: "rear differential"},
		{component: "front lower arm mount"},
		{component: "rear lower arm mount"},
		{component: "front steering knuckle"},
		{component: "front wheel"},
		{component: "front tire"},
		{component: "rear wheel"},
		{component: "rear tire"},
	}
	if data.RearDifferential != nil {
		refs[0].id = data.RearDifferential.ManufacturerID
	}
	if data.Suspension != nil {
		for i, axle := range []*AxleSuspension{data.Suspension.Front, data.Suspension.Rear} {
			if axle != nil && axle.LowerArmMount != nil {
				refs[i+1].id = axle.LowerArmMount.ManufacturerID
			}
		}
	}
	if data.FrontSteeringKnuckle != nil {
		refs[3].id = data.FrontSteeringKnuckle.ManufacturerID
	}
	if data.RunningGear != nil {
		for i, axle := range []*AxleRunningGear{data.RunningGear.Front, data.RunningGear.Rear} {
			if axle == nil {
				continue
			}
			if axle.Wheel != nil {
				refs[4+i*2].id = axle.Wheel.ManufacturerID
			}
			if axle.Tire != nil {
				refs[5+i*2].id = axle.Tire.ManufacturerID
			}
		}
	}
	return refs
}
