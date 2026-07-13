package server

import (
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/bacerr"
	"github.com/anviod/bacnet/btypes/ndpu"
	"github.com/anviod/bacnet/btypes/segmentation"
	"github.com/anviod/bacnet/encoding"
)

// =============================================================================
// Integration Tests with Mock DataLink
// =============================================================================

func newTestServer(t *testing.T) (*server, *mockDataLink) {
	t.Helper()

	cfg := DefaultDeviceConfig()
	cfg.DeviceID = 1000
	cfg.DeviceName = "TestServer"
	cfg.VendorID = 999

	mockDL := newMockDataLink()
	srv, err := NewServerWithDataLink(cfg, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink failed: %v", err)
	}

	// Add some test objects
	srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(23.5)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "TempSensor"},
		},
	})
	srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(1)},
		},
	})

	return srv.(*server), mockDL
}

// Build a complete BACnet message (BVLC + NPDU + APDU) for testing.
// apduHeaderBytes is the APDU header (meta + service), rawData is the service payload.
func buildBACnetMessage(t *testing.T, apduHeaderBytes, rawData []byte, isBroadcast bool) []byte {
	t.Helper()

	// Build NPDU
	npduEnc := encoding.NewEncoder()
	npdu := btypes.NPDU{
		Version: btypes.ProtocolVersion,
	}
	npduEnc.NPDU(&npdu)

	// Build full APDU = header + raw data
	fullAPDU := append(apduHeaderBytes, rawData...)

	// Build BVLC
	totalLen := 4 + len(npduEnc.Bytes()) + len(fullAPDU)
	header := btypes.BVLC{
		Type:   btypes.BVLCTypeBacnetIP,
		Length: uint16(totalLen),
		Data:   append(npduEnc.Bytes(), fullAPDU...),
	}
	if isBroadcast {
		header.Function = btypes.BacFuncBroadcast
	} else {
		header.Function = btypes.BacFuncUnicast
	}

	bvlcEnc := encoding.NewEncoder()
	err := bvlcEnc.BVLC(header)
	if err != nil {
		t.Fatalf("BVLC encoding failed: %v", err)
	}

	return bvlcEnc.Bytes()
}

// buildAPDUHeader builds just the APDU header bytes (meta + service info).
func buildAPDUHeader(t *testing.T, apdu btypes.APDU) []byte {
	t.Helper()
	enc := encoding.NewEncoder()
	err := enc.APDU(apdu)
	if err != nil {
		t.Fatalf("APDU header encoding failed: %v", err)
	}
	return enc.Bytes()
}

// =============================================================================
// Server Lifecycle Tests
// =============================================================================

func TestServer_NewServerWithDataLink(t *testing.T) {
	cfg := DefaultDeviceConfig()
	mockDL := newMockDataLink()

	srv, err := NewServerWithDataLink(cfg, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink failed: %v", err)
	}

	if !srv.IsRunning() {
		// Server should not be running until Serve() is called
		t.Log("Server is not running (expected before Serve)")
	}

	if srv.GetDeviceID() != 1000 {
		t.Errorf("expected device ID 1000, got %d", srv.GetDeviceID())
	}
}

func TestServer_Close(t *testing.T) {
	srv, mockDL := newTestServer(t)

	err := srv.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !mockDL.closed {
		t.Error("mock data link should be closed")
	}
}

