package bacnet

import (
	"net"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

func TestMultiDeviceDiscoveryAndReadWrite(t *testing.T) {
	const (
		targetIP      = "192.168.3.115"
		targetPort    = 47808
		localPort     = 47809
		deviceIDStart = 2228316
		deviceIDEnd   = 2228318
		readTimeout   = 5 * time.Second
	)

	t.Logf("=== 多设备集成测试 ===")
	t.Logf("目标IP: %s:%d", targetIP, targetPort)
	t.Logf("本地端口: %d", localPort)
	t.Logf("设备ID范围: %d - %d", deviceIDStart, deviceIDEnd)

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         "192.168.3.115",
		Port:       localPort,
		SubnetCIDR: 24,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer bacClient.Close()
	go bacClient.ClientRun()
	time.Sleep(500 * time.Millisecond)

	if !bacClient.IsRunning() {
		t.Fatalf("Client is not running")
	}
	t.Logf("Client running on port %d", localPort)

	internalClient := bacClient.(*client)
	broadcastAddr := datalink.IPPortToAddress(net.ParseIP("192.168.3.255"), targetPort)

	t.Run("DiscoverAllDevices", func(t *testing.T) {
		t.Logf("\n--- 设备发现阶段 ---")

		foundDevices := make(map[uint32]btypes.Device)

		t.Logf("\n使用子网广播发现所有设备...")
		devices, err := bacClient.WhoIs(&WhoIsOpts{
			Low:  0,
			High: 4194304,
		})
		if err != nil {
			t.Logf("  ✗ 广播WhoIs失败: %v", err)
		} else {
			t.Logf("  ✓ 广播发现 %d 台设备", len(devices))
			for _, d := range devices {
				foundDevices[uint32(d.DeviceID)] = d
				udpAddr, _ := d.Addr.UDPAddr()
				t.Logf("    DeviceID=%d, IP=%s:%d", d.DeviceID, udpAddr.IP, udpAddr.Port)
			}
		}

		time.Sleep(1000 * time.Millisecond)

		for deviceID := deviceIDStart; deviceID <= deviceIDEnd; deviceID++ {
			if _, ok := foundDevices[uint32(deviceID)]; !ok {
				t.Logf("\n单播查询设备ID %d...", deviceID)
				for retry := 1; retry <= 3; retry++ {
					t.Logf("    重试 %d...", retry)
					devices, err := bacClient.WhoIs(&WhoIsOpts{
						Low:         int(deviceID),
						High:        int(deviceID),
						Destination: broadcastAddr,
					})
					if err != nil {
						t.Logf("    WhoIs失败: %v", err)
					} else if len(devices) > 0 {
						foundDevices[uint32(deviceID)] = devices[0]
						udpAddr, _ := devices[0].Addr.UDPAddr()
						t.Logf("    ✓ 找到设备")
						t.Logf("      DeviceID=%d, IP=%s:%d", devices[0].DeviceID, udpAddr.IP, udpAddr.Port)
						break
					} else {
						t.Logf("    重试 %d: 未找到设备", retry)
						time.Sleep(1000 * time.Millisecond)
					}
				}
				if _, ok := foundDevices[uint32(deviceID)]; !ok {
					t.Logf("  ✗ 未找到设备")
				}
			} else {
				t.Logf("  ✓ 设备 %d 已发现", deviceID)
			}
		}

		for deviceID := deviceIDStart; deviceID <= deviceIDEnd; deviceID++ {
			if _, ok := foundDevices[uint32(deviceID)]; !ok {
				t.Errorf("设备 %d 未被发现", deviceID)
			} else {
				t.Logf("✓ 设备 %d 已发现", deviceID)
			}
		}
	})

	t.Run("ScanAndReadPoints", func(t *testing.T) {
		t.Logf("\n--- 点位扫描与读取阶段 ---")

		time.Sleep(500 * time.Millisecond)

		devices, err := bacClient.WhoIs(&WhoIsOpts{
			Low:  0,
			High: 4194304,
		})
		if err != nil {
			t.Logf("  ✗ WhoIs失败: %v", err)
			return
		}
		if len(devices) == 0 {
			t.Logf("  ✗ 未找到设备")
			return
		}

		t.Logf("  ✓ 发现 %d 台设备", len(devices))

		for _, device := range devices {
			if device.DeviceID < deviceIDStart || device.DeviceID > deviceIDEnd {
				continue
			}

			udpAddr, _ := device.Addr.UDPAddr()
			t.Logf("\n--- 查询设备 %d (IP=%s:%d) ---", device.DeviceID, udpAddr.IP, udpAddr.Port)

			dev := btypes.Device{
				DeviceID: device.DeviceID,
				Addr:     device.Addr,
				MaxApdu:  btypes.MaxAPDU,
				ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(device.DeviceID)},
			}

			rp, err := internalClient.ReadPropertyBroadcast(dev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{
						Type:     btypes.DeviceType,
						Instance: btypes.ObjectInstance(device.DeviceID),
					},
					Properties: []btypes.Property{{
						Type:       btypes.PROP_OBJECT_NAME,
						ArrayIndex: btypes.ArrayAll,
					}},
				},
			}, readTimeout, *broadcastAddr)
			if err != nil {
				t.Errorf("设备 %d: ReadProperty failed: %v", device.DeviceID, err)
				continue
			}

			t.Logf("  ✓ 读取设备名称成功: %s", rp.Object.Properties[0].Data)

			for objType := btypes.AnalogInput; objType <= btypes.BinaryOutput; objType++ {
				for instance := btypes.ObjectInstance(0); instance < 5; instance++ {
					rp, err := internalClient.ReadPropertyBroadcast(dev, btypes.PropertyData{
						Object: btypes.Object{
							ID: btypes.ObjectID{Type: objType, Instance: instance},
							Properties: []btypes.Property{{
								Type:       btypes.PROP_PRESENT_VALUE,
								ArrayIndex: btypes.ArrayAll,
							}},
						},
					}, readTimeout, *broadcastAddr)
					if err != nil {
						t.Logf("  读取 %s:%d Present_Value 失败: %v", objType, instance, err)
						continue
					}

					t.Logf("  ✓ 读取 %s:%d Present_Value = %v", objType, instance, rp.Object.Properties[0].Data)
				}
			}
		}
	})

	t.Run("WriteWritablePoints", func(t *testing.T) {
		t.Logf("\n--- 写入可写点位阶段 ---")

		time.Sleep(500 * time.Millisecond)

		devices, err := bacClient.WhoIs(&WhoIsOpts{
			Low:  0,
			High: 4194304,
		})
		if err != nil {
			t.Logf("  ✗ WhoIs失败: %v", err)
			return
		}
		if len(devices) == 0 {
			t.Logf("  ✗ 未找到设备")
			return
		}

		for _, device := range devices {
			if device.DeviceID < deviceIDStart || device.DeviceID > deviceIDEnd {
				continue
			}

			t.Logf("\n--- 查询设备 %d ---", device.DeviceID)

			dev := btypes.Device{
				DeviceID: device.DeviceID,
				Addr:     device.Addr,
				MaxApdu:  btypes.MaxAPDU,
				ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(device.DeviceID)},
			}

			testValue := float64(25.5 + float64(device.DeviceID-deviceIDStart))

			err := internalClient.WritePropertyBroadcast(dev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: btypes.AnalogOutput, Instance: 0},
					Properties: []btypes.Property{{
						Type:       btypes.PROP_PRESENT_VALUE,
						ArrayIndex: btypes.ArrayAll,
						Data:       testValue,
					}},
				},
			}, *broadcastAddr)
			if err != nil {
				t.Logf("  写入 AnalogOutput:0 失败: %v", err)
				continue
			}

			t.Logf("  ✓ 写入 AnalogOutput:0 = %v", testValue)

			time.Sleep(500 * time.Millisecond)

			rp, err := internalClient.ReadPropertyBroadcast(dev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: btypes.AnalogOutput, Instance: 0},
					Properties: []btypes.Property{{
						Type:       btypes.PROP_PRESENT_VALUE,
						ArrayIndex: btypes.ArrayAll,
					}},
				},
			}, readTimeout, *broadcastAddr)
			if err != nil {
				t.Logf("  验证写入值失败: %v", err)
				continue
			}

			if val, ok := rp.Object.Properties[0].Data.(float64); ok && val == testValue {
				t.Logf("  ✓ 验证成功: AnalogOutput:0 = %v", val)
			} else {
				t.Logf("  ✗ 验证失败: 期望值=%v, 实际值=%v", testValue, rp.Object.Properties[0].Data)
			}
		}
	})

	t.Logf("\n=== 多设备集成测试完成 ===")
}

