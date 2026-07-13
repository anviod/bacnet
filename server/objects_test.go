package server

import (
	"testing"

	"github.com/anviod/bacnet/btypes"
)

// =============================================================================
// ObjectStore Tests
// =============================================================================

func TestNewObjectStore(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	if store.GetDeviceID() != 1000 {
		t.Errorf("expected device ID 1000, got %d", store.GetDeviceID())
	}
	if store.GetDeviceName() != "TestDevice" {
		t.Errorf("expected device name 'TestDevice', got '%s'", store.GetDeviceName())
	}
	if store.GetVendorID() != 999 {
		t.Errorf("expected vendor ID 999, got %d", store.GetVendorID())
	}
}

func TestObjectStore_AddObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
				Data:       float64(25.5),
			},
		},
	}

	err := store.AddObject(obj)
	if err != nil {
		t.Fatalf("AddObject failed: %v", err)
	}

	// Verify the object was added
	retrieved, found := store.GetObject(btypes.AnalogInput, 1)
	if !found {
		t.Fatal("object not found after AddObject")
	}
	if retrieved.ID.Instance != 1 {
		t.Errorf("expected instance 1, got %d", retrieved.ID.Instance)
	}
}

func TestObjectStore_AddDuplicateObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
	}

	err := store.AddObject(obj)
	if err != nil {
		t.Fatalf("first AddObject failed: %v", err)
	}

	err = store.AddObject(obj)
	if err == nil {
		t.Fatal("expected error on duplicate AddObject, got nil")
	}
}

func TestObjectStore_RemoveObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
	}

	store.AddObject(obj)

	err := store.RemoveObject(btypes.AnalogInput, 1)
	if err != nil {
		t.Fatalf("RemoveObject failed: %v", err)
	}

	_, found := store.GetObject(btypes.AnalogInput, 1)
	if found {
		t.Fatal("object should not exist after RemoveObject")
	}
}

func TestObjectStore_RemoveNonexistentObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	err := store.RemoveObject(btypes.AnalogInput, 999)
	if err == nil {
		t.Fatal("expected error on removing nonexistent object")
	}
}

func TestObjectStore_GetObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.BinaryInput,
			Instance: 5,
		},
		Properties: []btypes.Property{
			{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
				Data:       uint32(1),
			},
		},
	}

	store.AddObject(obj)

	// Get existing object
	retrieved, found := store.GetObject(btypes.BinaryInput, 5)
	if !found {
		t.Fatal("object not found")
	}
	if retrieved.ID.Type != btypes.BinaryInput {
		t.Errorf("expected type BinaryInput, got %v", retrieved.ID.Type)
	}

	// Get non-existing object
	_, found = store.GetObject(btypes.AnalogInput, 999)
	if found {
		t.Fatal("should not find nonexistent object")
	}
}

func TestObjectStore_GetProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogValue,
			Instance: 3,
		},
		Properties: []btypes.Property{
			{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
				Data:       float64(72.0),
			},
			{
				Type:       btypes.PROP_OBJECT_NAME,
				ArrayIndex: btypes.ArrayAll,
				Data:       "TestPoint",
			},
		},
	}

	store.AddObject(obj)

	// Get existing property
	value, found := store.GetProperty(btypes.AnalogValue, 3, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Fatal("property not found")
	}
	if v, ok := value.(float64); !ok || v != 72.0 {
		t.Errorf("expected 72.0, got %v", value)
	}

	// Get another property
	name, found := store.GetProperty(btypes.AnalogValue, 3, btypes.PROP_OBJECT_NAME)
	if !found {
		t.Fatal("property not found")
	}
	if name != "TestPoint" {
		t.Errorf("expected 'TestPoint', got '%v'", name)
	}

	// Get non-existing property
	_, found = store.GetProperty(btypes.AnalogValue, 3, btypes.PROP_DESCRIPTION)
	if found {
		t.Fatal("should not find non-existing property")
	}

	// Get property from non-existing object
	_, found = store.GetProperty(btypes.BinaryValue, 99, btypes.PROP_PRESENT_VALUE)
	if found {
		t.Fatal("should not find property on non-existing object")
	}
}