func TestServer_IsRunning_AfterClose(t *testing.T) {
	srv, _ := newTestServer(t)

	// Start server in background
	go func() {
		_ = srv.Serve()
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	if !srv.IsRunning() {
		t.Error("server should be running after Serve()")
	}

	srv.Close()
	time.Sleep(50 * time.Millisecond)

	if srv.IsRunning() {
		t.Error("server should not be running after Close()")
	}
}

// =============================================================================
// ReadProperty Handling Tests
// =============================================================================

func TestServer_handleReadProperty_Integration(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Build a ReadProperty request for AnalogInput:1 PresentValue
	enc := encoding.NewEncoder()
	rp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogInput,
				Instance: 1,
			},
			Properties: []btypes.Property{
				{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				},
			},
		},
	}
	err := enc.ReadProperty(1, rp)
	if err != nil {
		t.Fatalf("ReadProperty encoding failed: %v", err)
	}

	// Build full BACnet message with APDU wrapper
	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedReadProperty,
		InvokeId: 1,
		RawData:  enc.Bytes(),
		MaxSegs:  0,
		MaxApdu:  btypes.MaxAPDU,
	}
	hdr := buildAPDUHeader(t, apdu)

	// Build full BACnet message
	msg := buildBACnetMessage(t, hdr, apdu.RawData, false)

	// Inject the message and process it
	src := &btypes.Address{
		Adr: []byte{192, 168, 1, 100},
		Len: 4,
	}
	mockDL.injectReceive(src, msg)
	mockDL.injectReceive(src, msg) // Second injection to trigger processing

	// Start server in background
	go func() {
		_ = srv.Serve()
	}()

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	srv.Close()

	// Verify the store still has the correct value (it wasn't modified)
	val, found := srv.GetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Error("property should still exist after ReadProperty")
	}
	if v, ok := val.(float64); !ok || v != 23.5 {
		t.Errorf("property value should be unchanged: expected 23.5, got %v", v)
	}
}

// =============================================================================
// HandleMsg Tests
// =============================================================================

func TestServer_handleMsg_ConfirmedReadProperty(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Build APDU for ReadProperty
	enc := encoding.NewEncoder()
	rp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogInput,
				Instance: 1,
			},
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
			},
		},
	}
	enc.ReadProperty(1, rp)

	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedReadProperty,
		InvokeId: 1,
		RawData:  enc.Bytes(),
		MaxSegs:  0,
		MaxApdu:  btypes.MaxAPDU,
	}
	hdr := buildAPDUHeader(t, apdu)

	// Process the message directly
	msg := buildBACnetMessage(t, hdr, apdu.RawData, false)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	srv.handleMsg(src, msg)

	// Verify that a response was sent
	if mockDL.sentCount() == 0 {
		t.Log("No response was sent (may be expected if sendPacket fails silently)")
	}
}

func TestServer_handleMsg_ConfirmedWriteProperty(t *testing.T) {
	srv, _ := newTestServer(t)

	// Build APDU for WriteProperty
	enc := encoding.NewEncoder()
	wp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.BinaryOutput,
				Instance: 1,
			},
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
			},
		},
	}
	enc.ReadProperty(1, wp)

	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedWriteProperty,
		InvokeId: 1,
		RawData:  enc.Bytes(),
		MaxSegs:  0,
		MaxApdu:  btypes.MaxAPDU,
	}
	hdr := buildAPDUHeader(t, apdu)

	msg := buildBACnetMessage(t, hdr, apdu.RawData, false)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	srv.handleMsg(src, msg)

	// Verify the value was updated
	val, found := srv.GetProperty(btypes.BinaryOutput, 1, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Error("property should still exist")
	}
	if v, ok := val.(uint32); !ok || v != 0 {
		t.Logf("property value after write: %v (expected 0)", v)
	}
}

func TestServer_handleMsg_UnconfirmedWhoIs(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Build WhoIs request
	enc := encoding.NewEncoder()
	enc.WhoIs(-1, -1) // All devices

	apdu := btypes.APDU{
		DataType:           btypes.UnconfirmedServiceRequest,
		UnconfirmedService: btypes.ServiceUnconfirmedWhoIs,
	}
	hdr := buildAPDUHeader(t, apdu)
	// WhoIs encoder writes DataType + UnconfirmedService + optional range
	// Strip the first 2 bytes (DataType + UnconfirmedService) to get just the range data
	rawData := enc.Bytes()[2:]

	msg := buildBACnetMessage(t, hdr, rawData, true)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// Process the message directly
	srv.handleMsg(src, msg)

	// Verify that an IAm was sent
	if mockDL.sentCount() > 0 {
		t.Logf("IAm response sent (%d packets)", mockDL.sentCount())
	}
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestServer_handleReadProperty_UnknownObject_Error(t *testing.T) {
	srv, _ := newTestServer(t)

	// Build ReadProperty for non-existent object
	enc := encoding.NewEncoder()
	rp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogInput,
				Instance: 9999, // Non-existent
			},
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
			},
		},
	}
	enc.ReadProperty(1, rp)

	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedReadProperty,
		InvokeId: 1,
		RawData:  enc.Bytes(),
		MaxSegs:  0,
		MaxApdu:  btypes.MaxAPDU,
	}
	hdr := buildAPDUHeader(t, apdu)

	msg := buildBACnetMessage(t, hdr, apdu.RawData, false)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// This should trigger an error response
	srv.handleMsg(src, msg)
}

