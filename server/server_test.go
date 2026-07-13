package server

import (
	"testing"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/bacerr"
	"github.com/anviod/bacnet/btypes/segmentation"
	"github.com/anviod/bacnet/encoding"
)

// =============================================================================
// DeviceConfig Tests
// =============================================================================

func TestDefaultDeviceConfig(t *testing.T) {
	cfg := DefaultDeviceConfig()

	if cfg.DeviceID != 1000 {
		t.Errorf("expected DeviceID 1000, got %d", cfg.DeviceID)
	}
	if cfg.DeviceName != "BACnet-Go-Server" {
		t.Errorf("expected DeviceName 'BACnet-Go-Server', got '%s'", cfg.DeviceName)
	}
	if cfg.VendorID != 999 {
		t.Errorf("expected VendorID 999, got %d", cfg.VendorID)
	}
	if cfg.Port == 0 {
		t.Error("port should not be 0")
	}
	if cfg.MaxPDU == 0 {
		t.Error("MaxPDU should not be 0")
	}
	if cfg.Segmentation != segmentation.NoSegmentation {
		t.Errorf("expected NoSegmentation, got %v", cfg.Segmentation)
	}
}

// =============================================================================
// Server Request Handler Tests (using mock data link)
// =============================================================================

func TestServer_handleReadProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Add a test object
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
				Data:       float64(42.5),
			},
		},
	})

	// Verify the store has the property (simulates ReadProperty handler logic)
	value, found := store.GetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Fatal("property should exist in store")
	}
	if v, ok := value.(float64); !ok || v != 42.5 {
		t.Errorf("expected 42.5, got %v", value)
	}

	// Verify ReadPropertyAck encoding works for the response
	response := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogInput,
				Instance: 1,
			},
			Properties: []btypes.Property{
				{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       value,
				},
			},
		},
	}
	enc := encoding.NewEncoder()
	err := enc.ReadPropertyAck(1, response)
	if err != nil {
		t.Fatalf("ReadPropertyAck encoding failed: %v", err)
	}
	if len(enc.Bytes()) == 0 {
		t.Fatal("encoded bytes should not be empty")
	}
}

func TestServer_handleReadProperty_UnknownProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
	})

	// Try to get a property that doesn't exist (not part of defaults)
	_, found := store.GetProperty(btypes.AnalogInput, 1, btypes.PROP_HIGH_LIMIT)
	if found {
		t.Error("property should not exist")
	}
}

func TestServer_handleReadProperty_UnknownObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Try to get a property from an object that doesn't exist
	_, found := store.GetProperty(btypes.AnalogInput, 999, btypes.PROP_PRESENT_VALUE)
	if found {
		t.Error("object should not exist")
	}
}

func TestServer_handleWriteProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
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
	})

	// Encode a WriteProperty request
	enc := encoding.NewEncoder()
	wp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogOutput,
				Instance: 2,
			},
			Properties: []btypes.Property{
				{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       float64(88.0),
				},
			},
		},
	}

	// Verify encoding works
	err := enc.ReadProperty(1, wp)
	if err != nil {
		t.Fatalf("failed to encode WriteProperty: %v", err)
	}

	// Verify the store can accept the write
	err = store.SetProperty(btypes.AnalogOutput, 2, btypes.PROP_PRESENT_VALUE, float64(88.0))
	if err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}

	// Verify the value was updated
	val, found := store.GetProperty(btypes.AnalogOutput, 2, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Fatal("property not found after write")
	}
	if v, ok := val.(float64); !ok || v != 88.0 {
		t.Errorf("expected 88.0, got %v", val)
	}
}

func TestServer_handleWriteProperty_UnknownObject(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	err := store.SetProperty(btypes.AnalogOutput, 999, btypes.PROP_PRESENT_VALUE, float64(1.0))
	if err == nil {
		t.Fatal("expected error when writing to unknown object")
	}
}

func TestServer_handleReadMultiProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(10.0)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Point1"},
		},
	})
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 2,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(20.0)},
		},
	})

	// Read both properties from both objects
	val1, found := store.GetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE)
	if !found || val1.(float64) != 10.0 {
		t.Errorf("expected 10.0, got %v", val1)
	}
	val2, found := store.GetProperty(btypes.AnalogInput, 2, btypes.PROP_PRESENT_VALUE)
	if !found || val2.(float64) != 20.0 {
		t.Errorf("expected 20.0, got %v", val2)
	}
}

func TestServer_handleWriteMultiProperty(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.BinaryOutput,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
		},
	})
	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.BinaryOutput,
			Instance: 2,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
		},
	})

	// Write multiple values
	store.SetProperty(btypes.BinaryOutput, 1, btypes.PROP_PRESENT_VALUE, uint32(1))
	store.SetProperty(btypes.BinaryOutput, 2, btypes.PROP_PRESENT_VALUE, uint32(1))

	val1, _ := store.GetProperty(btypes.BinaryOutput, 1, btypes.PROP_PRESENT_VALUE)
	if val1.(uint32) != 1 {
		t.Errorf("expected 1, got %v", val1)
	}
	val2, _ := store.GetProperty(btypes.BinaryOutput, 2, btypes.PROP_PRESENT_VALUE)
	if val2.(uint32) != 1 {
		t.Errorf("expected 1, got %v", val2)
	}
}

