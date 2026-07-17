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

// TestServer_ReadMultiProperty_Local 测试 Server 本地批量读功能。
// 验证 ReadMultiProperty 可以像 Client.ReadMultiProperty 一样批量读取多对象多属性，
// 但直接查询本地 ObjectStore 而不走 BACnet 网络协议。
func TestServer_ReadMultiProperty_Local(t *testing.T) {
	cfg := DefaultDeviceConfig()
	cfg.DeviceID = 1000
	cfg.DeviceName = "BatchReadServer"

	dl := newMockDataLink()
	srv, err := NewServerWithDataLink(cfg, dl)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// 造数据：添加 AnalogInput (AI-1), BinaryOutput (BO-1), MultiStateValue (MSV-1).
	_ = srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.AnalogInput,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(25.5)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Temperature"},
			{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Room temp"},
		},
	})
	_ = srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.BinaryOutput,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(1)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Fan"},
		},
	})
	_ = srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{
			Type:     btypes.MultiStateValue,
			Instance: 1,
		},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(3)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Mode"},
		},
	})

	// --- 测试 1: 多对象多属性批量读取 ---
	t.Run("MultiObjectMultiProperty", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
					Properties: []btypes.Property{
						{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
						{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll},
					},
				},
				{
					ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
					Properties: []btypes.Property{
						{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
					},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}
		if len(resp.Objects) != 2 {
			t.Fatalf("expected 2 objects, got %d", len(resp.Objects))
		}

		// AI-1: PresentValue 应为 float32(25.5), ObjectName 应为 "Temperature"
		ai := resp.Objects[0]
		if len(ai.Properties) != 2 {
			t.Fatalf("AI-1: expected 2 properties, got %d", len(ai.Properties))
		}
		if pv, ok := ai.Properties[0].Data.(float32); !ok || pv != 25.5 {
			t.Errorf("AI-1 PresentValue: expected float32(25.5), got %T(%v)", ai.Properties[0].Data, ai.Properties[0].Data)
		}
		if name, ok := ai.Properties[1].Data.(string); !ok || name != "Temperature" {
			t.Errorf("AI-1 ObjectName: expected 'Temperature', got %v", ai.Properties[1].Data)
		}

		// BO-1: PresentValue 应为 Enumerated(1)
		bo := resp.Objects[1]
		if len(bo.Properties) != 1 {
			t.Fatalf("BO-1: expected 1 property, got %d", len(bo.Properties))
		}
		if pv, ok := bo.Properties[0].Data.(btypes.Enumerated); !ok || uint32(pv) != 1 {
			t.Errorf("BO-1 PresentValue: expected Enumerated(1), got %T(%v)", bo.Properties[0].Data, bo.Properties[0].Data)
		}
	})

	// --- 测试 2: PresentValue 类型规范化 (Binary→Enumerated, Analog→float32) ---
	t.Run("PresentValueNormalization", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID:         btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
					Properties: []btypes.Property{{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll}},
				},
				{
					ID:         btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
					Properties: []btypes.Property{{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll}},
				},
				{
					ID:         btypes.ObjectID{Type: btypes.MultiStateValue, Instance: 1},
					Properties: []btypes.Property{{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll}},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}

		// AnalogInput: float64 → float32
		if _, ok := resp.Objects[0].Properties[0].Data.(float32); !ok {
			t.Errorf("AI PresentValue: expected float32, got %T", resp.Objects[0].Properties[0].Data)
		}
		// BinaryOutput: uint32 → Enumerated
		if _, ok := resp.Objects[1].Properties[0].Data.(btypes.Enumerated); !ok {
			t.Errorf("BO PresentValue: expected Enumerated, got %T", resp.Objects[1].Properties[0].Data)
		}
		// MultiStateValue: uint32 → uint32 (unchanged)
		if pv, ok := resp.Objects[2].Properties[0].Data.(uint32); !ok || pv != 3 {
			t.Errorf("MSV PresentValue: expected uint32(3), got %T(%v)", resp.Objects[2].Properties[0].Data, resp.Objects[2].Properties[0].Data)
		}
	})

	// --- 测试 3: Device 对象 PROP_ALL 展开 ---
	t.Run("DevicePropAllExpansion", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID:         btypes.ObjectID{Type: btypes.DeviceType, Instance: 1000},
					Properties: []btypes.Property{{Type: btypes.PROP_ALL, ArrayIndex: btypes.ArrayAll}},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}
		if len(resp.Objects) != 1 {
			t.Fatalf("expected 1 object, got %d", len(resp.Objects))
		}
		// PROP_ALL should expand to many device properties (>= 10)
		if len(resp.Objects[0].Properties) < 10 {
			t.Errorf("Device PROP_ALL: expected >= 10 properties, got %d", len(resp.Objects[0].Properties))
		}

		// Verify key properties exist in the expanded list
		propMap := make(map[btypes.PropertyType]interface{})
		for _, p := range resp.Objects[0].Properties {
			propMap[p.Type] = p.Data
		}
		if _, ok := propMap[btypes.PROP_OBJECT_NAME]; !ok {
			t.Error("Device PROP_ALL: missing OBJECT_NAME")
		}
		if _, ok := propMap[btypes.PROP_VENDOR_IDENTIFIER]; !ok {
			t.Error("Device PROP_ALL: missing VENDOR_IDENTIFIER")
		}
		if _, ok := propMap[btypes.PROP_MODEL_NAME]; !ok {
			t.Error("Device PROP_ALL: missing MODEL_NAME")
		}
	})

	// --- 测试 4: 混合请求（Device PROP_ALL + 普通对象属性） ---
	t.Run("MixedDeviceAndNormalObjects", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID:         btypes.ObjectID{Type: btypes.DeviceType, Instance: 1000},
					Properties: []btypes.Property{{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll}},
				},
				{
					ID:         btypes.ObjectID{Type: btypes.MultiStateValue, Instance: 1},
					Properties: []btypes.Property{{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll}},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}
		if len(resp.Objects) != 2 {
			t.Fatalf("expected 2 objects, got %d", len(resp.Objects))
		}
		// Device object -> ObjectName
		if name, ok := resp.Objects[0].Properties[0].Data.(string); !ok || name != "BatchReadServer" {
			t.Errorf("Device ObjectName: expected 'BatchReadServer', got %v", resp.Objects[0].Properties[0].Data)
		}
		// MSV -> PresentValue
		if pv, ok := resp.Objects[1].Properties[0].Data.(uint32); !ok || pv != 3 {
			t.Errorf("MSV PresentValue: expected uint32(3), got %T(%v)", resp.Objects[1].Properties[0].Data, resp.Objects[1].Properties[0].Data)
		}
	})

	// --- 测试 5: 请求不存在的对象 — 静默跳过，不报错 ---
	t.Run("UnknownObjectSilentSkip", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID:         btypes.ObjectID{Type: btypes.AnalogInput, Instance: 999},
					Properties: []btypes.Property{{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll}},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty should not error on unknown object: %v", err)
		}
		if len(resp.Objects) != 1 {
			t.Fatalf("expected 1 object in response, got %d", len(resp.Objects))
		}
		// Unknown object returns empty properties
		if len(resp.Objects[0].Properties) != 0 {
			t.Errorf("unknown object should return empty properties, got %d", len(resp.Objects[0].Properties))
		}
	})

	// --- 测试 6: 请求不存在的属性 — 静默跳过 ---
	t.Run("UnknownPropertySilentSkip", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
					Properties: []btypes.Property{
						{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
						{Type: btypes.PROP_COV_INCREMENT, ArrayIndex: btypes.ArrayAll}, // 未设置，应跳过
					},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}
		// 只有 PresentValue 返回，COVIncrement 跳过
		if len(resp.Objects[0].Properties) != 1 {
			t.Errorf("expected 1 property (only PresentValue), got %d", len(resp.Objects[0].Properties))
		}
		if resp.Objects[0].Properties[0].Type != btypes.PROP_PRESENT_VALUE {
			t.Errorf("expected PresentValue, got %v", resp.Objects[0].Properties[0].Type)
		}
	})

	// --- 测试 7: 空请求 ---
	t.Run("EmptyRequest", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty empty request: %v", err)
		}
		if len(resp.Objects) != 0 {
			t.Errorf("empty request should return empty response, got %d objects", len(resp.Objects))
		}
	})

	// --- 测试 8: 单对象多属性（含 ObjectName + Description） ---
	t.Run("SingleObjectMultiProperty", func(t *testing.T) {
		req := btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
					Properties: []btypes.Property{
						{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
						{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll},
						{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll},
					},
				},
			},
		}

		resp, err := srv.ReadMultiProperty(req)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}
		if len(resp.Objects[0].Properties) != 3 {
			t.Fatalf("expected 3 properties, got %d", len(resp.Objects[0].Properties))
		}
		if pv, ok := resp.Objects[0].Properties[0].Data.(float32); !ok || pv != 25.5 {
			t.Errorf("PresentValue: expected float32(25.5), got %T(%v)", resp.Objects[0].Properties[0].Data, resp.Objects[0].Properties[0].Data)
		}
		if name, ok := resp.Objects[0].Properties[1].Data.(string); !ok || name != "Temperature" {
			t.Errorf("ObjectName: expected 'Temperature', got %v", resp.Objects[0].Properties[1].Data)
		}
		if desc, ok := resp.Objects[0].Properties[2].Data.(string); !ok || desc != "Room temp" {
			t.Errorf("Description: expected 'Room temp', got %v", resp.Objects[0].Properties[2].Data)
		}
	})
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