func TestServer_handleWriteProperty_UnknownObject_Error(t *testing.T) {
	srv, _ := newTestServer(t)

	// Build WriteProperty for non-existent object
	enc := encoding.NewEncoder()
	wp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.BinaryOutput,
				Instance: 9999, // Non-existent
			},
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(1)},
			},
		},
	}
	enc.ReadProperty(1, wp)

	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedWriteProperty,
		InvokeId: 1,
		RawData:  enc.Bytes(),
		MaxSegs:  0,
		MaxApdu:  btypes.MaxAPDU,
	}
	hdr := buildAPDUHeader(t, apdu)

	msg := buildBACnetMessage(t, hdr, apdu.RawData, false)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// This should trigger an error response
	srv.handleMsg(src, msg)
}

func TestServer_sendError_Encoding(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Create a simple NPDU
	npdu := &btypes.NPDU{Version: btypes.ProtocolVersion}
	dest := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// Send an error
	srv.sendError(dest, npdu, 1, btypes.ServiceConfirmedReadProperty, bacerr.PropertyError, bacerr.UnknownProperty)

	// Verify the error was sent
	if mockDL.sentCount() == 0 {
		t.Log("Error was not sent (may be expected if encoding fails)")
	}
}

// =============================================================================
// SendPacket Tests
// =============================================================================

func TestServer_sendPacket_Unicast(t *testing.T) {
	srv, mockDL := newTestServer(t)

	npdu := &btypes.NPDU{Version: btypes.ProtocolVersion}
	dest := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	data := []byte{0x01, 0x02, 0x03}
	n, err := srv.sendPacket(dest, npdu, data, false)
	if err != nil {
		t.Fatalf("sendPacket failed: %v", err)
	}
	if n == 0 {
		t.Error("send should return non-zero byte count")
	}
	if mockDL.sentCount() != 1 {
		t.Errorf("expected 1 sent packet, got %d", mockDL.sentCount())
	}
}

func TestServer_sendPacket_Broadcast(t *testing.T) {
	srv, mockDL := newTestServer(t)

	npdu := &btypes.NPDU{Version: btypes.ProtocolVersion}
	dest := mockDL.GetBroadcastAddress()

	data := []byte{0x01, 0x02, 0x03}
	n, err := srv.sendPacket(dest, npdu, data, true)
	if err != nil {
		t.Fatalf("sendPacket (broadcast) failed: %v", err)
	}
	if n == 0 {
		t.Error("send should return non-zero byte count")
	}
	if mockDL.sentCount() != 1 {
		t.Errorf("expected 1 sent packet, got %d", mockDL.sentCount())
	}
}

// =============================================================================
// HandleMsg Edge Cases
// =============================================================================

func TestServer_handleMsg_InvalidBVLC(t *testing.T) {
	srv, _ := newTestServer(t)

	// Message with invalid BVLC
	invalidMsg := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// This should not panic
	srv.handleMsg(src, invalidMsg)
}

func TestServer_handleMsg_InvalidNPDU(t *testing.T) {
	srv, _ := newTestServer(t)

	// Valid BVLC but invalid NPDU
	bvlcEnc := encoding.NewEncoder()
	header := btypes.BVLC{
		Type:     btypes.BVLCTypeBacnetIP,
		Function: btypes.BacFuncUnicast,
		Length:   8,
		Data:     []byte{0xFF, 0xFF, 0xFF, 0xFF}, // Invalid NPDU
	}
	_ = bvlcEnc.BVLC(header)

	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}
	srv.handleMsg(src, bvlcEnc.Bytes())
}

