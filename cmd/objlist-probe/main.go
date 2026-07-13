// Command objlist-probe prints ReadPropertyAck APDU hex for Object_List
// (ArrayAll / index 0 / index 1) so encodings can be compared with Wireshark / YABE.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/encoding"
)

func main() {
	list := []btypes.ObjectID{
		{Type: btypes.DeviceType, Instance: 1234},
		{Type: btypes.AnalogInput, Instance: 1},
		{Type: btypes.AnalogValue, Instance: 1},
		{Type: btypes.BinaryInput, Instance: 1},
		{Type: btypes.MultiStateValue, Instance: 1},
	}

	cases := []struct {
		name string
		pd   btypes.PropertyData
	}{
		{"ArrayAll", btypes.PropertyData{Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type: btypes.PROP_OBJECT_LIST, ArrayIndex: btypes.ArrayAll, Data: list,
			}},
		}}},
		{"Index0", btypes.PropertyData{Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type: btypes.PROP_OBJECT_LIST, ArrayIndex: 0, Data: uint32(len(list)),
			}},
		}}},
		{"Index1", btypes.PropertyData{Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			Properties: []btypes.Property{{
				Type: btypes.PROP_OBJECT_LIST, ArrayIndex: 1, Data: list[0],
			}},
		}}},
	}

	for i, c := range cases {
		enc := encoding.NewEncoder()
		if err := enc.ReadPropertyAck(uint8(i+1), c.pd); err != nil {
			fmt.Fprintf(os.Stderr, "%s encode: %v\n", c.name, err)
			os.Exit(1)
		}
		raw := enc.Bytes()
		closeTag := raw[len(raw)-1]
		fmt.Printf("=== %s (len=%d close=0x%02X) ===\n%s\n", c.name, len(raw), closeTag, hex.EncodeToString(raw))
		if closeTag != 0x3f {
			fmt.Fprintf(os.Stderr, "FAIL: closing tag must be 0x3F, got 0x%02X\n", closeTag)
			os.Exit(1)
		}
	}
	fmt.Println("OK: all Acks close with tag 3 (0x3F)")
}
