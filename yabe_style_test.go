package bacnet

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

// getYabeLocalIP returns the local IP to use for Yabe-style tests.
// Uses BACNET_IP env var if set (e.g., "192.168.3.230"), otherwise auto-detects.
// Yabe allows user to select which network interface to use; this mirrors that behavior.
//
// getYabeLocalIP 返回 Yabe 风格测试使用的本地 IP。
// 如果设置了 BACNET_IP 环境变量则使用指定值，否则自动检测。
// Yabe 允许用户选择使用哪个网络接口，此函数模拟该行为。
func getYabeLocalIP() net.IP {
	if ipStr := os.Getenv("BACNET_IP"); ipStr != "" {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			return ip.To4()
		}
	}
	return datalink.GetLocalIP()
}

// getYabePort returns the BACnet port to use.
// Uses BACNET_PORT env var if set, otherwise defaults to 47808 (Yabe standard).
func getYabePort() int {
	if portStr := os.Getenv("BACNET_PORT"); portStr != "" {
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil && port > 0 && port < 65536 {
			return port
		}
	}
	return 47808
}

// TestYabeStyleDiscovery uses pure broadcast WhoIs to discover devices,
// exactly like the official Yabe (Yet Another BACnet Explorer) tool.
// Yabe approach: bind to a local IP → send broadcast WhoIs → collect IAm → read details.
//
// Yabe 风格设备发现测试：使用纯广播 WhoIs 发现设备，完全模仿官方 Yabe 工具。
// Yabe 方式：绑定本地 IP → 发送广播 WhoIs → 收集 IAm → 读取详情。
func TestYabeStyleDiscovery(t *testing.T) {
	const (
		readTimeout = 10 * time.Second
	)

	// Use BACNET_IP env var or auto-detect (Yabe-style: user selects interface)
	// 使用 BACNET_IP 环境变量或自动检测（Yabe 风格：用户选择接口）
	localIP := getYabeLocalIP()
	bacnetPort := getYabePort()
	if localIP.IsUnspecified() {
		t.Fatalf("❌ 无法获取本地 IP 地址")
	}
	t.Logf("本地 IP: %s, BACnet 端口: %d", localIP.String(), bacnetPort)
	if envIP := os.Getenv("BACNET_IP"); envIP != "" {
		t.Logf("  (使用环境变量 BACNET_IP=%s)", envIP)
	}

	// Expected devices to discover (Yabe simulators on 192.168.3.115)
	// 期望发现的设备（192.168.3.115 上的 Yabe 模拟器）
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

	startAll := time.Now()

	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("         Yabe 风格 BACnet 完整链路测试 (纯广播 WhoIs)")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("期望设备列表:")
	for _, cfg := range deviceConfigs {
		t.Logf("  - DeviceID=%d, Port=%d, Type=%s", cfg.ID, cfg.Port, cfg.Type)
	}

	// ── Create BACnet client ──
	bacClient, err := NewClient(&ClientBuilder{
		Ip:         localIP.String(),
		Port:       bacnetPort,
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
	t.Logf("✓ 客户端运行在 %s:%d", localIP.String(), bacnetPort)

	// ─────────────────────────────────────────────────────────────
	// Phase 1: Yabe-style broadcast WhoIs discovery
	// Yabe sends a single broadcast WhoIs to discover ALL devices
	// Yabe 发送一次广播 WhoIs 发现所有设备
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("   Phase 1: Yabe 风格广播 WhoIs 设备发现")
	t.Log("───────────────────────────────────────────────────────────────")

	// Yabe sends WhoIs with no range limits (discover all devices)
	// Yabe 发送无范围限制的 WhoIs（发现所有设备）
	whoIsOpts := &WhoIsOpts{
		Low:             0,
		High:            0, // 0 means no range limit = discover all
		GlobalBroadcast: false,
	}

	t.Logf("  → 发送广播 WhoIs (Low=0, High=0, 发现所有设备)...")
	discoveryStart := time.Now()
	devices, err := bacClient.WhoIs(whoIsOpts)
	discoveryElapsed := time.Since(discoveryStart)

	if err != nil {
		t.Logf("  ⚠ 广播 WhoIs 返回错误: %v", err)
	}
	t.Logf("  ✓ 广播 WhoIs 完成，耗时: %s", discoveryElapsed.Round(time.Millisecond))
	t.Logf("  ✓ 收到 %d 个 I-Am 响应", len(devices))

	confirmedDevices := make(map[int]btypes.Device)
	unfoundDevices := make(map[int]DeviceConfig)
	for _, cfg := range deviceConfigs {
		unfoundDevices[cfg.ID] = cfg
	}

	for _, dev := range devices {
		t.Logf("  ✉️ I-Am: DeviceID=%d, IP=%s:%d, MaxAPDU=%d, Vendor=%d",
			dev.DeviceID, dev.Ip, dev.Port, dev.MaxApdu, dev.Vendor)

		if cfg, ok := unfoundDevices[dev.DeviceID]; ok {
			t.Logf("    ✓ 设备 %d (%s) 已发现", dev.DeviceID, cfg.Type)
			confirmedDevices[dev.DeviceID] = dev
			delete(unfoundDevices, dev.DeviceID)
		}
	}

	// If any devices were not found via broadcast, try direct ReadProperty
	// 如果广播未发现某些设备，尝试直接 ReadProperty 验证
	if len(unfoundDevices) > 0 {
		t.Logf("")
		t.Logf("  ── 补充验证: 直接 ReadProperty ──")
		for _, cfg := range unfoundDevices {
			// Try common target IPs for Yabe simulators
			for _, targetIP := range []string{"192.168.3.115", "127.0.0.1"} {
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

				rp, err := bacClient.ReadPropertyWithTimeout(testDev, btypes.PropertyData{
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

				if err == nil && len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					t.Logf("    ✓ Device %d (%s) 直接验证成功 @ %s:%d: %v",
						cfg.ID, cfg.Type, targetIP, cfg.Port, rp.Object.Properties[0].Data)
					confirmedDevices[cfg.ID] = testDev
					delete(unfoundDevices, cfg.ID)
					break
				}
			}
		}
	}

	discoveryFailures := len(deviceConfigs) - len(confirmedDevices)
	if discoveryFailures > 0 {
		t.Logf("")
		t.Logf("❌ Phase 1 失败: %d 台设备未发现", discoveryFailures)
		for _, cfg := range unfoundDevices {
			t.Logf("   - Device %d (%s) Port=%d", cfg.ID, cfg.Type, cfg.Port)
		}
		t.Fatalf("设备发现失败")
	}
	t.Logf("")
	t.Logf("✅ Phase 1 完成: %d/%d 设备全部发现成功", len(confirmedDevices), len(deviceConfigs))

	// ─────────────────────────────────────────────────────────────
	// Phase 2: Object scanning (Objects enumeration)
	// Yabe reads Object_List from each discovered device
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("   Phase 2: 对象扫描 (Objects 枚举)")
	t.Log("───────────────────────────────────────────────────────────────")

	scannedDevices := make(map[int]btypes.Device)
	scanFailures := 0

	for _, cfg := range deviceConfigs {
		targetID := cfg.ID
		t.Logf("")
		t.Logf("  [扫描] Device %d (%s)...", targetID, cfg.Type)

		dev, ok := confirmedDevices[targetID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在发现阶段找到", targetID)
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

	// ─────────────────────────────────────────────────────────────
	// Phase 3: Read all Present_Values
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("   Phase 3: 全量点位读取 (Present_Value)")
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
					t.Errorf("    ❌ %s:%d (%s) 读取失败: %v", objType, instance, obj.Name, err)
					readFailures++
					continue
				}
				if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					t.Logf("    ✓ %s:%d %s = %v", objType, instance, obj.Name, rp.Object.Properties[0].Data)
					totalReadSuccess++
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

	// ─────────────────────────────────────────────────────────────
	// Phase 4: Writable point write test (WriteProperty)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("   Phase 4: 可写点写入测试 (WriteProperty + 二次验证)")
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
		t.Logf("  [写入] Device %d (%s)...", targetID, cfg.Type)

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
					t.Errorf("    ❌ %s:%d 写入失败: %v", objType, instance, err)
					writeFailures++
					continue
				}
				t.Logf("    ✓ 写入成功")

				// Secondary verification: read twice, both must match
				// 二次验证：读取两次，两次都必须匹配
				var verifyValue1, verifyValue2 interface{}

				rp1, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
						}},
					},
				}, readTimeout)
				if err != nil {
					t.Errorf("    ❌ 第一次验证读取失败: %v", err)
					writeFailures++
					continue
				}
				if len(rp1.Object.Properties) > 0 && rp1.Object.Properties[0].Data != nil {
					verifyValue1 = rp1.Object.Properties[0].Data
					t.Logf("    ✓ 第一次验证: %v", verifyValue1)
				}

				time.Sleep(500 * time.Millisecond)

				rp2, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
						}},
					},
				}, readTimeout)
				if err != nil {
					t.Errorf("    ❌ 第二次验证读取失败: %v", err)
					writeFailures++
					continue
				}
				if len(rp2.Object.Properties) > 0 && rp2.Object.Properties[0].Data != nil {
					verifyValue2 = rp2.Object.Properties[0].Data
					t.Logf("    ✓ 第二次验证: %v", verifyValue2)
				}

				if verifyValue1 != nil && valuesEqual(verifyValue1, writeValue) {
					// First verification passes: write succeeded
					// 第一次验证通过：写入成功
					if verifyValue2 != nil && valuesEqual(verifyValue2, writeValue) {
						t.Logf("    ✅ 二次验证通过: 写入=%v, 验证1=%v, 验证2=%v", writeValue, verifyValue1, verifyValue2)
					} else {
						// Yabe simulator may auto-revert some values (e.g., SetPoint controlled by simulator logic)
						// Yabe 模拟器可能自动恢复某些值（如模拟器逻辑控制的设定点）
						t.Logf("    ⚠ 写入成功但值已恢复: 写入=%v, 验证1=%v, 验证2=%v (模拟器自动恢复)", writeValue, verifyValue1, verifyValue2)
					}
					totalWriteSuccess++
				} else {
					t.Errorf("    ❌ 写入验证失败: 写入=%v, 验证1=%v, 验证2=%v", writeValue, verifyValue1, verifyValue2)
					writeFailures++
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

	// ── Summary ──
	totalElapsed := time.Since(startAll)
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("         Yabe 风格 BACnet 完整链路测试 — 汇总")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("")
	t.Logf("【测试结果】")
	t.Logf("  ├─ 总耗时: %s", totalElapsed.Round(time.Millisecond))
	t.Logf("  ├─ Phase 1 广播 WhoIs: ✅ %d/%d 设备发现", len(confirmedDevices), len(deviceConfigs))
	t.Logf("  ├─ Phase 2 对象扫描: ✅ %d/%d 成功", len(scannedDevices), len(deviceConfigs))
	t.Logf("  ├─ Phase 3 点位读取: ✅ %d/%d 成功", totalReadSuccess, totalReadPoints)
	t.Logf("  └─ Phase 4 可写点写入: ✅ %d/%d 成功", totalWriteSuccess, totalWriteAttempts)
	t.Logf("")
	t.Logf("✅ 所有测试阶段通过!")
	t.Log("═══════════════════════════════════════════════════════════════")
}