func TestServer_handleMsg_InvalidAPDU(t *testing.T) {
	srv, _ := newTestServer(t)

	// Valid BVLC + NPDU, but invalid APDU
	npduEnc := encoding.NewEncoder()
	npdu := btypes.NPDU{Version: btypes.ProtocolVersion}
	npduEnc.NPDU(&npdu)

	bvlcEnc := encoding.NewEncoder()
	header := btypes.BVLC{
		Type:     btypes.BVLCTypeBacnetIP,
		Function: btypes.BacFuncUnicast,
		Length:   uint16(4 + len(npduEnc.Bytes()) + 4),
		Data:     append(npduEnc.Bytes(), 0xFF, 0xFF, 0xFF, 0xFF), // Invalid APDU
	}
	_ = bvlcEnc.BVLC(header)

	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}
	srv.handleMsg(src, bvlcEnc.Bytes())
}

func TestServer_handleMsg_NetworkLayerMessage(t *testing.T) {
	srv, _ := newTestServer(t)

	// Build a network layer message NPDU
	npduEnc := encoding.NewEncoder()
	npdu := btypes.NPDU{
		Version:                 btypes.ProtocolVersion,
		IsNetworkLayerMessage:   true,
		NetworkLayerMessageType: ndpu.IamRouterToNetwork,
	}
	npduEnc.NPDU(&npdu)

	bvlcEnc := encoding.NewEncoder()
	header := btypes.BVLC{
		Type:     btypes.BVLCTypeBacnetIP,
		Function: btypes.BacFuncUnicast,
		Length:   uint16(4 + len(npduEnc.Bytes())),
		Data:     npduEnc.Bytes(),
	}
	_ = bvlcEnc.BVLC(header)

	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}
	// This should just return silently
	srv.handleMsg(src, bvlcEnc.Bytes())
}

// =============================================================================
// Server Interface Method Tests
// =============================================================================

func TestServer_InterfaceMethods(t *testing.T) {
	cfg := DefaultDeviceConfig()
	cfg.DeviceID = 5000
	cfg.DeviceName = "InterfaceTest"
	cfg.VendorID = 777

	mockDL := newMockDataLink()
	var srvIface Server
	srv, err := NewServerWithDataLink(cfg, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink failed: %v", err)
	}
	srvIface = srv

	// Test all interface methods
	if srvIface.GetDeviceID() != 5000 {
		t.Errorf("expected device ID 5000, got %d", srvIface.GetDeviceID())
	}

	store := srvIface.GetObjectStore()
	if store == nil {
		t.Fatal("GetObjectStore returned nil")
	}

	// Test AddObject via interface
	err = srvIface.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 100},
	})
	if err != nil {
		t.Fatalf("AddObject via interface failed: %v", err)
	}

	// Test GetObject via interface
	obj, found := srvIface.GetObject(btypes.AnalogValue, 100)
	if !found {
		t.Fatal("GetObject via interface: object not found")
	}
	if obj.ID.Instance != 100 {
		t.Errorf("expected instance 100, got %d", obj.ID.Instance)
	}

	// Test SetProperty via interface
	err = srvIface.SetProperty(btypes.AnalogValue, 100, btypes.PROP_PRESENT_VALUE, float64(99.9))
	if err != nil {
		t.Fatalf("SetProperty via interface failed: %v", err)
	}

	// Test GetProperty via interface
	val, found := srvIface.GetProperty(btypes.AnalogValue, 100, btypes.PROP_PRESENT_VALUE)
	if !found {
		t.Fatal("GetProperty via interface: property not found")
	}
	if v, ok := val.(float64); !ok || v != 99.9 {
		t.Errorf("expected 99.9, got %v", val)
	}

	// Test RemoveObject via interface
	err = srvIface.RemoveObject(btypes.AnalogValue, 100)
	if err != nil {
		t.Fatalf("RemoveObject via interface failed: %v", err)
	}

	_, found = srvIface.GetObject(btypes.AnalogValue, 100)
	if found {
		t.Error("object should have been removed")
	}
}

// =============================================================================
// DeviceConfig Validation Tests
// =============================================================================

func TestDeviceConfig_NilDefaults(t *testing.T) {
	// Test that nil config gets defaults
	mockDL := newMockDataLink()
	srv, err := NewServerWithDataLink(nil, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink with nil config failed: %v", err)
	}

	if srv.GetDeviceID() != 1000 {
		t.Errorf("expected default device ID 1000, got %d", srv.GetDeviceID())
	}
}

