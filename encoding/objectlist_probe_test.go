package encoding

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/anviod/bacnet/btypes"
)

// Fixed fixture used for golden-byte assertions (matches a minimal room device).
var goldenObjectList = []btypes.ObjectID{
	{Type: btypes.DeviceType, Instance: 1234},
	{Type: btypes.AnalogInput, Instance: 1},
	{Type: btypes.AnalogValue, Instance: 1},
	{Type: btypes.BinaryInput, Instance: 1},
	{Type: btypes.MultiStateValue, Instance: 1},
}

// Manual ASHRAE 135 encoding of ReadProperty-ACK for Device(1234).Object_List (ArrayAll).
//
//	30          Complex-ACK
//	01          invoke ID
//	0C          ReadProperty
//	0C 020004D2 context[0] Object Identifier = Device,1234
//	19 4C       context[1] Property Identifier = 76 (object-list)
//	3E          opening tag 3
//	C4 …        application Object Identifiers (5 × 5 bytes)
//	3F          closing tag 3  (MUST match opening; NOT 4F)
var goldenObjectListAPDU, _ = hex.DecodeString(
	"30010c" +
		"0c020004d2" +
		"194c" +
		"3e" +
		"c4020004d2" + // Device:1234
		"c400000001" + // AI:1
		"c400800001" + // AV:1  (type 2 << 22 | 1)
		"c400c00001" + // BI:1  (type 3 << 22 | 1)
		"c404c00001" + // MSV:1 (type 19 << 22 | 1)
		"3f",
)

func TestObjectList_ReadPropertyAck_GoldenBytes(t *testing.T) {
	enc := NewEncoder()
	err := enc.ReadPropertyAck(1, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_OBJECT_LIST,
				ArrayIndex: btypes.ArrayAll,
				Data:       goldenObjectList,
			}},
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := enc.Bytes()
	if !bytes.Equal(got, goldenObjectListAPDU) {
		t.Fatalf("Object_List ReadPropertyAck mismatch\n got: %x\nwant: %x", got, goldenObjectListAPDU)
	}
	// Explicitly guard the YABE failure mode: mismatched open/close tags.
	if got[len(got)-1] != 0x3f {
		t.Fatalf("closing tag must be 0x3F (tag 3), got 0x%02X", got[len(got)-1])
	}
	openIdx := bytes.IndexByte(got, 0x3e)
	if openIdx < 0 {
		t.Fatal("missing opening tag 0x3E")
	}
	if bytes.Contains(got, []byte{0x4f}) {
		t.Fatal("found erroneous closing tag 0x4F (tag 4); open/close must both be tag 3")
	}
}

func TestObjectList_Index0_GoldenBytes(t *testing.T) {
	// 30 02 0C  0C 020004D2  19 4C  29 00  3E  21 05  3F
	want, err := hex.DecodeString("30020c0c020004d2194c29003e21053f")
	if err != nil {
		t.Fatal(err)
	}
	enc := NewEncoder()
	if err := enc.ReadPropertyAck(2, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_OBJECT_LIST,
				ArrayIndex: 0,
				Data:       uint32(5),
			}},
		},
	}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := enc.Bytes()
	if !bytes.Equal(got, want) {
		t.Fatalf("index0 mismatch\n got: %x\nwant: %x", got, want)
	}
}

func TestObjectList_Index1_GoldenBytes(t *testing.T) {
	// 30 03 0C  0C 020004D2  19 4C  29 01  3E  C4 020004D2  3F
	want, err := hex.DecodeString("30030c0c020004d2194c29013ec4020004d23f")
	if err != nil {
		t.Fatal(err)
	}
	enc := NewEncoder()
	if err := enc.ReadPropertyAck(3, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_OBJECT_LIST,
				ArrayIndex: 1,
				Data:       goldenObjectList[0],
			}},
		},
	}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := enc.Bytes()
	if !bytes.Equal(got, want) {
		t.Fatalf("index1 mismatch\n got: %x\nwant: %x", got, want)
	}
}