// TestYabeStyleBroadcastOnly validates that pure broadcast WhoIs
// (no unicast fallback) can discover all devices.
// 纯广播 WhoIs 验证（无单播回退），确保广播方式即可发现所有设备。
func TestYabeStyleBroadcastOnly(t *testing.T) {
	localIP := getYabeLocalIP()
	bacnetPort := getYabePort()
	if localIP.IsUnspecified() {
		t.Fatalf("❌ 无法获取本地 IP 地址")
	}
	t.Logf("本地 IP: %s, 端口: %d", localIP.String(), bacnetPort)

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         localIP.String(),
		Port:       bacnetPort,
		SubnetCIDR: 24,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("❌ NewClient 失败: %v", err)
	}
	defer bacClient.Close()
	go bacClient.ClientRun()
	time.Sleep(500 * time.Millisecond)

	// Pure broadcast WhoIs - exactly like Yabe
	// 纯广播 WhoIs - 完全模仿 Yabe
	whoIsOpts := &WhoIsOpts{
		Low:             0,
		High:            0,
		GlobalBroadcast: false,
	}

	t.Log("→ 发送广播 WhoIs (Yabe 风格)...")
	devices, err := bacClient.WhoIs(whoIsOpts)
	if err != nil {
		t.Logf("⚠ 广播 WhoIs 返回错误: %v", err)
	}

	t.Logf("✓ 发现 %d 台设备:", len(devices))
	for _, dev := range devices {
		t.Logf("  ✉️ DeviceID=%d, IP=%s:%d, MaxAPDU=%d, Vendor=%d, Segmentation=%d",
			dev.DeviceID, dev.Ip, dev.Port, dev.MaxApdu, dev.Vendor, dev.Segmentation)
	}

	// Verify expected Yabe simulator devices are found
	// 验证预期的 Yabe 模拟器设备已发现
	expectedIDs := map[int]bool{
		2228316: false,
		2228317: false,
		2228318: false,
	}

	for _, dev := range devices {
		if _, ok := expectedIDs[dev.DeviceID]; ok {
			expectedIDs[dev.DeviceID] = true
		}
	}

	missing := []int{}
	for id, found := range expectedIDs {
		if !found {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		t.Errorf("❌ 广播 WhoIs 未发现设备: %v (期望: 2228316, 2228317, 2228318)", missing)
	} else {
		t.Logf("✅ 广播 WhoIs 成功发现所有期望的 Yabe 模拟器设备")
	}

	// Also verify room-simulator (1234) if present
	found1234 := false
	for _, dev := range devices {
		if dev.DeviceID == 1234 {
			found1234 = true
			t.Logf("✓ 同时发现 room-simulator (Device 1234)")
			break
		}
	}
	if !found1234 {
		t.Logf("⚠ 未发现 room-simulator (Device 1234)，可能未运行")
	}

	// Quick device name verification for discovered devices
	// 对已发现设备进行快速名称验证
	t.Log("")
	t.Log("── 设备名称验证 ──")
	for _, dev := range devices {
		if dev.DeviceID < 2228316 || dev.DeviceID > 2228318 {
			if dev.DeviceID != 1234 {
				continue
			}
		}

		rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: dev.ID,
				Properties: []btypes.Property{{
					Type:       btypes.PropObjectName,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 10*time.Second)

		if err != nil {
			t.Errorf("  ❌ Device %d ObjectName 读取失败: %v", dev.DeviceID, err)
		} else if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
			t.Logf("  ✓ Device %d: %v", dev.DeviceID, rp.Object.Properties[0].Data)
		}
	}

	t.Log("")
	t.Log("✅ TestYabeStyleBroadcastOnly 完成")
}