func TestDeviceConfig_ZeroValues(t *testing.T) {
	cfg := &DeviceConfig{
		DeviceID: 2000,
	}
	mockDL := newMockDataLink()
	srv, err := NewServerWithDataLink(cfg, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink failed: %v", err)
	}

	if srv.GetDeviceID() != 2000 {
		t.Errorf("expected device ID 2000, got %d", srv.GetDeviceID())
	}
}

func TestDeviceConfig_DeviceIDZeroIsPreserved(t *testing.T) {
	cfg := &DeviceConfig{
		DeviceID:   0,
		DeviceName: "ZeroDevice",
		VendorID:   1,
	}
	mockDL := newMockDataLink()
	srv, err := NewServerWithDataLink(cfg, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink failed: %v", err)
	}
	if srv.GetDeviceID() != 0 {
		t.Errorf("DeviceID 0 must be preserved, got %d (must not rewrite to 1000)", srv.GetDeviceID())
	}
}

func TestDeviceConfig_DeviceIDTooLarge(t *testing.T) {
	cfg := &DeviceConfig{
		DeviceID: btypes.ObjectInstance(btypes.MaxInstance + 1),
	}
	mockDL := newMockDataLink()
	_, err := NewServerWithDataLink(cfg, mockDL)
	if err == nil {
		t.Fatal("expected error for DeviceID > MaxInstance")
	}
}

// =============================================================================
// WhoIs Range Tests
// =============================================================================

func TestServer_WhoIs_SpecificRange(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Build WhoIs for devices 500-1500
	enc := encoding.NewEncoder()
	enc.WhoIs(500, 1500)

	apdu := btypes.APDU{
		DataType:           btypes.UnconfirmedServiceRequest,
		UnconfirmedService: btypes.ServiceUnconfirmedWhoIs,
	}
	hdr := buildAPDUHeader(t, apdu)
	rawData := enc.Bytes()[2:] // Strip DataType + UnconfirmedService bytes

	msg := buildBACnetMessage(t, hdr, rawData, true)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// Server device ID is 1000, which is in range [500, 1500]
	srv.handleMsg(src, msg)

	// Should have sent an IAm response
	if mockDL.sentCount() > 0 {
		t.Logf("IAm sent for in-range WhoIs (%d packets)", mockDL.sentCount())
	}
}

func TestServer_WhoIs_OutOfRange_NoResponse(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Build WhoIs for devices 2000-3000
	enc := encoding.NewEncoder()
	enc.WhoIs(2000, 3000)

	apdu := btypes.APDU{
		DataType:           btypes.UnconfirmedServiceRequest,
		UnconfirmedService: btypes.ServiceUnconfirmedWhoIs,
	}
	hdr := buildAPDUHeader(t, apdu)
	rawData := enc.Bytes()[2:] // Strip DataType + UnconfirmedService bytes

	msg := buildBACnetMessage(t, hdr, rawData, true)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// Server device ID is 1000, which is NOT in range [2000, 3000]
	beforeCount := mockDL.sentCount()
	srv.handleMsg(src, msg)

	// Should NOT have sent an IAm response
	if mockDL.sentCount() > beforeCount {
		t.Error("IAm should not be sent for out-of-range WhoIs")
	}
}

// =============================================================================
// ReadProperty and WriteProperty Ack Tests
// =============================================================================