func TestMultiDeviceQuickScan(t *testing.T) {
	const (
		targetIP      = "192.168.3.115"
		targetPort    = 47808
		localPort     = 47809
		deviceIDStart = 2228316
		deviceIDEnd   = 2228318
	)

	t.Logf("=== 快速扫描测试 ===")
	t.Logf("目标IP: %s:%d", targetIP, targetPort)
	t.Logf("本地端口: %d", localPort)
	t.Logf("设备ID: %d ~ %d", deviceIDStart, deviceIDEnd)

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         "192.168.3.115",
		Port:       localPort,
		SubnetCIDR: 24,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer bacClient.Close()
	go bacClient.ClientRun()
	time.Sleep(500 * time.Millisecond)

	if !bacClient.IsRunning() {
		t.Fatalf("Client is not running")
	}
	t.Logf("Client running on port %d", localPort)

	t.Logf("\n--- 子网广播查询所有设备 ---")
	devices, err := bacClient.WhoIs(&WhoIsOpts{
		Low:  0,
		High: 4194304,
	})
	if err != nil {
		t.Fatalf("WhoIs failed: %v", err)
	}

	t.Logf("  ✓ 找到 %d 台设备", len(devices))
	for _, d := range devices {
		udpAddr, _ := d.Addr.UDPAddr()
		t.Logf("    DeviceID=%d, IP=%s:%d", d.DeviceID, udpAddr.IP, udpAddr.Port)
	}

	t.Logf("\n--- 单播查询目标设备 ---")

	internalClient := bacClient.(*client)
	broadcastAddr := datalink.IPPortToAddress(net.ParseIP("192.168.3.255"), targetPort)

	for deviceID := deviceIDStart; deviceID <= deviceIDEnd; deviceID++ {
		t.Logf("\n查询设备ID %d...", deviceID)

		devices, err := bacClient.WhoIs(&WhoIsOpts{
			Low:         int(deviceID),
			High:        int(deviceID),
			Destination: broadcastAddr,
		})
		if err != nil {
			t.Logf("  ✗ WhoIs失败: %v", err)
			continue
		}
		if len(devices) == 0 {
			t.Logf("  ✗ 未找到设备")
			continue
		}

		t.Logf("  ✓ 找到设备")
		device := devices[0]
		udpAddr, _ := device.Addr.UDPAddr()
		t.Logf("    DeviceID=%d, IP=%s:%d", device.DeviceID, udpAddr.IP, udpAddr.Port)

		dev := btypes.Device{
			DeviceID: device.DeviceID,
			Addr:     device.Addr,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(device.DeviceID)},
		}

		rp, err := internalClient.ReadPropertyBroadcast(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 0},
				Properties: []btypes.Property{{
					Type:       btypes.PROP_PRESENT_VALUE,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second, *broadcastAddr)
		if err != nil {
			t.Logf("    ✗ 读取AnalogInput:0失败: %v", err)
			continue
		}

		t.Logf("    ✓ 读取AnalogInput:0 = %v", rp.Object.Properties[0].Data)
	}

	t.Logf("\n=== 快速扫描完成 ===")
}