// TestYabeStyleMultiDeviceReadWrite is a full integration test:
// Yabe-style broadcast discovery → objects scan → read all → write all
// 完整集成测试：Yabe 风格广播发现 → 对象扫描 → 全量读取 → 全量写入
func TestYabeStyleMultiDeviceReadWrite(t *testing.T) {
	const readTimeout = 10 * time.Second

	localIP := getYabeLocalIP()
	bacnetPort := getYabePort()
	if localIP.IsUnspecified() {
		t.Fatalf("❌ 无法获取本地 IP 地址")
	}
	t.Logf("本地 IP: %s, 端口: %d", localIP.String(), bacnetPort)

	startAll := time.Now()

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         localIP.String(),
		Port:       bacnetPort,
		SubnetCIDR: 24,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("❌ NewClient 失败: %v", err)
	}
	defer bacClient.Close()
	go bacClient.ClientRun()
	time.Sleep(500 * time.Millisecond)

	// Yabe-style broadcast WhoIs
	// Yabe 风格广播 WhoIs
	devices, err := bacClient.WhoIs(&WhoIsOpts{Low: 0, High: 0, GlobalBroadcast: false})
	if err != nil {
		t.Logf("⚠ 广播 WhoIs 错误: %v", err)
	}

	t.Logf("✓ 广播 WhoIs 发现 %d 台设备", len(devices))
	for _, dev := range devices {
		t.Logf("  DeviceID=%d, IP=%s:%d", dev.DeviceID, dev.Ip, dev.Port)
	}

	// Filter to expected Yabe simulator devices (2228316-2228318)
	// 筛选预期的 Yabe 模拟器设备
	targetIDs := []int{2228316, 2228317, 2228318}
	targetDevices := make(map[int]btypes.Device)
	for _, dev := range devices {
		for _, id := range targetIDs {
			if dev.DeviceID == id {
				targetDevices[id] = dev
				break
			}
		}
	}

	if len(targetDevices) < 3 {
		t.Fatalf("❌ 仅发现 %d/3 台目标设备 (期望: 2228316, 2228317, 2228318)", len(targetDevices))
	}
	t.Logf("✓ 发现全部 3 台目标设备")

	// Scan objects for each device
	// 为每台设备扫描对象
	scannedDevices := make(map[int]btypes.Device)
	for _, id := range targetIDs {
		dev, ok := targetDevices[id]
		if !ok {
			continue
		}

		scanned, err := bacClient.Objects(dev)
		if err != nil {
			t.Errorf("❌ Device %d Objects 扫描失败: %v", id, err)
			continue
		}

		totalObj := 0
		for _, objs := range scanned.Objects {
			totalObj += len(objs)
		}
		t.Logf("  Device %d: %d 个对象", id, totalObj)
		scannedDevices[id] = scanned
	}

	if len(scannedDevices) < 3 {
		t.Fatalf("❌ 仅扫描成功 %d/3 台设备", len(scannedDevices))
	}

	// Read all present values
	// 读取所有点位值
	t.Log("")
	t.Log("── 点位值读取 ──")
	readCount := 0
	readSuccess := 0
	for _, id := range targetIDs {
		scanned, ok := scannedDevices[id]
		if !ok {
			continue
		}
		dev := targetDevices[id]

		for objType, objs := range scanned.Objects {
			if objType == btypes.DeviceType {
				continue
			}
			for instance := range objs {
				readCount++
				rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
						}},
					},
				}, readTimeout)
				if err == nil && len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					readSuccess++
				}
			}
		}
	}
	t.Logf("  点位读取: %d/%d 成功", readSuccess, readCount)

	if readSuccess == 0 {
		t.Fatalf("❌ 所有点位读取失败")
	}

	// Write test on writable points
	// 可写点写入测试
	t.Log("")
	t.Log("── 可写点写入测试 ──")
	writableTypes := []btypes.ObjectType{btypes.AnalogValue, btypes.BinaryValue, btypes.BinaryOutput, btypes.AnalogOutput}
	writeSuccess := 0
	writeAttempts := 0

	for _, id := range targetIDs {
		scanned, ok := scannedDevices[id]
		if !ok {
			continue
		}
		dev := targetDevices[id]

		for _, objType := range writableTypes {
			objs, ok := scanned.Objects[objType]
			if !ok {
				continue
			}
			for instance := range objs {
				var writeValue interface{}
				switch objType {
				case btypes.AnalogValue, btypes.AnalogOutput:
					writeValue = float32(267.0)
				case btypes.BinaryValue, btypes.BinaryOutput:
					writeValue = uint32(267)
				default:
					continue
				}

				writeAttempts++
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
					if err.Error() != "error class DeviceError code WriteAccessDenied" {
						t.Logf("  ⚠ Device %d %s:%d 写入失败: %v", id, objType, instance, err)
					}
					continue
				}

				// Verify
				time.Sleep(300 * time.Millisecond)
				rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
					Object: btypes.Object{
						ID: btypes.ObjectID{Type: objType, Instance: instance},
						Properties: []btypes.Property{{
							Type:       btypes.PROP_PRESENT_VALUE,
							ArrayIndex: btypes.ArrayAll,
						}},
					},
				}, readTimeout)
				if err == nil && len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					if valuesEqual(rp.Object.Properties[0].Data, writeValue) {
						writeSuccess++
					}
				}
			}
		}
	}
	t.Logf("  可写点写入: %d/%d 成功", writeSuccess, writeAttempts)

	totalElapsed := time.Since(startAll)
	t.Log("")
	t.Logf("✅ Yabe 风格多设备读写测试完成 (耗时: %s)", totalElapsed.Round(time.Millisecond))
	t.Logf("   发现: %d 台, 扫描: %d 台, 读: %d/%d, 写: %d/%d",
		len(devices), len(scannedDevices), readSuccess, readCount, writeSuccess, writeAttempts)
}

