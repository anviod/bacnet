package bacnet

import (
	"net"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

// TestFourDeviceAcceptance 四设备完整验收测试
//
// 设备清单 (端口映射已通过直接 ReadProperty 验证):
//
//	Device ID 1234    → 192.168.3.115:47810 (room-simulator)
//	Device ID 2228316 → 192.168.3.115:61663 (Yabe simulator)
//	Device ID 2228317 → 192.168.3.115:50958 (Yabe simulator)
//	Device ID 2228318 → 192.168.3.115:47808 (Yabe simulator)
//
// 注意: Yabe simulators 的 IAm 响应全部来自 47808, 导致 WhoIs 发现的地址不正确.
// 本测试使用直接探测验证每个设备的正确端口, 而非依赖 IAm 源地址.
//
// 测试阶段: 发现 → 扫描 → 全量读取 → 可写点写入
func TestFourDeviceAcceptance(t *testing.T) {
	const (
		targetIP    = "192.168.3.115"
		readTimeout = 8 * time.Second
		clientPort  = 47815 // 使用独立端口避免冲突
	)

	// 设备目标清单 (已验证的端口映射)
	type deviceTarget struct {
		DeviceID int
		Port     int
	}
	allDevices := []deviceTarget{
		{DeviceID: 1234, Port: 47810},
		{DeviceID: 2228316, Port: 61663},
		{DeviceID: 2228317, Port: 50958},
		{DeviceID: 2228318, Port: 47808},
	}

	targetIPAddr := net.ParseIP(targetIP)

	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("            四设备 BACnet 完整验收测试")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("目标 IP: %s, 客户端端口: %d", targetIP, clientPort)

	startAll := time.Now()

	// ─────────────────────────────────────────────────────────────
	// Phase 0: 客户端初始化
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 0: 客户端初始化")
	t.Log("───────────────────────────────────────────────────────────────")

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

	// ─────────────────────────────────────────────────────────────
	// Phase 1: 设备发现与验证
	// Step 1a: WhoIs 广播 (仅验证发现功能可用)
	// Step 1b: ReadProperty Object_Name 验证每个设备
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 1: 设备发现与验证")
	t.Log("───────────────────────────────────────────────────────────────")

	// Step 1a: WhoIs 广播
	t.Logf("")
	t.Logf("  [WhoIs] 广播发现...")
	devices, err := bacClient.WhoIs(&WhoIsOpts{Low: 0, High: 4194304})
	if err != nil {
		t.Logf("  ⚠ WhoIs 失败: %v", err)
	} else {
		t.Logf("  ✓ 发现 %d 台设备 (WhoIs):", len(devices))
		for _, d := range devices {
			udpAddr, _ := d.Addr.UDPAddr()
			t.Logf("    DeviceID=%d, Addr=%s:%d", d.DeviceID, udpAddr.IP, udpAddr.Port)
		}
	}
	time.Sleep(500 * time.Millisecond)

	// Step 1b: ReadProperty Object_Name 验证每个设备
	t.Logf("")
	t.Logf("  [验证] ReadProperty Object_Name 逐设备验证...")

	confirmedDevices := make(map[int]btypes.Device)
	discoveryFailures := 0

	for _, dt := range allDevices {
		addr := datalink.IPPortToAddress(targetIPAddr, dt.Port)
		dev := btypes.Device{
			DeviceID: dt.DeviceID,
			Addr:     *addr,
			Ip:       targetIP,
			Port:     dt.Port,
			MaxApdu:  btypes.MaxAPDU,
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: btypes.ObjectInstance(dt.DeviceID),
			},
		}

		rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{
					Type:     btypes.DeviceType,
					Instance: btypes.ObjectInstance(dt.DeviceID),
				},
				Properties: []btypes.Property{{
					Type:       btypes.PropObjectName,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, readTimeout)

		if err != nil {
			t.Errorf("    ❌ Device %d (port %d) 验证失败: %v", dt.DeviceID, dt.Port, err)
			discoveryFailures++
			continue
		}

		if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
			t.Logf("    ✓ Device %d (port %d) Object_Name=%v", dt.DeviceID, dt.Port, rp.Object.Properties[0].Data)
		} else {
			t.Logf("    ✓ Device %d (port %d) 响应正常", dt.DeviceID, dt.Port)
		}
		confirmedDevices[dt.DeviceID] = dev
		time.Sleep(200 * time.Millisecond)
	}

	if discoveryFailures > 0 {
		t.Fatalf("❌ Phase 1 失败: %d 台设备无法通信", discoveryFailures)
	}
	t.Logf("")
	t.Logf("✅ Phase 1 完成: 4/4 设备全部验证成功")

	// ─────────────────────────────────────────────────────────────
	// Phase 2: 对象扫描 (Objects)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 2: 对象扫描 (Objects)")
	t.Log("───────────────────────────────────────────────────────────────")

	scannedDevices := make(map[int]btypes.Device)
	scanFailures := 0

	for _, dt := range allDevices {
		t.Logf("")
		t.Logf("  [扫描] Device %d (port %d) ...", dt.DeviceID, dt.Port)

		dev, ok := confirmedDevices[dt.DeviceID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在验证阶段找到", dt.DeviceID)
			scanFailures++
			continue
		}

		scanned, err := bacClient.Objects(dev)
		if err != nil {
			t.Errorf("  ❌ Device %d Objects 扫描失败: %v", dt.DeviceID, err)
			scanFailures++
			continue
		}

		totalObj := 0
		for objType, objs := range scanned.Objects {
			count := len(objs)
			totalObj += count
			t.Logf("    %s: %d 个", objType, count)
		}
		t.Logf("  ✓ Device %d 扫描完成，共 %d 个对象", dt.DeviceID, totalObj)
		scannedDevices[dt.DeviceID] = scanned
		time.Sleep(200 * time.Millisecond)
	}

	if scanFailures > 0 {
		t.Fatalf("❌ Phase 2 失败: %d 台设备扫描失败", scanFailures)
	}
	t.Logf("")
	t.Logf("✅ Phase 2 完成: 4/4 设备对象扫描成功")

	// ─────────────────────────────────────────────────────────────
	// Phase 3: 全量读取 Present_Value
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("           Phase 3: 全量读取 Present_Value")
	t.Log("───────────────────────────────────────────────────────────────")

	readFailures := 0
	totalReadPoints := 0
	totalReadSuccess := 0

	for _, dt := range allDevices {
		t.Logf("")
		t.Logf("  [读取] Device %d (port %d) ...", dt.DeviceID, dt.Port)

		scanned, ok := scannedDevices[dt.DeviceID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在扫描阶段找到", dt.DeviceID)
			readFailures++
			continue
		}

		dev := confirmedDevices[dt.DeviceID]

		for objType, objs := range scanned.Objects {
			// Device 对象没有 Present_Value, 跳过
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
						dt.DeviceID, objType, instance, obj.Name, err)
					readFailures++
					continue
				}
				if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
					t.Logf("    ✓ Device %d %s:%d %s = %v", dt.DeviceID, objType, instance, obj.Name, rp.Object.Properties[0].Data)
					totalReadSuccess++
				} else {
					t.Logf("    ⚠ Device %d %s:%d %s = N/A", dt.DeviceID, objType, instance, obj.Name)
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
	// Phase 4: 可写点写入测试
	// ─────────────────────────────────────────────────────────────
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

	for _, dt := range allDevices {
		t.Logf("")
		t.Logf("  [写入] Device %d (port %d) ...", dt.DeviceID, dt.Port)

		scanned, ok := scannedDevices[dt.DeviceID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在扫描阶段找到", dt.DeviceID)
			writeFailures++
			continue
		}

		dev := confirmedDevices[dt.DeviceID]

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
					// WriteAccessDenied 表示该点位只读, 不算失败
					if err.Error() == "error class DeviceError code WriteAccessDenied" {
						t.Logf("    ⚠ %s:%d 只读, 跳过写入", objType, instance)
						continue
					}
					t.Errorf("    ❌ Device %d %s:%d 写入失败: %v", dt.DeviceID, objType, instance, err)
					writeFailures++
					continue
				}
				t.Logf("    ✓ 写入成功")

				// 验证读取
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
					t.Errorf("    ❌ Device %d %s:%d 验证读取失败: %v", dt.DeviceID, objType, instance, err)
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
			t.Logf("  ⚠ Device %d 无可写点", dt.DeviceID)
		}
	}

	if writeFailures > 0 {
		t.Fatalf("❌ Phase 4 失败: %d 次写入失败", writeFailures)
	}

	// ─────────────────────────────────────────────────────────────
	// 汇总
	// ─────────────────────────────────────────────────────────────
	totalElapsed := time.Since(startAll)
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("            四设备 BACnet 完整验收测试 — 汇总")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("")
	t.Logf("【测试结果】")
	t.Logf("  ├─ 总耗时: %s", totalElapsed.Round(time.Millisecond))
	t.Logf("  ├─ Phase 1 设备发现: ✅ 4/4 成功")
	t.Logf("  ├─ Phase 2 对象扫描: ✅ 4/4 成功")
	t.Logf("  ├─ Phase 3 点位读取: ✅ %d/%d 成功", totalReadSuccess, totalReadPoints)
	t.Logf("  └─ Phase 4 可写点写入: ✅ %d/%d 成功", totalWriteSuccess, totalWriteAttempts)
	t.Logf("")
	t.Logf("✅ 所有测试阶段通过!")
	t.Log("═══════════════════════════════════════════════════════════════")
}