func TestObjectStore_SetProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogOutput,
			Instance: 2,
		},
		Properties: []btypes.Property{
			{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
				Data:       float64(0.0),
			},
		},
	}

	store.AddObject(obj)

	// Set existing property
	err := store.SetProperty(btypes.AnalogOutput, 2, btypes.PROP_PRESENT_VALUE, float64(100.0))
	if err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}

	// Verify the value was updated
	value, found := store.GetProperty(btypes.AnalogOutput, 2, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Fatal("property not found after SetProperty")
	}
	if v, ok := value.(float64); !ok || v != 100.0 {
		t.Errorf("expected 100.0, got %v", value)
	}

	// Set new property (not previously existing)
	err = store.SetProperty(btypes.AnalogOutput, 2, btypes.PROP_DESCRIPTION, "New Description")
	if err != nil {
		t.Fatalf("SetProperty for new property failed: %v", err)
	}
}

func TestObjectStore_SetPropertyNonexistentObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	err := store.SetProperty(btypes.AnalogInput, 999, btypes.PROP_PRESENT_VALUE, float64(1.0))
	if err == nil {
		t.Fatal("expected error when setting property on non-existent object")
	}
}

func TestObjectStore_GetDeviceProperty(t *testing.T) {
	store := NewObjectStore(1234, "MyDevice", 555)

	// Test device name
	value, found := store.GetProperty(btypes.DeviceType, 1234, btypes.PROP_OBJECT_NAME)
	if !found {
		t.Fatal("device name property not found")
	}
	if value != "MyDevice" {
		t.Errorf("expected 'MyDevice', got '%v'", value)
	}

	// Test vendor ID
	value, found = store.GetProperty(btypes.DeviceType, 1234, btypes.PROP_VENDOR_IDENTIFIER)
	if !found {
		t.Fatal("vendor ID property not found")
	}
	if v, ok := value.(uint32); !ok || v != 555 {
		t.Errorf("expected 555, got %v", value)
	}

	// Test object identifier
	value, found = store.GetProperty(btypes.DeviceType, 1234, btypes.PROP_OBJECT_IDENTIFIER)
	if !found {
		t.Fatal("object identifier property not found")
	}
	objID, ok := value.(btypes.ObjectID)
	if !ok {
		t.Fatalf("expected ObjectID, got %T", value)
	}
	if objID.Instance != 1234 {
		t.Errorf("expected instance 1234, got %d", objID.Instance)
	}
}

func TestObjectStore_GetObjectList(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
	})
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
	})
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
	})

	list := store.GetObjectList()
	if len(list) != 3 {
		t.Errorf("expected 3 objects, got %d", len(list))
	}
}

func TestObjectStore_ObjectList_ArrayIndexes(t *testing.T) {
	store := NewObjectStore(1234, "TestDevice", 999)
	_ = store.AddObject(btypes.Object{ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 2}})
	_ = store.AddObject(btypes.Object{ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1}})

	lenVal, ok := store.GetPropertyAt(btypes.DeviceType, 1234, btypes.PROP_OBJECT_LIST, 0)
	if !ok {
		t.Fatal("index 0 missing")
	}
	n, ok := lenVal.(uint32)
	if !ok || n != 3 { // Device + AI + AV
		t.Fatalf("index0 length=%v want 3", lenVal)
	}

	full, ok := store.GetPropertyAt(btypes.DeviceType, 1234, btypes.PROP_OBJECT_LIST, btypes.ArrayAll)
	if !ok {
		t.Fatal("ArrayAll missing")
	}
	ids, ok := full.([]btypes.ObjectID)
	if !ok || len(ids) != 3 {
		t.Fatalf("full=%v", full)
	}
	if ids[0].Type != btypes.DeviceType || ids[0].Instance != 1234 {
		t.Fatalf("first must be Device:1234, got %#v", ids[0])
	}
	// Sorted: Device, AI:1, AV:2
	if ids[1].Type != btypes.AnalogInput || ids[1].Instance != 1 {
		t.Fatalf("second=%#v", ids[1])
	}
	if ids[2].Type != btypes.AnalogValue || ids[2].Instance != 2 {
		t.Fatalf("third=%#v", ids[2])
	}

	el, ok := store.GetPropertyAt(btypes.DeviceType, 1234, btypes.PROP_OBJECT_LIST, 1)
	if !ok {
		t.Fatal("index 1 missing")
	}
	oid, ok := el.(btypes.ObjectID)
	if !ok || oid.Instance != 1234 {
		t.Fatalf("index1=%#v", el)
	}

	if _, ok := store.GetPropertyAt(btypes.DeviceType, 1234, btypes.PROP_OBJECT_LIST, 99); ok {
		t.Fatal("out-of-range index should miss")
	}
	if _, ok := store.GetPropertyAt(btypes.DeviceType, 9999, btypes.PROP_OBJECT_LIST, 0); ok {
		t.Fatal("wrong device instance should miss")
	}

	// Device_Address_Binding is present (nil empty list) and Object_Type is Enumerated.
	bind, ok := store.GetProperty(btypes.DeviceType, 1234, btypes.PROP_DEVICE_ADDRESS_BINDING)
	if !ok || bind != nil {
		t.Fatalf("Device_Address_Binding=%v ok=%v", bind, ok)
	}
	ot, ok := store.GetProperty(btypes.DeviceType, 1234, btypes.PROP_OBJECT_TYPE)
	if !ok {
		t.Fatal("Object_Type missing")
	}
	if _, ok := ot.(btypes.Enumerated); !ok {
		t.Fatalf("Object_Type should be Enumerated, got %T", ot)
	}
}