func TestReadPropertyAck_OpeningClosingTagMatch(t *testing.T) {
	// Any single-value Ack must also close with tag 3 (regression for YABE).
	enc := NewEncoder()
	if err := enc.ReadPropertyAck(7, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_OBJECT_NAME,
				ArrayIndex: btypes.ArrayAll,
				Data:       "Room Simulator",
			}},
		},
	}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw := enc.Bytes()
	if raw[len(raw)-1] != 0x3f {
		t.Fatalf("Object_Name Ack closing tag = 0x%02X, want 0x3F; full=%x", raw[len(raw)-1], raw)
	}
	if bytes.Contains(raw, []byte{0x4f}) {
		t.Fatalf("spurious tag-4 closing byte in %x", raw)
	}
}

func TestObjectList_ReadPropertyAck_RoundTrip(t *testing.T) {
	list := goldenObjectList

	t.Run("ArrayAll", func(t *testing.T) {
		enc := NewEncoder()
		err := enc.ReadPropertyAck(1, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_OBJECT_LIST,
					ArrayIndex: btypes.ArrayAll,
					Data:       list,
				}},
			},
		})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		raw := enc.Bytes()
		countC4 := 0
		for _, b := range raw {
			if b == 0xC4 {
				countC4++
			}
		}
		if countC4 < len(list) {
			t.Fatalf("expected >= %d app object-id tags, got %d in %x", len(list), countC4, raw)
		}

		dec := NewDecoder(raw)
		var apdu btypes.APDU
		if err := dec.APDU(&apdu); err != nil {
			t.Fatalf("APDU: %v", err)
		}
		if len(apdu.RawData) == 0 {
			t.Fatal("ComplexAck RawData should snapshot service payload")
		}
		var out btypes.PropertyData
		if err := NewDecoder(apdu.RawData).ReadProperty(&out); err != nil {
			t.Fatalf("decode via RawData: %v", err)
		}
		vals, ok := out.Object.Properties[0].Data.([]interface{})
		if !ok {
			t.Fatalf("expected []interface{}, got %T", out.Object.Properties[0].Data)
		}
		if len(vals) != len(list) {
			t.Fatalf("len=%d want %d", len(vals), len(list))
		}
		first, ok := vals[0].(btypes.ObjectID)
		if !ok || first.Type != btypes.DeviceType || first.Instance != 1234 {
			t.Fatalf("first element = %#v", vals[0])
		}
	})

	t.Run("Index0_Length", func(t *testing.T) {
		enc := NewEncoder()
		err := enc.ReadPropertyAck(2, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_OBJECT_LIST,
					ArrayIndex: 0,
					Data:       uint32(len(list)),
				}},
			},
		})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		dec := NewDecoder(enc.Bytes())
		var apdu btypes.APDU
		_ = dec.APDU(&apdu)
		var out btypes.PropertyData
		if err := NewDecoder(apdu.RawData).ReadProperty(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Object.Properties[0].ArrayIndex != 0 {
			t.Fatalf("arrayIndex=%d", out.Object.Properties[0].ArrayIndex)
		}
		n, ok := out.Object.Properties[0].Data.(uint32)
		if !ok || int(n) != len(list) {
			t.Fatalf("length=%v (%T)", out.Object.Properties[0].Data, out.Object.Properties[0].Data)
		}
	})

	t.Run("Index1_Element", func(t *testing.T) {
		enc := NewEncoder()
		err := enc.ReadPropertyAck(3, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_OBJECT_LIST,
					ArrayIndex: 1,
					Data:       list[0],
				}},
			},
		})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		dec := NewDecoder(enc.Bytes())
		var apdu btypes.APDU
		_ = dec.APDU(&apdu)
		var out btypes.PropertyData
		if err := NewDecoder(apdu.RawData).ReadProperty(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		id, ok := out.Object.Properties[0].Data.(btypes.ObjectID)
		if !ok || id.Instance != 1234 || id.Type != btypes.DeviceType {
			t.Fatalf("element=%#v", out.Object.Properties[0].Data)
		}
	})
}

func TestAppData_NilEncodesNothing(t *testing.T) {
	enc := NewEncoder()
	if err := enc.AppData(nil, false); err != nil {
		t.Fatal(err)
	}
	if len(enc.Bytes()) != 0 {
		t.Fatalf("nil should encode empty, got %x", enc.Bytes())
	}
}
