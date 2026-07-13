package main

import (
	"fmt"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/units"
	"github.com/anviod/bacnet/server"
)

// RoomObjectDef describes one pre-seeded room simulator object.
// RoomObjectDef 描述房间模拟器预置的一个对象。
type RoomObjectDef struct {
	Type     btypes.ObjectType
	Instance btypes.ObjectInstance
	Name     string
	Desc     string
	// PresentValue defaults: float64 for analog, uint32 for binary/multi-state.
	PresentValue interface{}
	Units        units.Unit // 0 / no-units when not applicable
	// NumberOfStates is set for multi-state objects (>0).
	NumberOfStates uint32
}

// DefaultRoomObjects returns a Room Simulator–like object set for YABE browsing.
// DefaultRoomObjects 返回贴近 Bacnet.Room.Simulator 的常见房间点位集合。
func DefaultRoomObjects() []RoomObjectDef {
	return []RoomObjectDef{
		{Type: btypes.AnalogInput, Instance: 1, Name: "Space Temperature", Desc: "Room space temperature", PresentValue: float64(22.5), Units: units.DegreesCelsius},
		{Type: btypes.AnalogInput, Instance: 2, Name: "Outdoor Temperature", Desc: "Outdoor air temperature", PresentValue: float64(18.0), Units: units.DegreesCelsius},
		{Type: btypes.AnalogInput, Instance: 3, Name: "Humidity", Desc: "Room relative humidity", PresentValue: float64(45.0), Units: units.PercentRelativeHumidity},
		{Type: btypes.AnalogInput, Instance: 4, Name: "Supply Air Temperature", Desc: "Supply air temperature", PresentValue: float64(14.0), Units: units.DegreesCelsius},

		{Type: btypes.AnalogValue, Instance: 1, Name: "Temperature Setpoint", Desc: "Space temperature setpoint", PresentValue: float64(22.0), Units: units.DegreesCelsius},
		{Type: btypes.AnalogValue, Instance: 2, Name: "Cooling Setpoint", Desc: "Cooling setpoint", PresentValue: float64(24.0), Units: units.DegreesCelsius},
		{Type: btypes.AnalogValue, Instance: 3, Name: "Heating Setpoint", Desc: "Heating setpoint", PresentValue: float64(20.0), Units: units.DegreesCelsius},

		{Type: btypes.BinaryInput, Instance: 1, Name: "Occupancy", Desc: "Occupancy sensor (0=unoccupied, 1=occupied)", PresentValue: uint32(1)},
		{Type: btypes.BinaryInput, Instance: 2, Name: "Window Status", Desc: "Window contact (0=closed, 1=open)", PresentValue: uint32(0)},

		{Type: btypes.BinaryOutput, Instance: 1, Name: "Fan", Desc: "Fan enable", PresentValue: uint32(0)},
		{Type: btypes.BinaryOutput, Instance: 2, Name: "Light", Desc: "Room light", PresentValue: uint32(0)},

		{Type: btypes.BinaryValue, Instance: 1, Name: "Occupancy Override", Desc: "Manual occupancy override", PresentValue: uint32(0)},

		// 1=Off, 2=Heat, 3=Cool, 4=Auto
		{Type: btypes.MultiStateValue, Instance: 1, Name: "HVAC Mode", Desc: "HVAC operating mode", PresentValue: uint32(4), NumberOfStates: 4},
		// 1=Off, 2=Low, 3=Med, 4=High
		{Type: btypes.MultiStateValue, Instance: 2, Name: "Fan Speed", Desc: "Fan speed", PresentValue: uint32(1), NumberOfStates: 4},
	}
}

// SeedRoomObjects adds the default room objects and tunes device metadata for browsing.
// SeedRoomObjects 向服务端写入预置房间对象，并调整 Device 元数据便于 YABE 识别。
func SeedRoomObjects(srv server.Server) error {
	store := srv.GetObjectStore()
	store.SetDeviceProperty(btypes.PROP_MODEL_NAME, "Room Simulator")
	store.SetDeviceProperty(btypes.PROP_DESCRIPTION, "BACnet-Go Room Simulator (YABE-compatible demo)")
	store.SetDeviceProperty(btypes.PropVendorName, "BACnet-Go")
	store.SetDeviceProperty(btypes.PROP_APPLICATION_SOFTWARE_VERSION, "room-simulator/1.0")

	for _, def := range DefaultRoomObjects() {
		if err := srv.AddObject(buildObject(def)); err != nil {
			return fmt.Errorf("add %s %d (%s): %w", def.Type, def.Instance, def.Name, err)
		}
	}
	return nil
}

func buildObject(def RoomObjectDef) btypes.Object {
	props := []btypes.Property{
		prop(btypes.PROP_OBJECT_IDENTIFIER, btypes.ObjectID{Type: def.Type, Instance: def.Instance}),
		prop(btypes.PROP_OBJECT_NAME, def.Name),
		prop(btypes.PROP_OBJECT_TYPE, btypes.Enumerated(def.Type)),
		prop(btypes.PROP_DESCRIPTION, def.Desc),
		prop(btypes.PROP_PRESENT_VALUE, def.PresentValue),
		prop(btypes.PROP_STATUS_FLAGS, btypes.NewBitString(4)),
		prop(btypes.PROP_EVENT_STATE, uint32(0)),
		prop(btypes.PROP_OUT_OF_SERVICE, false),
	}

	switch def.Type {
	case btypes.AnalogInput, btypes.AnalogOutput, btypes.AnalogValue:
		u := uint32(def.Units)
		if def.Units == 0 {
			u = uint32(units.NoUnits)
		}
		props = append(props, prop(btypes.PROP_Propertybtypes, u))
	case btypes.BinaryInput, btypes.BinaryOutput, btypes.BinaryValue:
		props = append(props,
			prop(btypes.PROP_INACTIVE_TEXT, "Inactive"),
			prop(btypes.PROP_ACTIVE_TEXT, "Active"),
		)
	case btypes.MultiStateInput, btypes.MultiStateOutput, btypes.MultiStateValue:
		n := def.NumberOfStates
		if n == 0 {
			n = 1
		}
		props = append(props, prop(btypes.PROP_NUMBER_OF_STATES, n))
	}

	return btypes.Object{
		ID: btypes.ObjectID{
			Type:     def.Type,
			Instance: def.Instance,
		},
		Properties: props,
	}
}

func prop(t btypes.PropertyType, data interface{}) btypes.Property {
	return btypes.Property{
		Type:       t,
		ArrayIndex: btypes.ArrayAll,
		Data:       data,
	}
}
