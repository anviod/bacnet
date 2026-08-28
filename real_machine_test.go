//go:build integration

package bacnet

import (
	"net"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

func TestRealMachineDiscovery(t *testing.T) {
	const (
		localIP       = "192.168.3.115"
		remoteIP      = "192.168.3.230"
		readTimeout   = 10 * time.Second
		clientPort    = 47820
	)

	type DeviceConfig struct {
		ID   int
		Port int
		Type string
	}

	deviceConfigs := []DeviceConfig{
		{ID: 1234, Port: 47810, Type: "room-simulator"},
		{ID: 2228316, Port: 47808, Type: "Yabe simulator"},
		{ID: 2228317, Port: 47808, Type: "Yabe simulator"},
		{ID: 2228318, Port: 47808, Type: "Yabe simulator"},
	}

	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("         真机测试: 远程设备发现本机设备")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("本地 IP: %s, 远程 IP: %s, 客户端端口: %d", localIP, remoteIP, clientPort)

	startAll := time.Now()

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         remoteIP,
		Port:       clientPort,
		SubnetCIDR: 24,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("❌ NewClient 失败: %v", err)
	}
	defer bacClient.Close()
	go bacClient.ClientRun()
	time.Sleep(500 * time.Millisecond)
	t.Logf("✓ 客户端运行在端口 %d", clientPort)

	confirmedDevices := make(map[int]btypes.Device)
	unfoundDevices := make(map[int]DeviceConfig)
	for _, cfg := range deviceConfigs {
		unfoundDevices[cfg.ID] = cfg
	}

	// ── Step 1: Unicast WhoIs ──
	t.Log("")
	t.Log("── Step 1: Unicast WhoIs ──")
	seenTargets := make(map[int]bool)
	for _, cfg := range deviceConfigs {
		if seenTargets[cfg.Port] {
			continue
		}
		seenTargets[cfg.Port] = true
		t.Logf("  Unicast WhoIs -> %s:%d", localIP, cfg.Port)
		unicastAddr := datalink.IPPortToAddress(net.ParseIP(localIP), cfg.Port)
		devices, err := bacClient.WhoIs(&WhoIsOpts{
			Low: 0, High: 4194304, Destination: unicastAddr,
		})
		if err != nil {
			t.Logf("    ⚠ 失败: %v", err)
		} else {
			t.Logf("    ✓ 收到 %d 个 I-Am", len(devices))
			for _, dev := range devices {
				t.Logf("    ✉️ DeviceID=%d, IP=%s:%d", dev.DeviceID, dev.Ip, dev.Port)
				if _, ok := unfoundDevices[dev.DeviceID]; ok {
					confirmedDevices[dev.DeviceID] = btypes.Device{
						DeviceID: dev.DeviceID, Addr: dev.Addr, Ip: dev.Ip, Port: dev.Port,
						MaxApdu: btypes.MaxAPDU,
						ID:      btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(dev.DeviceID)},
					}
					delete(unfoundDevices, dev.DeviceID)
					t.Logf("    ✓ Device %d 已确认", dev.DeviceID)
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}

	// ── Step 2: Direct ReadProperty + diagnostic ──
	if len(unfoundDevices) > 0 {
		t.Log("")
		t.Log("── Step 2: ReadProperty 诊断 ──")

		// First, try Device 2228316 to confirm it works
		t.Log("")
		t.Log("  [基准] Device 2228316 (应正常工作):")
		addr8316 := datalink.IPPortToAddress(net.ParseIP(localIP), 47808)
		dev8316 := btypes.Device{
			DeviceID: 2228316, Addr: *addr8316, Ip: localIP, Port: 47808, MaxApdu: btypes.MaxAPDU,
			ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2228316},
		}
		rp8316, err8316 := bacClient.ReadPropertyWithTimeout(dev8316, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2228316},
				Properties: []btypes.Property{{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll}},
			},
		}, readTimeout)
		if err8316 != nil {
			t.Logf("    ⚠ 失败: %v", err8316)
		} else if len(rp8316.Object.Properties) > 0 {
			t.Logf("    ✓ ObjectName: %v", rp8316.Object.Properties[0].Data)
			confirmedDevices[2228316] = dev8316
			delete(unfoundDevices, 2228316)
		}

		// Read ObjectList from 2228316
		t.Logf("    → 读取 ObjectList...")
		rpList, errList := bacClient.ReadPropertyWithTimeout(dev8316, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 2228316},
				Properties: []btypes.Property{{Type: btypes.PropObjectList, ArrayIndex: 0}},
			},
		}, readTimeout)
		if errList != nil {
			t.Logf("    ⚠ ObjectList 失败: %v", errList)
		} else if len(rpList.Object.Properties) > 0 {
			t.Logf("    ✓ ObjectList: %v", rpList.Object.Properties[0].Data)
		}

		time.Sleep(300 * time.Millisecond)

		// Now diagnose 2228317
		for _, cfg := range []DeviceConfig{{ID: 2228317, Port: 47808}, {ID: 2228318, Port: 47808}} {
			t.Log("")
			t.Logf("  [诊断] Device %d:", cfg.ID)
			addr := datalink.IPPortToAddress(net.ParseIP(localIP), cfg.Port)
			testDev := btypes.Device{
				DeviceID: cfg.ID, Addr: *addr, Ip: localIP, Port: cfg.Port, MaxApdu: btypes.MaxAPDU,
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(cfg.ID)},
			}

			// Try ObjectList first
			t.Logf("    → 读取 ObjectList (76)...")
			rpList, errList := bacClient.ReadPropertyWithTimeout(testDev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(cfg.ID)},
					Properties: []btypes.Property{{Type: btypes.PropObjectList, ArrayIndex: 0}},
				},
			}, readTimeout)
			if errList != nil {
				t.Logf("    ⚠ ObjectList: %v", errList)
			} else if len(rpList.Object.Properties) > 0 {
				t.Logf("    ✓ ObjectList: %v", rpList.Object.Properties[0].Data)
			}

			// Try ObjectName
			rpName, errName := bacClient.ReadPropertyWithTimeout(testDev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(cfg.ID)},
					Properties: []btypes.Property{{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll}},
				},
			}, readTimeout)
			if errName != nil {
				t.Logf("    ⚠ ObjectName: %v", errName)
			} else if len(rpName.Object.Properties) > 0 {
				t.Logf("    ✓ ObjectName: %v", rpName.Object.Properties[0].Data)
				confirmedDevices[cfg.ID] = testDev
				delete(unfoundDevices, cfg.ID)
			}

			// Try Objects() function (ReadMultiProperty)
			t.Logf("    → 尝试 Objects() 全量读取...")
			objs, errObjs := bacClient.Objects(testDev)
			if errObjs != nil {
				t.Logf("    ⚠ Objects() 失败: %v", errObjs)
			} else {
				t.Logf("    ✓ Objects() 成功, 对象数: %d", len(objs.Objects))
				confirmedDevices[cfg.ID] = testDev
				delete(unfoundDevices, cfg.ID)
			}

			time.Sleep(300 * time.Millisecond)
		}

		// Also try room-simulator 1234
		if _, ok := unfoundDevices[1234]; ok {
			t.Log("")
			t.Log("  [诊断] Device 1234 (room-simulator):")
			addr1234 := datalink.IPPortToAddress(net.ParseIP(localIP), 47810)
			dev1234 := btypes.Device{
				DeviceID: 1234, Addr: *addr1234, Ip: localIP, Port: 47810, MaxApdu: btypes.MaxAPDU,
				ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
			}
			rp1234, err1234 := bacClient.ReadPropertyWithTimeout(dev1234, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: btypes.DeviceType, Instance: 1234},
					Properties: []btypes.Property{{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll}},
				},
			}, readTimeout)
			if err1234 != nil {
				t.Logf("    ⚠ 失败: %v", err1234)
			} else if len(rp1234.Object.Properties) > 0 {
				t.Logf("    ✓ ObjectName: %v", rp1234.Object.Properties[0].Data)
				confirmedDevices[1234] = dev1234
				delete(unfoundDevices, 1234)
			}
		}
	}

	discoveryFailures := len(deviceConfigs) - len(confirmedDevices)
	if discoveryFailures > 0 {
		t.Logf("")
		t.Logf("❌ 真机测试失败: %d 台设备无法通信", discoveryFailures)
		for _, cfg := range unfoundDevices {
			t.Logf("   - Device %d (%s)", cfg.ID, cfg.Type)
		}
		t.Fail()
	} else {
		t.Logf("")
		t.Logf("✅ 真机测试完成: %d/%d 设备全部验证成功", len(confirmedDevices), len(deviceConfigs))
	}
	t.Logf("总耗时: %.3fs", time.Since(startAll).Seconds())
}