//go:build integration

package bacnet

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/units"
	"github.com/anviod/bacnet/datalink"
	"github.com/anviod/bacnet/server"
)

func TestMasterSlaveIntegration(t *testing.T) {
	const (
		serverDeviceID = 1001
		clientDeviceID = 2001
	)

	serverPort := freeUDPPort(t)
	clientPort := freeUDPPort(t)

	srv, err := server.NewServer(&server.DeviceConfig{
		DeviceID:   btypes.ObjectInstance(serverDeviceID),
		DeviceName: "Master-Slave Test Server",
		VendorID:   999,
		Ip:         "127.0.0.1",
		Port:       serverPort,
		SubnetCIDR: 8,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	testObjects := []btypes.Object{
		{
			ID:   btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
			Name: "Test Analog Value",
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(20.0)},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Test analog value object"},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
			Name: "Test Binary Output",
			Properties: []btypes.Property{
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Test binary output object"},
			},
		},
	}
	for _, obj := range testObjects {
		if err := srv.AddObject(obj); err != nil {
			t.Fatalf("AddObject failed: %v", err)
		}
	}

	go func() { _ = srv.Serve() }()
	time.Sleep(100 * time.Millisecond)

	client, err := NewClient(&ClientBuilder{
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

	t.Run("DeviceDiscovery", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		devices, err := client.WhoIs(&WhoIsOpts{
			Low:         serverDeviceID,
			High:        serverDeviceID,
			Destination: serverAddr,
		})
		if err != nil {
			t.Fatalf("WhoIs failed: %v", err)
		}
		if len(devices) == 0 {
			t.Fatal("WhoIs returned no devices")
		}
		found := false
		for _, d := range devices {
			if d.DeviceID == serverDeviceID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Server device %d not found in WhoIs response", serverDeviceID)
		}
		t.Logf("✓ Device discovery successful: found %d devices", len(devices))
	})

	t.Run("ReadDeviceProperties", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID:         dev.ID,
				Properties: []btypes.Property{{Type: btypes.PROP_OBJECT_LIST, ArrayIndex: btypes.ArrayAll}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty Object_List failed: %v", err)
		}
		if rp.Object.Properties[0].Data == nil {
			t.Fatal("Empty Object_List response")
		}
		t.Logf("✓ Read Object_List successful")
	})

	t.Run("WriteAndReadProperty", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		err := client.WriteProperty(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       float32(25.5),
				}},
			},
		})
		if err != nil {
			t.Fatalf("WriteProperty failed: %v", err)
		}
		t.Logf("✓ WriteProperty successful")

		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty after write failed: %v", err)
		}
		if rp.Object.Properties[0].Data != float32(25.5) {
			t.Fatalf("Read value mismatch: expected 25.5, got %v", rp.Object.Properties[0].Data)
		}
		t.Logf("✓ ReadProperty after write successful: value = %v", rp.Object.Properties[0].Data)
	})

	t.Run("BinaryWriteAndRead", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		err := client.WriteProperty(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       uint32(1),
				}},
			},
		})
		if err != nil {
			t.Fatalf("WriteProperty BinaryOutput failed: %v", err)
		}
		t.Logf("✓ WriteProperty BinaryOutput successful")

		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty BinaryOutput failed: %v", err)
		}
		if rp.Object.Properties[0].Data != uint32(1) {
			t.Fatalf("Read BinaryOutput value mismatch: expected 1 (Active), got %v", rp.Object.Properties[0].Data)
		}
		t.Logf("✓ ReadProperty BinaryOutput successful: value = %v", rp.Object.Properties[0].Data)
	})

	t.Log("\n=== All Master-Slave integration tests passed ===")
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to allocate free UDP port: %v", err)
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port
}