func TestObjectStore_GetAllObjects(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
	})
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.BinaryInput, Instance: 2},
	})

	all := store.GetAllObjects()
	if len(all) != 2 {
		t.Errorf("expected 2 types, got %d", len(all))
	}

	// Verify the returned map is a copy by modifying it
	all[btypes.AnalogInput][1].Name = "modified"
	original, _ := store.GetObject(btypes.AnalogInput, 1)
	if original.Name == "modified" {
		t.Error("GetAllObjects should return a copy, not the original")
	}
}

func TestObjectStore_DefaultProperties(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Add an object with no properties - should get defaults
	obj := btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
	}

	err := store.AddObject(obj)
	if err != nil {
		t.Fatalf("AddObject failed: %v", err)
	}

	// Check that default properties were created
	_, found := store.GetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Error("default present-value property should exist")
	}

	_, found = store.GetProperty(btypes.AnalogInput, 1, btypes.PROP_OBJECT_NAME)
	if !found {
		t.Error("default object-name property should exist")
	}
}

func TestObjectStore_Concurrency(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Add objects concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			obj := btypes.Object{
				ID: btypes.ObjectID{
					Type:     btypes.AnalogInput,
					Instance: btypes.ObjectInstance(id),
				},
			}
			store.AddObject(obj)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all objects were added
	list := store.GetObjectList()
	if len(list) != 10 {
		t.Errorf("expected 10 objects, got %d", len(list))
	}

	// Concurrent reads and writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			store.SetProperty(btypes.AnalogInput, btypes.ObjectInstance(id), btypes.PROP_PRESENT_VALUE, float64(id*10))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify values
	for i := 0; i < 10; i++ {
		val, found := store.GetProperty(btypes.AnalogInput, btypes.ObjectInstance(i), btypes.PROP_PRESENT_VALUE)
		if !found {
			t.Errorf("property for instance %d not found", i)
		}
		if v, ok := val.(float64); !ok || v != float64(i*10) {
			t.Errorf("instance %d: expected %f, got %v", i, float64(i*10), val)
		}
	}
}

func TestObjectStore_DatabaseRevision(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Initial revision
	rev, found := store.GetProperty(btypes.DeviceType, 1000, btypes.PROP_DATABASE_REVISION)
	if !found {
		t.Fatal("database revision not found")
	}
	initialRev := rev.(uint32)

	// Add an object - should increment revision
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
	})

	rev, _ = store.GetProperty(btypes.DeviceType, 1000, btypes.PROP_DATABASE_REVISION)
	if rev.(uint32) != initialRev+1 {
		t.Errorf("expected revision %d, got %d", initialRev+1, rev.(uint32))
	}

	// Set a property - should increment revision
	store.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float64(50.0))

	rev, _ = store.GetProperty(btypes.DeviceType, 1000, btypes.PROP_DATABASE_REVISION)
	if rev.(uint32) != initialRev+2 {
		t.Errorf("expected revision %d, got %d", initialRev+2, rev.(uint32))
	}
}

func TestObjectStore_BuildServicesSupported(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	bs := store.buildServicesSupported()
	if bs == nil {
		t.Fatal("services supported bitstring is nil")
	}

	// Verify ReadProperty is supported (bit 12)
	if !bs.Bit(12) {
		t.Error("ReadProperty should be supported")
	}
	// Verify WriteProperty is supported (bit 15)
	if !bs.Bit(15) {
		t.Error("WriteProperty should be supported")
	}
}

func TestObjectStore_BuildObjectTypesSupported(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	bs := store.buildObjectTypesSupported()
	if bs == nil {
		t.Fatal("object types supported bitstring is nil")
	}

	// Verify AnalogInput is supported (bit 0)
	if !bs.Bit(0) {
		t.Error("AnalogInput should be supported")
	}
}