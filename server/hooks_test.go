package server

import (
	"fmt"
	"testing"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/encoding"
)

// =============================================================================
// Property Write Hook Tests
// =============================================================================

// confirmedRequestHeaderLen is the APDU header size of a non-segmented
// ConfirmedServiceRequest (meta + max-segs/max-apdu + invoke-id + service).
// Encoder methods write it before the service payload, so it must be stripped
// to obtain the raw payload expected by the server-side decoders.
const confirmedRequestHeaderLen = 4

func TestServer_OnPropertyWrite_WriteProperty(t *testing.T) {
	srv, _ := newTestServer(t)

	var got []PropertyWriteEvent
	srv.OnPropertyWrite(func(evt PropertyWriteEvent) {
		got = append(got, evt)
	})

	// Build WriteProperty: BinaryOutput 1, Present_Value = 0
	enc := encoding.NewEncoder()
	wp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
			},
		},
	}
	if err := enc.WriteProperty(1, wp); err != nil {
		t.Fatalf("failed to encode WriteProperty: %v", err)
	}

	rawData := enc.Bytes()[confirmedRequestHeaderLen:]
	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedWriteProperty,
		InvokeId: 1,
		RawData:  rawData,
		MaxApdu:  btypes.MaxAPDU,
	}
	msg := buildBACnetMessage(t, buildAPDUHeader(t, apdu), apdu.RawData, false)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	srv.handleMsg(src, msg)

	if len(got) != 1 {
		t.Fatalf("expected 1 hook event, got %d", len(got))
	}
	evt := got[0]
	if evt.ObjectType != btypes.BinaryOutput || evt.ObjectInstance != 1 {
		t.Errorf("unexpected target object: %s instance %d", evt.ObjectType, evt.ObjectInstance)
	}
	if evt.PropertyType != btypes.PROP_PRESENT_VALUE {
		t.Errorf("expected PROP_PRESENT_VALUE, got %v", evt.PropertyType)
	}
	if fmt.Sprintf("%v", evt.OldValue) != "1" {
		t.Errorf("expected old value 1, got %v", evt.OldValue)
	}
	if fmt.Sprintf("%v", evt.NewValue) != "0" {
		t.Errorf("expected new value 0, got %v", evt.NewValue)
	}
	if evt.Source == nil {
		t.Error("expected non-nil Source for a remote write")
	}
}

func TestServer_OnPropertyWrite_WriteMultiProperty(t *testing.T) {
	srv, _ := newTestServer(t)

	var got []PropertyWriteEvent
	srv.OnPropertyWrite(func(evt PropertyWriteEvent) {
		got = append(got, evt)
	})

	// Build WritePropertyMultiple: AI:1 PV=21.5 and BO:1 PV=0
	enc := encoding.NewEncoder()
	wmp := btypes.MultiplePropertyData{
		Objects: []btypes.Object{
			{
				ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
				Properties: []btypes.Property{
					{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(21.5)},
				},
			},
			{
				ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
				Properties: []btypes.Property{
					{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
				},
			},
		},
	}
	if err := enc.WriteMultiProperty(1, wmp); err != nil {
		t.Fatalf("failed to encode WritePropertyMultiple: %v", err)
	}

	rawData := enc.Bytes()[confirmedRequestHeaderLen:]
	apdu := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedWritePropMultiple,
		InvokeId: 1,
		RawData:  rawData,
		MaxApdu:  btypes.MaxAPDU,
	}
	msg := buildBACnetMessage(t, buildAPDUHeader(t, apdu), apdu.RawData, false)
	src := &btypes.Address{Adr: []byte{192, 168, 1, 100}, Len: 4}

	srv.handleMsg(src, msg)

	if len(got) != 2 {
		t.Fatalf("expected 2 hook events, got %d", len(got))
	}

	first := got[0]
	if first.ObjectType != btypes.AnalogInput || first.ObjectInstance != 1 {
		t.Errorf("unexpected first target object: %s instance %d", first.ObjectType, first.ObjectInstance)
	}
	if fmt.Sprintf("%v", first.OldValue) != "23.5" {
		t.Errorf("expected old value 23.5, got %v", first.OldValue)
	}
	if fmt.Sprintf("%v", first.NewValue) != "21.5" {
		t.Errorf("expected new value 21.5, got %v", first.NewValue)
	}

	second := got[1]
	if second.ObjectType != btypes.BinaryOutput || second.ObjectInstance != 1 {
		t.Errorf("unexpected second target object: %s instance %d", second.ObjectType, second.ObjectInstance)
	}
	if fmt.Sprintf("%v", second.NewValue) != "0" {
		t.Errorf("expected new value 0, got %v", second.NewValue)
	}
}

func TestServer_OnPropertyWrite_LocalSetProperty(t *testing.T) {
	srv, _ := newTestServer(t)

	var got []PropertyWriteEvent
	srv.OnPropertyWrite(func(evt PropertyWriteEvent) {
		got = append(got, evt)
	})

	if err := srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float64(30.5)); err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 hook event, got %d", len(got))
	}
	evt := got[0]
	if evt.Source != nil {
		t.Error("expected nil Source for a local write")
	}
	if fmt.Sprintf("%v", evt.OldValue) != "23.5" {
		t.Errorf("expected old value 23.5, got %v", evt.OldValue)
	}
	if fmt.Sprintf("%v", evt.NewValue) != "30.5" {
		t.Errorf("expected new value 30.5, got %v", evt.NewValue)
	}
}

func TestServer_OnPropertyWrite_Unsubscribe(t *testing.T) {
	srv, _ := newTestServer(t)

	called := 0
	unsubscribe := srv.OnPropertyWrite(func(evt PropertyWriteEvent) {
		called++
	})
	unsubscribe()

	if err := srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float64(19.0)); err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}

	if called != 0 {
		t.Errorf("expected hook not to be called after unsubscribe, called %d times", called)
	}
}

func TestServer_OnPropertyWrite_PanickingHookDoesNotBreakServer(t *testing.T) {
	srv, _ := newTestServer(t)

	called := 0
	srv.OnPropertyWrite(func(evt PropertyWriteEvent) {
		panic("boom")
	})
	srv.OnPropertyWrite(func(evt PropertyWriteEvent) {
		called++
	})

	if err := srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float64(18.0)); err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}

	if called != 1 {
		t.Errorf("expected second hook to still be called, called %d times", called)
	}
}