func TestMasterSlaveWithRoomSimulator(t *testing.T) {
	const serverDeviceID = 1234

	serverPort := freeUDPPort(t)
	clientPort := freeUDPPort(t)

	srv, err := server.NewServer(&server.DeviceConfig{
		DeviceID:   btypes.ObjectInstance(serverDeviceID),
		DeviceName: "Room Simulator",
		VendorID:   999,
		Ip:         "127.0.0.1",
		Port:       serverPort,
		SubnetCIDR: 8,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	if err := seedRoomObjects(srv); err != nil {
		t.Fatalf("seedRoomObjects: %v", err)
	}

	go func() { _ = srv.Serve() }()
	time.Sleep(100 * time.Millisecond)

	client, err := NewClient(&ClientBuilder{
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

	t.Run("DiscoverRoomSimulator", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		devices, err := client.WhoIs(&WhoIsOpts{
			Low:         serverDeviceID,
			High:        serverDeviceID,
			Destination: serverAddr,
		})
		if err != nil {
			t.Fatalf("WhoIs failed: %v", err)
		}
		if len(devices) == 0 {
			t.Fatal("WhoIs returned no devices")
		}
		t.Logf("✓ Discovered Room Simulator: %d devices found", len(devices))
	})

	t.Run("ScanRoomObjects", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		scanned, err := client.Objects(dev)
		if err != nil {
			t.Fatalf("Objects scan failed: %v", err)
		}

		ai := scanned.Objects[btypes.AnalogInput]
		if len(ai) == 0 {
			t.Fatal("No AnalogInput objects found")
		}
		t.Logf("✓ Scanned %d AnalogInput objects", len(ai))

		av := scanned.Objects[btypes.AnalogValue]
		if len(av) == 0 {
			t.Fatal("No AnalogValue objects found")
		}
		t.Logf("✓ Scanned %d AnalogValue objects", len(av))

		bo := scanned.Objects[btypes.BinaryOutput]
		if len(bo) == 0 {
			t.Fatal("No BinaryOutput objects found")
		}
		t.Logf("✓ Scanned %d BinaryOutput objects", len(bo))
	})

	t.Run("ReadRoomAnalogInputs", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty AnalogInput:1 failed: %v", err)
		}
		if rp.Object.Properties[0].Data == nil {
			t.Fatal("Empty AnalogInput:1 response")
		}
		t.Logf("✓ Read Space Temperature: %v", rp.Object.Properties[0].Data)

		rp2, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty AnalogInput:2 failed: %v", err)
		}
		t.Logf("✓ Read Outdoor Temperature: %v", rp2.Object.Properties[0].Data)
	})

	t.Run("WriteRoomAnalogValues", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		err := client.WriteProperty(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       float32(24.0),
				}},
			},
		})
		if err != nil {
			t.Fatalf("WriteProperty AnalogValue:1 failed: %v", err)
		}
		t.Logf("✓ Write Temperature Setpoint: 24.0")

		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty AnalogValue:1 failed: %v", err)
		}
		if rp.Object.Properties[0].Data != float32(24.0) {
			t.Fatalf("Read value mismatch: expected 24.0, got %v", rp.Object.Properties[0].Data)
		}
		t.Logf("✓ Verified Temperature Setpoint: %v", rp.Object.Properties[0].Data)
	})

	t.Run("ControlRoomBinaryOutputs", func(t *testing.T) {
		serverAddr := datalink.IPPortToAddress(net.ParseIP("127.0.0.1"), serverPort)
		dev := btypes.Device{
			DeviceID: serverDeviceID,
			Addr:     *serverAddr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(serverDeviceID)},
		}

		err := client.WriteProperty(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
					Data:       uint32(1),
				}},
			},
		})
		if err != nil {
			t.Fatalf("WriteProperty BinaryOutput:1 failed: %v", err)
		}
		t.Logf("✓ Turned on Room Light")

		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Fatalf("ReadProperty BinaryOutput:1 failed: %v", err)
		}
		if rp.Object.Properties[0].Data != uint32(1) {
			t.Fatalf("Read BinaryOutput value mismatch: expected 1 (Active), got %v", rp.Object.Properties[0].Data)
		}
		t.Logf("✓ Verified Room Light is ON: %v", rp.Object.Properties[0].Data)
	})

	t.Log("\n=== All Room Simulator integration tests passed ===")
}

func seedRoomObjects(srv server.Server) error {
	objs := []btypes.Object{
		{
			ID:   btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
			Name: "Space Temperature",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Space Temperature"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Room space temperature"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(22.5)},
				{Type: btypes.PROP_Propertybtypes, ArrayIndex: btypes.ArrayAll, Data: uint32(units.DegreesCelsius)},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
			Name: "Outdoor Temperature",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Outdoor Temperature"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Outdoor air temperature"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(15.0)},
				{Type: btypes.PROP_Propertybtypes, ArrayIndex: btypes.ArrayAll, Data: uint32(units.DegreesCelsius)},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.AnalogInput, Instance: 3},
			Name: "Humidity",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Humidity"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Room relative humidity"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(45.0)},
				{Type: btypes.PROP_Propertybtypes, ArrayIndex: btypes.ArrayAll, Data: uint32(units.PercentRelativeHumidity)},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
			Name: "Temperature Setpoint",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Temperature Setpoint"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Space temperature setpoint"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(23.0)},
				{Type: btypes.PROP_Propertybtypes, ArrayIndex: btypes.ArrayAll, Data: uint32(units.DegreesCelsius)},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.AnalogValue, Instance: 2},
			Name: "Cooling Setpoint",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Cooling Setpoint"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Cooling setpoint"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(25.0)},
				{Type: btypes.PROP_Propertybtypes, ArrayIndex: btypes.ArrayAll, Data: uint32(units.DegreesCelsius)},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.BinaryOutput, Instance: 1},
			Name: "Light",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Light"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Room light"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
			},
		},
		{
			ID:   btypes.ObjectID{Type: btypes.BinaryValue, Instance: 1},
			Name: "Occupancy Override",
			Properties: []btypes.Property{
				{Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Occupancy Override"},
				{Type: btypes.PROP_DESCRIPTION, ArrayIndex: btypes.ArrayAll, Data: "Manual occupancy override"},
				{Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: uint32(0)},
			},
		},
	}

	for _, obj := range objs {
		if err := srv.AddObject(obj); err != nil {
			return fmt.Errorf("add object %s:%d: %w", obj.ID.Type, obj.ID.Instance, err)
		}
	}
	return nil
}
