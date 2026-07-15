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
		broadcastPort = 47808
	)

	type DeviceConfig struct {
		ID   int
		Port int
	}

	deviceConfigs := []DeviceConfig{
		{ID: 1234, Port: 47810},
		{ID: 2228316, Port: 58494},
		{ID: 2228317, Port: 64339},
		{ID: 2228318, Port: 54304},
	}

	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("         真机测试: 远程设备发现本机设备")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("本地 IP: %s, 远程 IP: %s, 客户端端口: %d", localIP, remoteIP, clientPort)
	t.Logf("预配置设备列表:")
	for _, cfg := range deviceConfigs {
		t.Logf("  - DeviceID=%d, Port=%d", cfg.ID, cfg.Port)
	}

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

	if !bacClient.IsRunning() {
		t.Fatalf("❌ 客户端未运行")
	}
	t.Logf("✓ 客户端运行在端口 %d", clientPort)

	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              真机测试: 两步设备发现")
	t.Log("───────────────────────────────────────────────────────────────")

	confirmedDevices := make(map[int]btypes.Device)
	unfoundDevices := make(map[int]DeviceConfig)

	t.Logf("")
	t.Logf("  Step 1: 使用用户提供的 ID+IP+Port 进行直接验证...")

	for _, cfg := range deviceConfigs {
		t.Logf("")
		t.Logf("    ── Device %d 直接验证 ──", cfg.ID)

		addr := datalink.IPPortToAddress(net.ParseIP(localIP), cfg.Port)
		testDev := btypes.Device{
			DeviceID: cfg.ID,
			Addr:     *addr,
			Ip:       localIP,
			Port:     cfg.Port,
			MaxApdu:  btypes.MaxAPDU,
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: btypes.ObjectInstance(cfg.ID),
			},
		}

		rpTest, errTest := bacClient.ReadPropertyWithTimeout(testDev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{
					Type:     btypes.DeviceType,
					Instance: btypes.ObjectInstance(cfg.ID),
				},
				Properties: []btypes.Property{{
					Type:       btypes.PropObjectName,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, readTimeout)

		if errTest != nil {
			t.Logf("    ⚠ Device %d 直接验证失败: %v", cfg.ID, errTest)
			t.Logf("      └─ 加入广播扫描队列")
			unfoundDevices[cfg.ID] = cfg
		} else if len(rpTest.Object.Properties) > 0 && rpTest.Object.Properties[0].Data != nil {
			t.Logf("    ✓ Device %d (%s) 验证成功", cfg.ID, rpTest.Object.Properties[0].Data)
			t.Logf("      ├─ IP: %s", localIP)
			t.Logf("      └─ Port: %d", cfg.Port)
			confirmedDevices[cfg.ID] = testDev
		} else {
			t.Logf("    ✓ Device %d 验证成功 (无名称)", cfg.ID)
			t.Logf("      ├─ IP: %s", localIP)
			t.Logf("      └─ Port: %d", cfg.Port)
			confirmedDevices[cfg.ID] = testDev
		}
		time.Sleep(500 * time.Millisecond)
	}

	if len(unfoundDevices) > 0 {
		t.Logf("")
		t.Logf("  Step 2: 对未发现的设备使用广播方式扫描 (端口 %d)...", broadcastPort)

		broadcastAddr := datalink.IPPortToAddress(net.ParseIP(localIP), broadcastPort)
		whoIsOpts := &WhoIsOpts{
			Low:             0,
			High:            4194304,
			Destination:     broadcastAddr,
			GlobalBroadcast: false,
		}

		devices, err := bacClient.WhoIs(whoIsOpts)
		if err != nil {
			t.Logf("    ⚠ 广播扫描失败: %v", err)
		} else {
			foundCount := 0
			for _, dev := range devices {
				t.Logf("    ✉️ 收到 I-Am: DeviceID=%d, IP=%s:%d", dev.DeviceID, dev.Ip, dev.Port)
				if _, ok := unfoundDevices[dev.DeviceID]; ok {
					t.Logf("    ✓ Device %d 通过广播发现", dev.DeviceID)
					t.Logf("      ├─ IP: %s", dev.Ip)
					t.Logf("      ├─ Port: %d", dev.Port)
					t.Logf("      └─ Type: 预配置设备")
					confirmedDevices[dev.DeviceID] = dev
					delete(unfoundDevices, dev.DeviceID)
					foundCount++
				}
			}
			if foundCount > 0 {
				t.Logf("    ✓ 广播扫描成功发现 %d 台设备", foundCount)
			}
		}
	}

	discoveryFailures := len(deviceConfigs) - len(confirmedDevices)
	if discoveryFailures > 0 {
		t.Fatalf("❌ 真机测试失败: %d 台设备无法通信", discoveryFailures)
	}

	t.Logf("")
	t.Logf("✅ 真机测试完成: %d/%d 设备全部验证成功", len(confirmedDevices), len(deviceConfigs))
	t.Logf("总耗时: %.3fs", time.Since(startAll).Seconds())
}
