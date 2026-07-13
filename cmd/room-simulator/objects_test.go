package main

import (
	"strconv"
	"testing"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/server"
)

func TestDefaultRoomObjects_UniqueAndNamed(t *testing.T) {
	defs := DefaultRoomObjects()
	if len(defs) < 10 {
		t.Fatalf("expected at least 10 room objects, got %d", len(defs))
	}

	seen := make(map[string]bool)
	for _, d := range defs {
		if d.Name == "" {
			t.Fatalf("object %s:%d has empty name", d.Type, d.Instance)
		}
		key := d.Type.String() + ":" + strconv.FormatUint(uint64(d.Instance), 10)
		if seen[key] {
			t.Fatalf("duplicate object %s", key)
		}
		seen[key] = true
		if d.PresentValue == nil {
			t.Fatalf("%s missing PresentValue", d.Name)
		}
	}

	wantNames := []string{
		"Space Temperature",
		"Temperature Setpoint",
		"Occupancy",
		"HVAC Mode",
		"Fan",
	}
	for _, name := range wantNames {
		found := false
		for _, d := range defs {
			if d.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing expected object name %q", name)
		}
	}
}

func TestSeedRoomObjects_ObjectList(t *testing.T) {
	cfg := server.DefaultDeviceConfig()
	cfg.DeviceID = 1234
	cfg.DeviceName = "Room Simulator"
	srv, err := server.NewServerWithDataLink(cfg, &nopDataLink{})
	if err != nil {
		t.Fatalf("NewServerWithDataLink: %v", err)
	}
	defer srv.Close()

	if err := SeedRoomObjects(srv); err != nil {
		t.Fatalf("SeedRoomObjects: %v", err)
	}

	list, ok := srv.GetProperty(btypes.DeviceType, cfg.DeviceID, btypes.PROP_OBJECT_LIST)
	if !ok {
		t.Fatal("Object_List not found")
	}
	ids, ok := list.([]btypes.ObjectID)
	if !ok {
		t.Fatalf("Object_List type %T", list)
	}

	want := 1 + len(DefaultRoomObjects())
	if len(ids) != want {
		t.Fatalf("Object_List len=%d want %d", len(ids), want)
	}

	name, ok := srv.GetProperty(btypes.AnalogInput, 1, btypes.PROP_OBJECT_NAME)
	if !ok || name != "Space Temperature" {
		t.Fatalf("AI:1 Object_Name = %v", name)
	}
	model, ok := srv.GetObjectStore().GetProperty(btypes.DeviceType, cfg.DeviceID, btypes.PROP_MODEL_NAME)
	if !ok || model != "Room Simulator" {
		t.Fatalf("Model_Name = %v", model)
	}
}

// nopDataLink satisfies datalink.DataLink for store-only tests.
type nopDataLink struct {
	closed chan struct{}
}

func (n *nopDataLink) ensure() {
	if n.closed == nil {
		n.closed = make(chan struct{})
	}
}

func (n *nopDataLink) Close() error {
	n.ensure()
	select {
	case <-n.closed:
	default:
		close(n.closed)
	}
	return nil
}

func (n *nopDataLink) Receive([]byte) (*btypes.Address, int, error) {
	n.ensure()
	<-n.closed
	return nil, 0, errClosed{}
}

func (n *nopDataLink) Send([]byte, *btypes.NPDU, *btypes.Address) (int, error) {
	return 0, nil
}
func (n *nopDataLink) GetMyAddress() *btypes.Address        { return &btypes.Address{} }
func (n *nopDataLink) GetBroadcastAddress() *btypes.Address { return &btypes.Address{} }

type errClosed struct{}

func (errClosed) Error() string { return "datalink closed" }
