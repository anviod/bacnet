package bacnet

import (
	"net"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

func TestFourDeviceAcceptance(t *testing.T) {
	const (
		targetIP      = "192.168.3.115"
		readTimeout   = 10 * time.Second
		clientPort    = 47815
		broadcastPort = 47808
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
	t.Log("            四设备 BACnet 完整验收测试")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("目标 IP: %s, 客户端端口: %d", targetIP, clientPort)
	t.Logf("预配置设备列表:")
	for _, cfg := range deviceConfigs {
		t.Logf("  - DeviceID=%d, Port=%d, Type=%s", cfg.ID, cfg.Port, cfg.Type)
	}

	startAll := time.Now()

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         targetIP,
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
	t.Log("              Phase 1: 设备发现与验证 (两步扫描)")
	t.Log("───────────────────────────────────────────────────────────────")

	confirmedDevices := make(map[int]btypes.Device)
	unfoundDevices := make(map[int]DeviceConfig)

	t.Logf("")
	t.Logf("  Step 1: 使用用户提供的 ID+IP+Port 进行直接验证...")

	for _, cfg := range deviceConfigs {
		t.Logf("")
		t.Logf("    ── Device %d (%s) 直接验证 ──", cfg.ID, cfg.Type)

		addr := datalink.IPPortToAddress(net.ParseIP(targetIP), cfg.Port)
		testDev := btypes.Device{
			DeviceID: cfg.ID,
			Addr:     *addr,
			Ip:       targetIP,
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
			t.Logf("      ├─ IP: %s", targetIP)
			t.Logf("      ├─ Port: %d", cfg.Port)
			t.Logf("      └─ Type: %s", cfg.Type)
			confirmedDevices[cfg.ID] = testDev
		} else {
			t.Logf("    ✓ Device %d 验证成功 (无名称)", cfg.ID)
			t.Logf("      ├─ IP: %s", targetIP)
			t.Logf("      ├─ Port: %d", cfg.Port)
			t.Logf("      └─ Type: %s", cfg.Type)
			confirmedDevices[cfg.ID] = testDev
		}
		time.Sleep(500 * time.Millisecond)
	}

	if len(unfoundDevices) > 0 {
		t.Logf("")
		t.Logf("  Step 2: 对未发现的设备使用广播方式扫描 (端口 %d)...", broadcastPort)

		broadcastAddr := datalink.IPPortToAddress(net.ParseIP(targetIP), broadcastPort)
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
				if cfg, ok := unfoundDevices[dev.DeviceID]; ok {
					t.Logf("    ✓ Device %d 通过广播发现", dev.DeviceID)
					t.Logf("      ├─ IP: %s", dev.Ip)
					t.Logf("      ├─ Port: %d", dev.Port)
					t.Logf("      └─ Type: %s", cfg.Type)
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
		t.Fatalf("❌ Phase 1 失败: %d 台设备无法通信", discoveryFailures)
	}
	t.Logf("")
	t.Logf("✅ Phase 1 完成: %d/%d 设备全部验证成功", len(confirmedDevices), len(deviceConfigs))

	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 2: 对象扫描 (Objects)")
	t.Log("───────────────────────────────────────────────────────────────")

	scannedDevices := make(map[int]btypes.Device)
	scanFailures := 0

	for _, cfg := range deviceConfigs {
		targetID := cfg.ID
		t.Logf("")
		t.Logf("  [扫描] Device %d ...", targetID)

		dev, ok := confirmedDevices[targetID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在验证阶段找到", targetID)
			scanFailures++
			continue
		}

		scanned, err := bacClient.Objects(dev)
		if err != nil {
			t.Errorf("  ❌ Device %d Objects 扫描失败: %v", targetID, err)
			scanFailures++
			continue
		}

		totalObj := 0
		for objType, objs := range scanned.Objects {
			count := len(objs)
			totalObj += count
			t.Logf("    %s: %d 个", objType, count)
		}
		t.Logf("  ✓ Device %d 扫描完成，共 %d 个对象", targetID, totalObj)
		scannedDevices[targetID] = scanned
		time.Sleep(200 * time.Millisecond)
	}

	if scanFailures > 0 {
		t.Fatalf("❌ Phase 2 失败: %d 台设备扫描失败", scanFailures)
	}
	t.Logf("")
	t.Logf("✅ Phase 2 完成: %d/%d 设备对象扫描成功", len(scannedDevices), len(deviceConfigs))

	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("           Phase 3: 全量读取 Present_Value")
	t.Log("───────────────────────────────────────────────────────────────")

	readFailures := 0
	totalReadPoints := 0
	totalReadSuccess := 0

	for _, cfg := range deviceConfigs {
		targetID := cfg.ID
		t.Logf("")
		t.Logf("  [读取] Device %d ...", targetID)

		scanned, ok := scannedDevices[targetID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在扫描阶段找到", targetID)
			readFailures++
			continue
		}

		dev := confirmedDevices[targetID]

		for objType, objs := range scanned.Objects {
			if objType == btypes.DeviceType {
				continue
			}
			for instance, obj := range objs {
				totalReadPoints++

				rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
						}},
					},
				}, readTimeout)

				if err != nil {
					t.Errorf("    ❌ Device %d %s:%d (%s) 读取失败: %v",
						targetID, objType, instance, obj.Name, err)
					readFailures++
					continue
				}
				if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					t.Logf("    ✓ Device %d %s:%d %s = %v", targetID, objType, instance, obj.Name, rp.Object.Properties[0].Data)
					totalReadSuccess++
				} else {
					t.Logf("    ⚠ Device %d %s:%d %s = N/A", targetID, objType, instance, obj.Name)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	if readFailures > 0 {
		t.Fatalf("❌ Phase 3 失败: %d 个点位读取失败", readFailures)
	}
	t.Logf("")
	t.Logf("✅ Phase 3 完成: %d/%d 点位读取成功", totalReadSuccess, totalReadPoints)

	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("           Phase 4: 可写点写入测试 (WriteProperty)")
	t.Log("───────────────────────────────────────────────────────────────")

	writableTypes := []btypes.ObjectType{
		btypes.AnalogValue,
		btypes.BinaryValue,
		btypes.BinaryOutput,
		btypes.AnalogOutput,
	}

	writeFailures := 0
	totalWriteAttempts := 0
	totalWriteSuccess := 0

	for _, cfg := range deviceConfigs {
		targetID := cfg.ID
		t.Logf("")
		t.Logf("  [写入] Device %d ...", targetID)

		scanned, ok := scannedDevices[targetID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在扫描阶段找到", targetID)
			writeFailures++
			continue
		}

		dev := confirmedDevices[targetID]

		deviceHasWritable := false
		for _, objType := range writableTypes {
			objs, ok := scanned.Objects[objType]
			if !ok || len(objs) == 0 {
				continue
			}
			deviceHasWritable = true

			for instance, obj := range objs {
				var writeValue interface{}
				switch objType {
				case btypes.AnalogValue, btypes.AnalogOutput:
					writeValue = float32(267.0)
				case btypes.BinaryValue, btypes.BinaryOutput:
					writeValue = uint32(267)
				default:
					continue
				}

				totalWriteAttempts++
				t.Logf("    写入 %s:%d %s = %v", objType, instance, obj.Name, writeValue)

				err := bacClient.WriteProperty(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
							Data:       writeValue,
							Priority:   btypes.Normal,
						}},
					},
				})

				if err != nil {
					if err.Error() == "error class DeviceError code WriteAccessDenied" {
						t.Logf("    ⚠ %s:%d 只读, 跳过写入", objType, instance)
						continue
					}
					t.Errorf("    ❌ Device %d %s:%d 写入失败: %v", targetID, objType, instance, err)
					writeFailures++
					continue
				}
				t.Logf("    ✓ 写入成功")

				rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
						}},
					},
				}, readTimeout)

				if err != nil {
					t.Errorf("    ❌ Device %d %s:%d 验证读取失败: %v", targetID, objType, instance, err)
					writeFailures++
					continue
				}
				if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					t.Logf("    ✓ 验证成功: %v", rp.Object.Properties[0].Data)
					totalWriteSuccess++
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		if !deviceHasWritable {
			t.Logf("  ⚠ Device %d 无可写点", targetID)
		}
	}

	if writeFailures > 0 {
		t.Fatalf("❌ Phase 4 失败: %d 次写入失败", writeFailures)
	}

	totalElapsed := time.Since(startAll)
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("            四设备 BACnet 完整验收测试 — 汇总")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("")
	t.Logf("【测试结果】")
	t.Logf("  ├─ 总耗时: %s", totalElapsed.Round(time.Millisecond))
	t.Logf("  ├─ Phase 1 设备发现: ✅ %d/%d 成功", len(confirmedDevices), len(deviceConfigs))
	t.Logf("  ├─ Phase 2 对象扫描: ✅ %d/%d 成功", len(scannedDevices), len(deviceConfigs))
	t.Logf("  ├─ Phase 3 点位读取: ✅ %d/%d 成功", totalReadSuccess, totalReadPoints)
	t.Logf("  └─ Phase 4 可写点写入: ✅ %d/%d 成功", totalWriteSuccess, totalWriteAttempts)
	t.Logf("")
	t.Logf("✅ 所有测试阶段通过!")
	t.Log("═══════════════════════════════════════════════════════════════")
}