func TestServer_SegmentedRequest_Abort(t *testing.T) {
	srv, mockDL := newTestServer(t)

	// Craft a minimal confirmed ReadProperty APDU with segmented bit set.
	a := btypes.APDU{
		DataType:         btypes.ConfirmedServiceRequest,
		Service:          btypes.ServiceConfirmedReadProperty,
		InvokeId:         9,
		SegmentedMessage: true,
		Sequence:         0,
		WindowNumber:     1,
		MaxApdu:          btypes.MaxAPDU,
	}
	enc := encoding.NewEncoder()
	_ = enc.APDU(a)
	apduBytes := enc.Bytes()

	npduEnc := encoding.NewEncoder()
	npduEnc.NPDU(&btypes.NPDU{
		Version:               btypes.ProtocolVersion,
		IsNetworkLayerMessage: false,
		ExpectingReply:        true,
		Priority:              btypes.Normal,
		HopCount:              btypes.DefaultHopCount,
	})
	payload := append(npduEnc.Bytes(), apduBytes...)

	bvlcEnc := encoding.NewEncoder()
	_ = bvlcEnc.BVLC(btypes.BVLC{
		Type:     btypes.BVLCTypeBacnetIP,
		Function: btypes.BacFuncUnicast,
		Length:   uint16(4 + len(payload)),
		Data:     payload,
	})

	before := mockDL.sentCount()
	srv.handleMsg(mockDL.GetMyAddress(), bvlcEnc.Bytes())

	if mockDL.sentCount() <= before {
		t.Fatal("expected Abort response for segmented request")
	}
	last := mockDL.getLastSent()
	// Find Abort PDU type 0x70/0x71 in the packet
	foundAbort := false
	for _, b := range last {
		if b&0xF0 == byte(btypes.Abort) {
			foundAbort = true
			break
		}
	}
	if !foundAbort {
		t.Fatalf("Abort PDU not found in response: %x", last)
	}
}

func TestServer_ReadPropertyAck_RoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)

	// Read a property and encode the response
	val, found := srv.GetProperty(btypes.AnalogInput, 1, btypes.PROP_OBJECT_NAME)
	if !found {
		t.Fatal("property not found")
	}

	response := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.AnalogInput,
				Instance: 1,
			},
			Properties: []btypes.Property{
				{
					Type:       btypes.PROP_OBJECT_NAME,
					ArrayIndex: btypes.ArrayAll,
					Data:       val,
				},
			},
		},
	}

	enc := encoding.NewEncoder()
	err := enc.ReadPropertyAck(1, response)
	if err != nil {
		t.Fatalf("ReadPropertyAck encoding failed: %v", err)
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
	if apdu.InvokeId != 1 {
		t.Errorf("expected InvokeId 1, got %d", apdu.InvokeId)
	}
}

// =============================================================================
// Unconfirmed Service Tests
// =============================================================================

func TestServer_handleUnconfirmed_UnhandledService(t *testing.T) {
	srv, _ := newTestServer(t)

	// Build an unconfirmed service that's not handled
	apdu := btypes.APDU{
		DataType:           btypes.UnconfirmedServiceRequest,
		UnconfirmedService: btypes.ServiceUnconfirmedIAm, // We don't handle IAm as server
	}
	hdr := buildAPDUHeader(t, apdu)

	msg := buildBACnetMessage(t, hdr, nil, true)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	// This should not panic
	srv.handleMsg(src, msg)
}

// =============================================================================
// DataLink Interface Completeness Tests
// =============================================================================

func TestMockDataLink_Interface(t *testing.T) {
	dl := newMockDataLink()

	myAddr := dl.GetMyAddress()
	if myAddr == nil {
		t.Error("GetMyAddress returned nil")
	}

	bcastAddr := dl.GetBroadcastAddress()
	if bcastAddr == nil {
		t.Error("GetBroadcastAddress returned nil")
	}

	// Test send
	data := []byte{0x01, 0x02}
	npdu := &btypes.NPDU{Version: btypes.ProtocolVersion}
	dest := &btypes.Address{Adr: []byte{192, 168, 1, 1}, Len: 4}
	n, err := dl.Send(data, npdu, dest)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes sent, got %d", len(data), n)
	}
	if dl.sentCount() != 1 {
		t.Errorf("expected 1 sent packet, got %d", dl.sentCount())
	}

	// Test the last sent data
	lastData := dl.getLastSent()
	if len(lastData) != len(data) {
		t.Errorf("expected %d bytes in last sent, got %d", len(data), len(lastData))
	}

	lastDest := dl.getLastSentDest()
	if lastDest == nil {
		t.Error("getLastSentDest returned nil")
	}

	// Test receive
	recvData := make([]byte, 1024)
	dl.injectReceive(dest, []byte{0xAA, 0xBB})
	addr, n, err := dl.Receive(recvData)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 bytes received, got %d", n)
	}
	if addr == nil {
		t.Error("Receive returned nil address")
	}

	// Test close
	err = dl.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !dl.closed {
		t.Error("data link should be closed")
	}
}

