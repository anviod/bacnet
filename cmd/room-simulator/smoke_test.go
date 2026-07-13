package main

import (
	"fmt"
	"net"
	"testing"
	"time"

	bacnet "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
	"github.com/anviod/bacnet/server"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port
}

// TestRoomSimulatorSmoke_WhoIsObjectsRead runs a client against a seeded room simulator over UDP.
func TestRoomSimulatorSmoke_WhoIsObjectsRead(t *testing.T) {
	serverPort := freeUDPPort(t)
	clientPort := freeUDPPort(t)
	const deviceID = 1234

	cfg := &server.DeviceConfig{
		DeviceID:   btypes.ObjectInstance(deviceID),
		DeviceName: "Room Simulator",
		VendorID:   999,
		Ip:         "127.0.0.1",
		Port:       serverPort,
		SubnetCIDR: 8,
		MaxPDU:     btypes.MaxAPDU,
	}
	srv, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()
	if err := SeedRoomObjects(srv); err != nil {
		t.Fatalf("SeedRoomObjects: %v", err)
	}
	go func() { _ = srv.Serve() }()
	time.Sleep(100 * time.Millisecond)

	client, err := bacnet.NewClient(&bacnet.ClientBuilder{
		Ip:         "127.0.0.1",
		Port:       clientPort,
		SubnetCIDR: 8,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	go client.ClientRun()
	time.Sleep(50 * time.Millisecond)

	serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
	devices, err := client.WhoIs(&bacnet.WhoIsOpts{
		Low:         deviceID,
		High:        deviceID,
		Destination: serverAddr,
	})
	if err != nil {
		t.Fatalf("WhoIs: %v", err)
	}
	if len(devices) == 0 {
		t.Fatal("WhoIs returned no devices")
	}
	found := false
	for _, d := range devices {
		if d.DeviceID == deviceID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("device %d not found in %v", deviceID, devices)
	}

	dev := devices[0]
	dev.Addr = *serverAddr
	dev.MaxApdu = btypes.MaxAPDU
	dev.ID = btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(deviceID)}

	// Explicit Object_List checks (what YABE uses to populate the object tree).
	lenRP, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
		Object: btypes.Object{
			ID:         dev.ID,
			Properties: []btypes.Property{{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: 0}},
		},
	}, 3*time.Second)
	if err != nil {
		t.Fatalf("Object_List index0: %v", err)
	}
	objCount, ok := lenRP.Object.Properties[0].Data.(uint32)
	if !ok || objCount < 10 {
		t.Fatalf("Object_List length=%v", lenRP.Object.Properties[0].Data)
	}
	fullRP, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
		Object: btypes.Object{
			ID:         dev.ID,
			Properties: []btypes.Property{{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: btypes.ArrayAll}},
		},
	}, 3*time.Second)
	if err != nil {
		t.Fatalf("Object_List full: %v", err)
	}
	switch v := fullRP.Object.Properties[0].Data.(type) {
	case []interface{}:
		if uint32(len(v)) != objCount {
			t.Fatalf("full len=%d index0=%d", len(v), objCount)
		}
	case []btypes.ObjectID:
		if uint32(len(v)) != objCount {
			t.Fatalf("full len=%d index0=%d", len(v), objCount)
		}
	default:
		t.Fatalf("full type %T", fullRP.Object.Properties[0].Data)
	}
	idx1, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
		Object: btypes.Object{
			ID:         dev.ID,
			Properties: []btypes.Property{{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: 1}},
		},
	}, 3*time.Second)
	if err != nil {
		t.Fatalf("Object_List index1: %v", err)
	}
	if id, ok := idx1.Object.Properties[0].Data.(btypes.ObjectID); !ok || id.Instance != deviceID {
		t.Fatalf("index1=%#v", idx1.Object.Properties[0].Data)
	}

	scanned, err := client.Objects(dev)
	if err != nil {
		t.Fatalf("Objects: %v", err)
	}
	ai := scanned.Objects[btypes.AnalogInput]
	if len(ai) == 0 {
		t.Fatal("no AnalogInput objects after scan")
	}
	spaceOK := false
	for _, obj := range ai {
		if obj.Name == "Space Temperature" {
			spaceOK = true
			break
		}
	}
	if !spaceOK {
		t.Fatalf("Space Temperature not found: %v", namesOf(ai))
	}

	rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
			}},
		},
	}, 3*time.Second)
	if err != nil {
		t.Fatalf("ReadProperty Present_Value: %v", err)
	}
	if len(rp.Object.Properties) == 0 || rp.Object.Properties[0].Data == nil {
		t.Fatal("empty Present_Value response")
	}
	t.Logf("Space Temperature Present_Value = %v", rp.Object.Properties[0].Data)

	err = client.WriteProperty(dev, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
			Properties: []btypes.Property{{
				Type:       btypes.PROP_PRESENT_VALUE,
				ArrayIndex: btypes.ArrayAll,
				Data:       float32(23.0),
			}},
		},
	})
	if err != nil {
		t.Fatalf("WriteProperty setpoint: %v", err)
	}
}

func namesOf(m map[btypes.ObjectInstance]btypes.Object) []string {
	var out []string
	for inst, o := range m {
		out = append(out, fmt.Sprintf("%d:%s", inst, o.Name))
	}
	return out
}