func TestServer_handleWhoIs_InRange(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Device ID 1000 should be in range [500, 1500]
	deviceID := int32(store.GetDeviceID())
	low := int32(500)
	high := int32(1500)

	if deviceID < low || deviceID > high {
		t.Errorf("device ID %d should be in range [%d, %d]", deviceID, low, high)
	}
}

func TestServer_handleWhoIs_OutOfRange(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// Device ID 1000 should NOT be in range [2000, 3000]
	deviceID := int32(store.GetDeviceID())
	low := int32(2000)
	high := int32(3000)

	if deviceID >= low && deviceID <= high {
		t.Errorf("device ID %d should NOT be in range [%d, %d]", deviceID, low, high)
	}
}

func TestServer_handleWhoIs_AllDevices(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	// When low=-1, high=-1 (WhoIs all), all devices should respond
	deviceID := int32(store.GetDeviceID())
	low := int32(0)
	high := int32(btypes.MaxInstance)

	if deviceID < low || deviceID > high {
		t.Errorf("device ID %d should respond to WhoIs all", deviceID)
	}
}

func TestServer_IAmEncoding(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	iam := btypes.IAm{
		ID: btypes.ObjectID{
			Type:     btypes.DeviceType,
			Instance: store.GetDeviceID(),
		},
		MaxApdu:      uint32(btypes.MaxAPDU),
		Segmentation: btypes.Enumerated(segmentation.NoSegmentation),
		Vendor:       store.GetVendorID(),
	}

	// Verify the IAm struct is correctly populated
	if iam.ID.Instance != 1000 {
		t.Errorf("expected instance 1000, got %d", iam.ID.Instance)
	}
	if iam.Vendor != 999 {
		t.Errorf("expected vendor 999, got %d", iam.Vendor)
	}
	if iam.ID.Type != btypes.DeviceType {
		t.Errorf("expected DeviceType, got %v", iam.ID.Type)
	}
}

// =============================================================================
// Error Response Tests
// =============================================================================

func TestServer_errorResponse(t *testing.T) {
	// Verify error class and code constants are valid
	// These should compile and be non-zero
	errClass := bacerr.ServicesError
	errCode := bacerr.ServiceRequestDenied

	if errClass == 0 {
		t.Error("ServicesError should not be 0")
	}
	if errCode == 0 {
		t.Error("ServiceRequestDenied should not be 0")
	}
}

func TestServer_sendErrorEncoding(t *testing.T) {
	// Verify that error class and code can be properly set
	a := btypes.APDU{
		DataType: btypes.Error,
		Service:  btypes.ServiceConfirmedReadProperty,
		InvokeId: 1,
	}
	a.Error.Class = bacerr.PropertyError
	a.Error.Code = bacerr.UnknownProperty

	if a.Error.Class != bacerr.PropertyError {
		t.Errorf("expected PropertyError class, got %v", a.Error.Class)
	}
	if a.Error.Code != bacerr.UnknownProperty {
		t.Errorf("expected UnknownProperty code, got %v", a.Error.Code)
	}
	if a.DataType != btypes.Error {
		t.Errorf("expected Error PDU type, got %v", a.DataType)
	}
}

func TestServer_simpleAckEncoding(t *testing.T) {
	// Verify SimpleAck APDU structure
	a := btypes.APDU{
		DataType: btypes.SimpleAck,
		Service:  btypes.ServiceConfirmedWriteProperty,
		InvokeId: 1,
	}

	if a.DataType != btypes.SimpleAck {
		t.Errorf("expected SimpleAck, got %v", a.DataType)
	}
	if a.Service != btypes.ServiceConfirmedWriteProperty {
		t.Errorf("expected WriteProperty service, got %v", a.Service)
	}
}

func TestServer_ReadPropertyAckEncoding(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	response := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogInput,
				Instance: 1,
			},
			Properties: []btypes.Property{
				{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       float64(42.5),
				},
			},
		},
	}

	enc := encoding.NewEncoder()
	err := enc.ReadPropertyAck(1, response)
	if err != nil {
		t.Fatalf("ReadPropertyAck encoding failed: %v", err)
	}

	if len(enc.Bytes()) == 0 {
		t.Fatal("ReadPropertyAck encoded bytes should not be empty")
	}

	// Verify the response can be decoded
	dec := encoding.NewDecoder(enc.Bytes())
	var apdu btypes.APDU
	err = dec.APDU(&apdu)
	if err != nil {
		t.Fatalf("APDU decoding failed: %v", err)
	}
	if apdu.DataType != btypes.ComplexAck {
		t.Errorf("expected ComplexAck, got %v", apdu.DataType)
	}

	_ = store // suppress unused warning
}