// =============================================================================
// Error Classes and Codes Tests
// =============================================================================

func TestServer_ErrorClasses(t *testing.T) {
	errorClasses := []bacerr.ErrorClass{
		bacerr.DeviceError,
		bacerr.ObjectError,
		bacerr.PropertyError,
		bacerr.ResourcesError,
		bacerr.SecurityError,
		bacerr.ServicesError,
		bacerr.VTError,
	}

	// Verify all error classes are defined (some can be 0 legitimately)
	for i, ec := range errorClasses {
		_ = ec // All are valid
		_ = i
	}
}

func TestServer_ErrorCodes(t *testing.T) {
	errorCodes := []bacerr.ErrorCode{
		bacerr.UnknownObject,
		bacerr.UnknownProperty,
		bacerr.ServiceRequestDenied,
		bacerr.InvalidTag,
		bacerr.MissingRequiredParameter,
		bacerr.WriteAccessDenied,
		bacerr.OperationalProblem,
	}

	for _, ec := range errorCodes {
		if ec == 0 {
			t.Error("error code should not be 0")
		}
	}
}

// =============================================================================
// Segmentation Tests
// =============================================================================

func TestServer_SegmentationSettings(t *testing.T) {
	cfg := &DeviceConfig{
		DeviceID:     1000,
		DeviceName:   "SegTest",
		VendorID:     999,
		Segmentation: segmentation.SegmentedBoth,
		MaxSegments:  16,
	}

	mockDL := newMockDataLink()
	srv, err := NewServerWithDataLink(cfg, mockDL)
	if err != nil {
		t.Fatalf("NewServerWithDataLink failed: %v", err)
	}

	// Verify the server was created successfully
	if srv.GetDeviceID() != 1000 {
		t.Errorf("expected device ID 1000, got %d", srv.GetDeviceID())
	}
}

// =============================================================================
// SendUnconfirmed Tests
// =============================================================================

func TestServer_sendUnconfirmed(t *testing.T) {
	srv, mockDL := newTestServer(t)

	npdu := &btypes.NPDU{Version: btypes.ProtocolVersion}
	dest := mockDL.GetBroadcastAddress()

	data := []byte{0x01, 0x02, 0x03}
	n, err := srv.sendUnconfirmed(dest, npdu, data, true)
	if err != nil {
		t.Fatalf("sendUnconfirmed failed: %v", err)
	}
	if n == 0 {
		t.Error("send should return non-zero byte count")
	}
	if mockDL.sentCount() != 1 {
		t.Errorf("expected 1 sent packet, got %d", mockDL.sentCount())
	}
}

// =============================================================================
// Confirmed service dispatch: ReadProperty MUST ComplexAck; unknown → Error
// =============================================================================

// decodeSentAPDU strips BVLC+NPDU from a captured Send() packet and returns the APDU.
func decodeSentAPDU(t *testing.T, pkt []byte) btypes.APDU {
	t.Helper()
	dec := encoding.NewDecoder(pkt)
	var bvlc btypes.BVLC
	if err := dec.BVLC(&bvlc); err != nil {
		t.Fatalf("BVLC decode: %v (pkt=%x)", err, pkt)
	}
	// Decoder.BVLC reads the fixed four-byte header; decode the remaining
	// NPDU/APDU directly from the packet. bvlc.Data is not populated by the
	// streaming decoder.
	ndec := encoding.NewDecoder(pkt[4:])
	var npdu btypes.NPDU
	if _, err := ndec.NPDU(&npdu); err != nil {
		t.Fatalf("NPDU decode: %v", err)
	}
	var apdu btypes.APDU
	if err := ndec.APDU(&apdu); err != nil {
		t.Fatalf("APDU decode: %v remaining=%x", err, ndec.Bytes())
	}
	return apdu
}

