package encoding

import (
	"testing"

	"github.com/anviod/bacnet/btypes"
)

func TestSubscribeCOV_RoundTrip(t *testing.T) {
	enc := NewEncoder()
	in := btypes.SubscribeCOVData{
		ProcessID:                   7,
		ObjectID:                    btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
		IssueConfirmedNotifications: false,
		Lifetime:                    120,
	}
	if err := enc.SubscribeCOV(3, in); err != nil {
		t.Fatalf("encode: %v", err)
	}

	dec := NewDecoder(enc.Bytes())
	var apdu btypes.APDU
	if err := dec.APDU(&apdu); err != nil {
		t.Fatalf("apdu: %v", err)
	}
	if apdu.Service != btypes.ServiceConfirmedSubscribeCOV {
		t.Fatalf("service=%v", apdu.Service)
	}
	var out btypes.SubscribeCOVData
	body := NewDecoder(apdu.RawData)
	if err := body.SubscribeCOV(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ProcessID != 7 || out.ObjectID.Instance != 1 || out.Lifetime != 120 || out.Cancellation {
		t.Fatalf("unexpected %#v", out)
	}
}

func TestSubscribeCOV_CancelRoundTrip(t *testing.T) {
	enc := NewEncoder()
	in := btypes.SubscribeCOVData{
		ProcessID:    7,
		ObjectID:     btypes.ObjectID{Type: btypes.AnalogValue, Instance: 2},
		Cancellation: true,
	}
	if err := enc.SubscribeCOV(1, in); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := NewDecoder(enc.Bytes())
	var apdu btypes.APDU
	if err := dec.APDU(&apdu); err != nil {
		t.Fatalf("apdu: %v", err)
	}
	var out btypes.SubscribeCOVData
	if err := NewDecoder(apdu.RawData).SubscribeCOV(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Cancellation || out.ProcessID != 7 {
		t.Fatalf("unexpected %#v", out)
	}
}

func TestCOVNotification_RoundTrip(t *testing.T) {
	enc := NewEncoder()
	in := btypes.COVNotification{
		ProcessID:          9,
		InitiatingDeviceID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1000},
		MonitoredObjectID:  btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
		TimeRemaining:      60,
		ListOfValues: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: ArrayAll, Data: float32(23.5)},
		},
	}
	if err := enc.UnconfirmedCOVNotification(in); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := NewDecoder(enc.Bytes())
	var apdu btypes.APDU
	if err := dec.APDU(&apdu); err != nil {
		t.Fatalf("apdu: %v", err)
	}
	if apdu.UnconfirmedService != btypes.ServiceUnconfirmedCOVNotification {
		t.Fatalf("service=%v", apdu.UnconfirmedService)
	}
	var out btypes.COVNotification
	if err := NewDecoder(apdu.RawData).COVNotification(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ProcessID != 9 || out.MonitoredObjectID.Instance != 1 || out.TimeRemaining != 60 {
		t.Fatalf("unexpected %#v", out)
	}
	if len(out.ListOfValues) != 1 {
		t.Fatalf("list len=%d", len(out.ListOfValues))
	}
	if v, ok := out.ListOfValues[0].Data.(float32); !ok || v < 23.4 || v > 23.6 {
		t.Fatalf("value=%v (%T)", out.ListOfValues[0].Data, out.ListOfValues[0].Data)
	}
}

func TestAbort_RoundTrip(t *testing.T) {
	enc := NewEncoder()
	a := btypes.APDU{
		DataType: btypes.Abort,
		InvokeId: 5,
		RawData:  []byte{uint8(AbortReasonSegmentationNotSupported)},
	}
	if err := enc.APDU(a); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := NewDecoder(enc.Bytes())
	var out btypes.APDU
	if err := dec.APDU(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.DataType != btypes.Abort || out.InvokeId != 5 {
		t.Fatalf("unexpected %#v", out)
	}
	reason, err := AbortReasonFromAPDU(&out)
	if err != nil || reason != AbortReasonSegmentationNotSupported {
		t.Fatalf("reason=%v err=%v", reason, err)
	}
}