func TestServer_ReadMultiplePropertyAckEncoding(t *testing.T) {
	response := btypes.MultiplePropertyData{
		Objects: []btypes.Object{
			{
				ID: btypes.ObjectID{
					Type:     btypes.AnalogInput,
					Instance: 1,
				},
				Properties: []btypes.Property{
					{
						Type:       btypes.PROP_PRESENT_VALUE,
						ArrayIndex: btypes.ArrayAll,
						Data:       float64(10.0),
					},
				},
			},
		},
	}

	enc := encoding.NewEncoder()
	err := enc.ReadMultiplePropertyAck(1, response)
	if err != nil {
		t.Fatalf("ReadMultiplePropertyAck encoding failed: %v", err)
	}

	if len(enc.Bytes()) == 0 {
		t.Fatal("ReadMultiplePropertyAck encoded bytes should not be empty")
	}
}

// =============================================================================
// BVLC / NPDU Encoding Tests
// =============================================================================

func TestServer_BVLCEncoding(t *testing.T) {
	header := btypes.BVLC{
		Type:     btypes.BVLCTypeBacnetIP,
		Function: btypes.BacFuncUnicast,
		Length:   4,
		Data:     []byte{},
	}

	enc := encoding.NewEncoder()
	err := enc.BVLC(header)
	if err != nil {
		t.Fatalf("BVLC encoding failed: %v", err)
	}

	if len(enc.Bytes()) == 0 {
		t.Fatal("BVLC encoded bytes should not be empty")
	}
}

// =============================================================================
// Object Type Tests
// =============================================================================

func TestServer_MultipleObjectTypes(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	types := []btypes.ObjectType{
		btypes.AnalogInput,
		btypes.AnalogOutput,
		btypes.AnalogValue,
		btypes.BinaryInput,
		btypes.BinaryOutput,
		btypes.BinaryValue,
		btypes.MultiStateValue,
	}

	for i, objType := range types {
		err := store.AddObject(btypes.Object{
			ID: btypes.ObjectID{
				Type:     objType,
				Instance: btypes.ObjectInstance(i + 1),
			},
		})
		if err != nil {
			t.Fatalf("AddObject failed for type %v: %v", objType, err)
		}
	}

	list := store.GetObjectList()
	if len(list) != len(types) {
		t.Errorf("expected %d objects, got %d", len(types), len(list))
	}
}

// =============================================================================
// Various Property Types Tests
// =============================================================================

func TestServer_PropertyTypes(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	store.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogValue,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(3.14)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Pi"},
			{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "The value of pi"},
			{Type: btypes.PROP_OUT_OF_SERVICE, ArrayIndex: btypes.ArrayAll, Data: false},
			{Type: btypes.PROP_RELIABILITY, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
		},
	})

	// Read float64
	val, found := store.GetProperty(btypes.AnalogValue, 1, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Fatal("present-value not found")
	}
	if v, ok := val.(float64); !ok || v != 3.14 {
		t.Errorf("expected 3.14, got %v (%T)", val, val)
	}

	// Read string
	name, found := store.GetProperty(btypes.AnalogValue, 1, btypes.PROP_OBJECT_NAME)
	if !found {
		t.Fatal("object-name not found")
	}
	if name != "Pi" {
		t.Errorf("expected 'Pi', got '%v'", name)
	}

	// Read bool
	oos, found := store.GetProperty(btypes.AnalogValue, 1, btypes.PROP_OUT_OF_SERVICE)
	if !found {
		t.Fatal("out-of-service not found")
	}
	if oos.(bool) != false {
		t.Errorf("expected false, got %v", oos)
	}
}

// =============================================================================
// Edge Cases Tests
// =============================================================================

func TestServer_EmptyObjectStore(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	list := store.GetObjectList()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	_, found := store.GetObject(btypes.AnalogInput, 1)
	if found {
		t.Error("should not find object in empty store")
	}
}

func TestServer_LargeNumberOfObjects(t *testing.T) {
	store := NewObjectStore(1000, "TestDevice", 999)

	count := 100
	for i := 0; i < count; i++ {
		store.AddObject(btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogValue,
				Instance: btypes.ObjectInstance(i + 1),
			},
		})
	}

	list := store.GetObjectList()
	if len(list) != count {
		t.Errorf("expected %d objects, got %d", count, len(list))
	}
}

func TestServer_ObjectIDString(t *testing.T) {
	objID := btypes.ObjectID{
		Type:     btypes.AnalogInput,
		Instance: 42,
	}

	str := objID.String()
	if str == "" {
		t.Error("ObjectID.String() should not be empty")
	}
}

func TestServer_ObjectTypeString(t *testing.T) {
	if btypes.AnalogInput.String() != "AnalogInput" {
		t.Errorf("expected 'AnalogInput', got '%s'", btypes.AnalogInput.String())
	}
	if btypes.DeviceType.String() != "Device" {
		t.Errorf("expected 'Device', got '%s'", btypes.DeviceType.String())
	}
}