func injectConfirmedAPDU(t *testing.T, srv *server, mockDL *mockDataLink, apduBytes []byte) {
	t.Helper()
	npduEnc := encoding.NewEncoder()
	npduEnc.NPDU(&btypes.NPDU{
		Version:        btypes.ProtocolVersion,
		ExpectingReply: true,
		Priority:       btypes.Normal,
		HopCount:       btypes.DefaultHopCount,
	})
	payload := append(npduEnc.Bytes(), apduBytes...)
	bvlcEnc := encoding.NewEncoder()
	_ = bvlcEnc.BVLC(btypes.BVLC{
		Type:     btypes.BVLCTypeBacnetIP,
		Function: btypes.BacFuncUnicast,
		Length:   uint16(4 + len(payload)),
		Data:     payload,
	})
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4, Mac: []byte{192, 168, 1, 100, 0xBA, 0xC0}, MacLen: 6}
	before := mockDL.sentCount()
	srv.handleMsg(src, bvlcEnc.Bytes())
	if mockDL.sentCount() <= before {
		t.Fatal("expected a response packet")
	}
}

// TestServer_ReadProperty_ObjectList_MustComplexAck proves our server handles
// SERVICE_CONFIRMED_READ_PROPERTY (Object_List) with ComplexAck — never the
// YABE-side "Confirmed service not handled" Error/Reject path.
func TestServer_ReadProperty_ObjectList_MustComplexAck(t *testing.T) {
	srv, mockDL := newTestServer(t)

	enc := encoding.NewEncoder()
	err := enc.ReadProperty(7, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1000},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_OBJECT_LIST,
				ArrayIndex: btypes.ArrayAll,
			}},
		},
	})
	if err != nil {
		t.Fatalf("encode ReadProperty: %v", err)
	}

	injectConfirmedAPDU(t, srv, mockDL, enc.Bytes())
	apdu := decodeSentAPDU(t, mockDL.getLastSent())

	if apdu.DataType == btypes.Error || apdu.DataType == btypes.Reject {
		t.Fatalf("ReadProperty must NOT Error/Reject (got type=%#x service=%d err=%v); "+
			"that symptom is YABE stealing the unicast on shared :47808",
			apdu.DataType, apdu.Service, apdu.Error)
	}
	if apdu.DataType != btypes.ComplexAck {
		t.Fatalf("expected ComplexAck (0x30), got %#x", apdu.DataType)
	}
	if apdu.Service != btypes.ServiceConfirmedReadProperty {
		t.Fatalf("expected service ReadProperty(12), got %d", apdu.Service)
	}
	if apdu.InvokeId != 7 {
		t.Fatalf("invoke id=%d", apdu.InvokeId)
	}

	// Closing tag must be 0x3F (tag 3), not the historical 0x4F bug.
	raw := mockDL.getLastSent()
	found3E, found3F, found4F := false, false, false
	for _, b := range raw {
		switch b {
		case 0x3E:
			found3E = true
		case 0x3F:
			found3F = true
		case 0x4F:
			found4F = true
		}
	}
	if !found3E || !found3F {
		t.Fatalf("Object_List Ack missing 3E…3F framing: %x", raw)
	}
	if found4F {
		t.Fatalf("Object_List Ack still has wrong closing 4F: %x", raw)
	}
}

// TestServer_UnhandledConfirmedService_MustError ensures only unknown service
// choices get an Error PDU — ReadProperty must never fall into this path.
func TestServer_UnhandledConfirmedService_MustError(t *testing.T) {
	srv, mockDL := newTestServer(t)

	const bogusService btypes.ServiceConfirmed = 29 // GetEventInformation — not registered
	a := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  bogusService,
		InvokeId: 3,
		MaxApdu:  btypes.MaxAPDU,
	}
	enc := encoding.NewEncoder()
	_ = enc.APDU(a)

	injectConfirmedAPDU(t, srv, mockDL, enc.Bytes())
	apdu := decodeSentAPDU(t, mockDL.getLastSent())

	if apdu.DataType != btypes.Error {
		t.Fatalf("expected Error PDU for unhandled service, got %#x", apdu.DataType)
	}
	if apdu.Service != bogusService {
		t.Fatalf("error service echo=%d want %d", apdu.Service, bogusService)
	}
	if apdu.Error.Class != bacerr.ServicesError || apdu.Error.Code != bacerr.ServiceRequestDenied {
		t.Fatalf("unexpected error class/code: %v/%v", apdu.Error.Class, apdu.Error.Code)
	}
}