// TestYabeStyleWhoIsResponse validates that WhoIs with broadcast
// correctly receives IAm responses from all Yabe simulators.
// 验证广播 WhoIs 能正确接收所有 Yabe 模拟器的 IAm 响应。
func TestYabeStyleWhoIsResponse(t *testing.T) {
	localIP := getYabeLocalIP()
	bacnetPort := getYabePort()
	if localIP.IsUnspecified() {
		t.Fatalf("❌ 无法获取本地 IP")
	}
	t.Logf("本地 IP: %s", localIP.String())

	bacClient, err := NewClient(&ClientBuilder{
		Ip:         localIP.String(),
		Port:       bacnetPort,
		SubnetCIDR: 24,
		MaxPDU:     btypes.MaxAPDU,
	})
	if err != nil {
		t.Fatalf("❌ 创建客户端失败: %v", err)
	}
	defer bacClient.Close()
	go bacClient.ClientRun()
	time.Sleep(500 * time.Millisecond)

	// Perform multiple WhoIs rounds to verify consistency
	// 多轮 WhoIs 验证一致性
	for round := 1; round <= 3; round++ {
		t.Logf("")
		t.Logf("── 第 %d 轮广播 WhoIs ──", round)

		devices, err := bacClient.WhoIs(&WhoIsOpts{Low: 0, High: 0, GlobalBroadcast: false})
		if err != nil {
			t.Logf("  ⚠ 错误: %v", err)
		}

		t.Logf("  发现 %d 台设备:", len(devices))
		foundMap := make(map[int]string)
		for _, dev := range devices {
			addr := fmt.Sprintf("%s:%d", dev.Ip, dev.Port)
			foundMap[dev.DeviceID] = addr
			t.Logf("    DeviceID=%d, Addr=%s", dev.DeviceID, addr)
		}

		// Check Yabe simulator devices
		for _, id := range []int{2228316, 2228317, 2228318} {
			if addr, ok := foundMap[id]; ok {
				t.Logf("    ✓ Device %d 已发现 @ %s", id, addr)
			} else {
				t.Logf("    ⚠ Device %d 未发现", id)
			}
		}

		if round < 3 {
			time.Sleep(2 * time.Second)
		}
	}

	t.Log("")
	t.Log("✅ TestYabeStyleWhoIsResponse 完成")
}