package server_test

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
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port
}

func floatValue(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// TestClientServerInterop_UDP verifies that the bacnet client can discover and
// read/write objects exposed by the server package over real UDP sockets.
func TestClientServerInterop_UDP(t *testing.T) {
	serverPort := freeUDPPort(t)
	clientPort := freeUDPPort(t)

	cfg := &server.DeviceConfig{
		DeviceID:   2001,
		DeviceName: "InteropServer",
		VendorID:   999,
		Ip:         "127.0.0.1",
		Port:       serverPort,
		SubnetCIDR: 8,
		MaxPDU:     btypes.MaxAPDU,
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer srv.Close()

	err = srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(23.5)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "TempSensor"},
			{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Interop AI"},
		},
	})
	if err != nil {
		t.Fatalf("AddObject AI failed: %v", err)
	}
	err = srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(10)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Setpoint"},
			{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Interop AV"},
		},
	})
	if err != nil {
		t.Fatalf("AddObject AV failed: %v", err)
	}
	err = srv.AddObject(btypes.Object{
		ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
		Properties: []btypes.Property{
			{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
			{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Relay"},
			{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Interop BO"},
		},
	})
	if err != nil {
		t.Fatalf("AddObject BO failed: %v", err)
	}

	go func() {
		_ = srv.Serve()
	}()
	time.Sleep(100 * time.Millisecond)

	client, err := bacnet.NewClient(&bacnet.ClientBuilder{
		Ip:         "127.0.0.1",
		Port:       clientPort,
		SubnetCIDR: 8,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()
	go client.ClientRun()
	time.Sleep(50 * time.Millisecond)

	serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)

	t.Run("WhoIs", func(t *testing.T) {
		devices, err := client.WhoIs(&bacnet.WhoIsOpts{
			Low:         2001,
			High:        2001,
			Destination: serverAddr,
		})
		if err != nil {
			t.Fatalf("WhoIs failed: %v", err)
		}
		if len(devices) == 0 {
			t.Fatal("WhoIs discovered 0 devices")
		}
		found := false
		for _, d := range devices {
			if d.DeviceID == 2001 {
				found = true
				if d.Vendor != 999 {
					t.Errorf("expected vendor 999, got %d", d.Vendor)
				}
			}
		}
		if !found {
			t.Fatalf("device 2001 not found in %v", devices)
		}
	})

	devices, err := client.WhoIs(&bacnet.WhoIsOpts{
		Low:         2001,
		High:        2001,
		Destination: serverAddr,
	})
	if err != nil || len(devices) == 0 {
		t.Fatalf("setup WhoIs failed: %v devices=%d", err, len(devices))
	}
	dev := devices[0]
	dev.Addr = *serverAddr
	dev.MaxApdu = btypes.MaxAPDU
	dev.ID = btypes.ObjectID{Type: btypes.DeviceType, Instance: 2001}

	t.Run("ReadProperty_AnalogInput", func(t *testing.T) {
		result, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
				Properties: []btypes.Property{
					{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
				},
			},
		}, 3*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty failed: %v", err)
		}
		if len(result.Object.Properties) == 0 {
			t.Fatal("no properties returned")
		}
		val, ok := floatValue(result.Object.Properties[0].Data)
		if !ok {
			t.Fatalf("unexpected value type %T", result.Object.Properties[0].Data)
		}
		if val < 23.4 || val > 23.6 {
			t.Fatalf("expected ~23.5, got %v", val)
		}
	})

	t.Run("WriteProperty_AnalogValue", func(t *testing.T) {
		err := client.WriteProperty(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
				Properties: []btypes.Property{
					{
						Type:       btypes.PROP_PRESENT_VALUE,
						ArrayIndex: btypes.ArrayAll,
						Data:       float32(42.25),
						Priority:   btypes.Normal,
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("WriteProperty failed: %v", err)
		}

		stored, found := srv.GetProperty(btypes.AnalogValue, 1, btypes.PROP_PRESENT_VALUE)
		if !found {
			t.Fatal("property missing after write")
		}
		val, ok := floatValue(stored)
		if !ok {
			t.Fatalf("unexpected stored type %T", stored)
		}
		if val < 42.2 || val > 42.3 {
			t.Fatalf("expected ~42.25 stored, got %v", val)
		}

		result, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
				Properties: []btypes.Property{
					{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
				},
			},
		}, 3*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty after write failed: %v", err)
		}
		val, ok = floatValue(result.Object.Properties[0].Data)
		if !ok || val < 42.2 || val > 42.3 {
			t.Fatalf("readback expected ~42.25, got %v (%T)", result.Object.Properties[0].Data, result.Object.Properties[0].Data)
		}
	})

	t.Run("ReadMultiProperty", func(t *testing.T) {
		result, err := client.ReadMultiPropertyWithTimeout(dev, btypes.MultiplePropertyData{
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
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadMultiProperty failed: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Fatalf("expected 2 objects, got %d", len(result.Objects))
		}
		if len(result.Objects[0].Properties) < 2 {
			t.Fatalf("expected >=2 properties on AI, got %d", len(result.Objects[0].Properties))
		}
		name, ok := result.Objects[0].Properties[1].Data.(string)
		if !ok || name != "TempSensor" {
			t.Fatalf("expected object name TempSensor, got %v", result.Objects[0].Properties[1].Data)
		}
	})

	t.Run("WriteMultiProperty", func(t *testing.T) {
		err := client.WriteMultiProperty(dev, btypes.MultiplePropertyData{
			Objects: []btypes.Object{
				{
					ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
					Properties: []btypes.Property{
						{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(55.5)},
					},
				},
				{
					ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
					Properties: []btypes.Property{
						{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(1)},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("WriteMultiProperty failed: %v", err)
		}

		av, found := srv.GetProperty(btypes.AnalogValue, 1, btypes.PROP_PRESENT_VALUE)
		if !found {
			t.Fatal("AV missing after WriteMulti")
		}
		if val, ok := floatValue(av); !ok || val < 55.4 || val > 55.6 {
			t.Fatalf("AV expected ~55.5, got %v", av)
		}
		bo, found := srv.GetProperty(btypes.BinaryOutput, 1, btypes.PROP_PRESENT_VALUE)
		if !found {
			t.Fatal("BO missing after WriteMulti")
		}
		switch v := bo.(type) {
		case uint32:
			if v != 1 {
				t.Fatalf("BO expected 1, got %d", v)
			}
		case btypes.Enumerated:
			if uint32(v) != 1 {
				t.Fatalf("BO expected 1, got %d", v)
			}
		default:
			t.Fatalf("BO unexpected type %T (%v)", bo, bo)
		}
	})

	t.Run("ObjectList_Length", func(t *testing.T) {
		result, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2001},
				Properties: []btypes.Property{
					{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: 0},
				},
			},
		}, 3*time.Second)
		if err != nil {
			t.Fatalf("ObjectList length read failed: %v", err)
		}
		length, ok := result.Object.Properties[0].Data.(uint32)
		if !ok {
			t.Fatalf("expected uint32 length, got %T", result.Object.Properties[0].Data)
		}
		// Device + AI + AV + BO
		if length < 4 {
			t.Fatalf("expected object list length >= 4, got %d", length)
		}
	})

	t.Run("ObjectList_FullAndByIndex", func(t *testing.T) {
		full, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2001},
				Properties: []btypes.Property{
					{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: btypes.ArrayAll},
				},
			},
		}, 3*time.Second)
		if err != nil {
			t.Fatalf("ObjectList full read failed: %v", err)
		}
		var n int
		switch v := full.Object.Properties[0].Data.(type) {
		case []interface{}:
			n = len(v)
			first, ok := v[0].(btypes.ObjectID)
			if !ok || first.Type != btypes.DeviceType || first.Instance != 2001 {
				t.Fatalf("full[0]=%#v", v[0])
			}
		case []btypes.ObjectID:
			n = len(v)
			if v[0].Instance != 2001 {
				t.Fatalf("full[0]=%#v", v[0])
			}
		default:
			t.Fatalf("unexpected full type %T", full.Object.Properties[0].Data)
		}

		el, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2001},
				Properties: []btypes.Property{
					{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: 1},
				},
			},
		}, 3*time.Second)
		if err != nil {
			t.Fatalf("ObjectList index1 failed: %v", err)
		}
		id, ok := el.Object.Properties[0].Data.(btypes.ObjectID)
		if !ok || id.Instance != 2001 {
			t.Fatalf("index1=%#v", el.Object.Properties[0].Data)
		}

		for i := uint32(2); i <= uint32(n); i++ {
			item, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2001},
					Properties: []btypes.Property{
						{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: i},
					},
				},
			}, 3*time.Second)
			if err != nil {
				t.Fatalf("ObjectList index %d failed: %v", i, err)
			}
			if _, ok := item.Object.Properties[0].Data.(btypes.ObjectID); !ok {
				t.Fatalf("index %d type %T", i, item.Object.Properties[0].Data)
			}
		}
	})

	t.Run("ReadProperty_UnknownObject", func(t *testing.T) {
		_, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 99},
				Properties: []btypes.Property{
					{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
				},
			},
		}, 3*time.Second)
		if err == nil {
			t.Fatal("expected error for unknown object")
		}
	})

	t.Run("Objects_Scan", func(t *testing.T) {
		scanned, err := client.Objects(dev)
		if err != nil {
			t.Fatalf("Objects scan failed: %v", err)
		}
		ai, ok := scanned.Objects[btypes.AnalogInput]
		if !ok || ai[1].ID.Instance != 1 {
			t.Fatalf("AnalogInput:1 missing from scan: %v", scanned.Objects)
		}
		if ai[1].Name == "" {
			t.Logf("warning: AI name empty after scan (description/name RPM may be partial)")
		}
		fmt.Printf("Objects scan OK: types=%d\n", len(scanned.Objects))
	})

	t.Run("SubscribeCOV_Unconfirmed", func(t *testing.T) {
		const processID = uint32(42)
		err := client.SubscribeCOV(dev, btypes.SubscribeCOVData{
			ProcessID:                   processID,
			ObjectID:                    btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
			IssueConfirmedNotifications: false,
			Lifetime:                    60,
		})
		if err != nil {
			t.Fatalf("SubscribeCOV failed: %v", err)
		}

		// Initial notification after subscribe
		notif, err := client.WaitCOVNotification(int64(processID), 3*time.Second)
		if err != nil {
			t.Fatalf("initial COV notification: %v", err)
		}
		if notif.MonitoredObjectID.Type != btypes.AnalogValue || notif.MonitoredObjectID.Instance != 1 {
			t.Fatalf("unexpected monitored object %#v", notif.MonitoredObjectID)
		}

		// Change triggers another notification
		if err := srv.SetProperty(btypes.AnalogValue, 1, btypes.PROP_PRESENT_VALUE, float64(99.5)); err != nil {
			t.Fatalf("SetProperty: %v", err)
		}
		notif2, err := client.WaitCOVNotification(int64(processID), 3*time.Second)
		if err != nil {
			t.Fatalf("change COV notification: %v", err)
		}
		if len(notif2.ListOfValues) == 0 {
			t.Fatal("empty listOfValues")
		}
		val, ok := floatValue(notif2.ListOfValues[0].Data)
		if !ok || val < 99.4 || val > 99.6 {
			t.Fatalf("expected ~99.5, got %v (%T)", notif2.ListOfValues[0].Data, notif2.ListOfValues[0].Data)
		}

		if err := client.CancelSubscribeCOV(dev, processID, btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1}); err != nil {
			t.Fatalf("CancelSubscribeCOV: %v", err)
		}

		// After cancel, change should not notify (wait should time out)
		_ = srv.SetProperty(btypes.AnalogValue, 1, btypes.PROP_PRESENT_VALUE, float64(1))
		if _, err := client.WaitCOVNotification(int64(processID), 400*time.Millisecond); err == nil {
			t.Fatal("expected no COV after cancel")
		}
	})
}
