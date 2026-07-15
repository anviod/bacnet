package bacnet

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

type DeviceRecord struct {
	DeviceID     int
	Ip           string
	Port         int
	Name         string
	MaxApdu      uint32
	Segmentation btypes.Enumerated
	Vendor       uint32
	Status       string
}

type PointRecord struct {
	DeviceID     int
	ObjectType   btypes.ObjectType
	Instance     btypes.ObjectInstance
	Name         string
	PresentValue interface{}
	Unit         string
	Writable     bool
	Status       string
}

type PointValueSnapshot struct {
	DeviceID   int
	ObjectType btypes.ObjectType
	Instance   btypes.ObjectInstance
	Name       string
	Round1     interface{}
	Round2     interface{}
	Round3     interface{}
	HasChanged bool
}

func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch va := a.(type) {
	case float32:
		vb, ok := b.(float32)
		return ok && va == vb
	case float64:
		vb, ok := b.(float64)
		return ok && va == vb
	case uint32:
		vb, ok := b.(uint32)
		return ok && va == vb
	case int32:
		vb, ok := b.(int32)
		return ok && va == vb
	case []interface{}:
		vb, ok := b.([]interface{})
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !valuesEqual(va[i], vb[i]) {
				return false
			}
		}
		return true
	case []uint8:
		vb, ok := b.([]uint8)
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if va[i] != vb[i] {
				return false
			}
		}
		return true
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	default:
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
}

func TestBACnetDriverWorkflow(t *testing.T) {
	const (
		targetIP         = "192.168.3.115"
		readTimeout      = 10 * time.Second
		clientPort       = 47815
		broadcastPort    = 47808
		discoveryTimeout = 10 * time.Second
	)

	type DeviceConfig struct {
		ID   int
		Port int
		Type string
	}

	deviceConfigs := []DeviceConfig{
		{ID: 1234, Port: 47810, Type: "room-simulator"},
		{ID: 2228316, Port: 58494, Type: "Yabe simulator"},
		{ID: 2228317, Port: 64339, Type: "Yabe simulator"},
		{ID: 2228318, Port: 54304, Type: "Yabe simulator"},
		{ID: 2228319, Port: 58301, Type: "Yabe simulator"},
	}

	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("         BACnet 驱动完整链路测试")
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

	// ─────────────────────────────────────────────────────────────
	// Phase 1: 设备发现 (两步扫描)
	//   Step 1: 使用用户提供的 ID+IP+Port 进行直接验证
	//   Step 2: 对未发现的设备，使用广播方式（47808端口）扫描
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 1: 设备发现 (两步扫描)")
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
	t.Logf("✅ Phase 1 完成: %d/%d 设备全部发现成功", len(confirmedDevices), len(deviceConfigs))

	// ─────────────────────────────────────────────────────────────
	// Phase 2: 设备注册 (构建设备管理表)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 2: 设备注册 (构建设备管理表)")
	t.Log("───────────────────────────────────────────────────────────────")

	deviceRegistry := make(map[int]*DeviceRecord)

	for _, cfg := range deviceConfigs {
		targetID := cfg.ID
		dev, ok := confirmedDevices[targetID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在验证阶段找到", targetID)
			continue
		}

		rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: dev.ID,
				Properties: []btypes.Property{{
					Type:       btypes.PropObjectName,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, readTimeout)

		name := ""
		if err == nil && len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
			name = rp.Object.Properties[0].Data.(string)
		}

		record := &DeviceRecord{
			DeviceID:     dev.DeviceID,
			Ip:           dev.Ip,
			Port:         dev.Port,
			Name:         name,
			MaxApdu:      dev.MaxApdu,
			Segmentation: dev.Segmentation,
			Vendor:       dev.Vendor,
			Status:       "Online",
		}
		deviceRegistry[targetID] = record

		t.Logf("  ✓ Device %d 注册成功", targetID)
		t.Logf("    ├─ Name: %s", record.Name)
		t.Logf("    ├─ IP: %s:%d", record.Ip, record.Port)
		t.Logf("    ├─ MaxAPDU: %d", record.MaxApdu)
		t.Logf("    └─ Status: %s", record.Status)
	}

	t.Logf("")
	t.Logf("✅ Phase 2 完成: 成功注册 %d 台设备", len(deviceRegistry))

	// ─────────────────────────────────────────────────────────────
	// Phase 3: 点位扫描 (Objects 枚举全部对象)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 3: 点位扫描 (Objects 枚举)")
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
		t.Fatalf("❌ Phase 3 失败: %d 台设备扫描失败", scanFailures)
	}
	t.Logf("")
	t.Logf("✅ Phase 3 完成: %d/%d 设备对象扫描成功", len(scannedDevices), len(deviceConfigs))

	// ─────────────────────────────────────────────────────────────
	// Phase 4: 点位注册 (构建点位监控表)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 4: 点位注册 (构建监控点位表)")
	t.Log("───────────────────────────────────────────────────────────────")

	pointRegistry := make(map[string]*PointRecord)

	writableTypes := []btypes.ObjectType{
		btypes.AnalogValue,
		btypes.BinaryValue,
		btypes.BinaryOutput,
		btypes.AnalogOutput,
	}

	for _, cfg := range deviceConfigs {
		targetID := cfg.ID
		t.Logf("")
		t.Logf("  [注册点位] Device %d ...", targetID)

		scanned, ok := scannedDevices[targetID]
		if !ok {
			t.Errorf("  ❌ Device %d 未在扫描阶段找到", targetID)
			continue
		}

		pointCount := 0
		for objType, objs := range scanned.Objects {
			if objType == btypes.DeviceType {
				continue
			}

			for instance, obj := range objs {
				key := fmt.Sprintf("%d-%s-%d", targetID, objType, instance)

				isWritable := false
				for _, wt := range writableTypes {
					if wt == objType {
						isWritable = true
						break
					}
				}

				record := &PointRecord{
					DeviceID:     targetID,
					ObjectType:   objType,
					Instance:     instance,
					Name:         obj.Name,
					PresentValue: nil,
					Unit:         "",
					Writable:     isWritable,
					Status:       "Online",
				}
				pointRegistry[key] = record
				pointCount++

				writableMark := ""
				if isWritable {
					writableMark = "(可写)"
				}
				t.Logf("    ✓ 注册 %s:%d %s %s", objType, instance, obj.Name, writableMark)
			}
		}
		t.Logf("  ✓ Device %d 注册完成，共 %d 个点位", targetID, pointCount)
	}

	t.Logf("")
	t.Logf("✅ Phase 4 完成: 成功注册 %d 个监控点位", len(pointRegistry))

	// ─────────────────────────────────────────────────────────────
	// Phase 5: 实时数据轮询 (连续3轮读取，验证点位实时更新)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 5: 实时数据轮询 (3轮)")
	t.Log("───────────────────────────────────────────────────────────────")

	snapshots := make(map[string]*PointValueSnapshot)

	for round := 1; round <= 3; round++ {
		t.Logf("")
		t.Logf("  [轮次 %d] 读取全部点位值...", round)

		for key, point := range pointRegistry {
			dev, ok := confirmedDevices[point.DeviceID]
			if !ok {
				continue
			}

			rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: point.ObjectType, Instance: point.Instance},
					Properties: []btypes.Property{{
						Type:       btypes.PROP_PRESENT_VALUE,
						ArrayIndex: btypes.ArrayAll,
					}},
				},
			}, readTimeout)

			if err != nil {
				t.Logf("    ⚠ %s:%d 读取失败: %v", point.ObjectType, point.Instance, err)
				continue
			}

			var value interface{}
			if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
				value = rp.Object.Properties[0].Data
			}

			if snapshots[key] == nil {
				snapshots[key] = &PointValueSnapshot{
					DeviceID:   point.DeviceID,
					ObjectType: point.ObjectType,
					Instance:   point.Instance,
					Name:       point.Name,
				}
			}

			switch round {
			case 1:
				snapshots[key].Round1 = value
			case 2:
				snapshots[key].Round2 = value
			case 3:
				snapshots[key].Round3 = value
			}

			t.Logf("    ✓ %s:%d %s = %v", point.ObjectType, point.Instance, point.Name, value)
		}

		if round < 3 {
			t.Logf("  等待 2 秒进行下一轮读取...")
			time.Sleep(2 * time.Second)
		}
	}

	t.Logf("")
	t.Logf("  [实时性分析] 点位值变化对比:")
	t.Logf("  ┌───────────────────────────────────────────────────────────────")
	t.Logf("  │ Device │ 对象类型 │ Instance │ 点位名称              │ Round1 │ Round2 │ Round3 │ 变化")
	t.Logf("  ├───────────────────────────────────────────────────────────────")

	changedCount := 0
	for _, snapshot := range snapshots {
		hasChanged := false
		if snapshot.Round1 != nil && snapshot.Round2 != nil && !valuesEqual(snapshot.Round1, snapshot.Round2) {
			hasChanged = true
		}
		if snapshot.Round2 != nil && snapshot.Round3 != nil && !valuesEqual(snapshot.Round2, snapshot.Round3) {
			hasChanged = true
		}
		snapshot.HasChanged = hasChanged
		if hasChanged {
			changedCount++
		}

		round1Str := "-"
		if snapshot.Round1 != nil {
			round1Str = fmt.Sprintf("%v", snapshot.Round1)
		}
		round2Str := "-"
		if snapshot.Round2 != nil {
			round2Str = fmt.Sprintf("%v", snapshot.Round2)
		}
		round3Str := "-"
		if snapshot.Round3 != nil {
			round3Str = fmt.Sprintf("%v", snapshot.Round3)
		}

		changedMark := "❌"
		if hasChanged {
			changedMark = "✅"
		}

		t.Logf("  │ %6d │ %8s │ %8d │ %-20s │ %6s │ %6s │ %6s │ %s",
			snapshot.DeviceID,
			snapshot.ObjectType,
			snapshot.Instance,
			snapshot.Name,
			round1Str,
			round2Str,
			round3Str,
			changedMark,
		)
	}
	t.Logf("  └───────────────────────────────────────────────────────────────")

	t.Logf("")
	t.Logf("✅ Phase 5 完成: %d 个点位中有 %d 个点位值发生变化（实时更新验证）", len(snapshots), changedCount)

	// ─────────────────────────────────────────────────────────────
	// Phase 6: 可写点写入测试 (WriteProperty)
	// ─────────────────────────────────────────────────────────────
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 6: 可写点写入测试 (WriteProperty)")
	t.Log("───────────────────────────────────────────────────────────────")

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

				if objType == btypes.AnalogValue && instance == 2 && obj.Name == "Setpoint.2" {
					switch targetID {
					case 2228316:
						writeValue = float32(31.6)
					case 2228317:
						writeValue = float32(31.7)
					case 2228318:
						writeValue = float32(31.8)
					default:
						writeValue = float32(267.0)
					}
				} else {
					switch objType {
					case btypes.AnalogValue, btypes.AnalogOutput:
						writeValue = float32(267.0)
					case btypes.BinaryValue, btypes.BinaryOutput:
						writeValue = uint32(267)
					default:
						continue
					}
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
					t.Errorf("    ❌ Device %d %s:%d 第一次验证读取失败: %v", targetID, objType, instance, err)
					writeFailures++
					continue
				}
				if len(rp1.Object.Properties) > 0 && rp1.Object.Properties[0].Data != nil {
					verifyValue1 = rp1.Object.Properties[0].Data
					t.Logf("    ✓ 第一次验证: %v", verifyValue1)
				} else {
					t.Errorf("    ❌ Device %d %s:%d 第一次验证无数据", targetID, objType, instance)
					writeFailures++
					continue
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
					t.Errorf("    ❌ Device %d %s:%d 第二次验证读取失败: %v", targetID, objType, instance, err)
					writeFailures++
					continue
				}
				if len(rp2.Object.Properties) > 0 && rp2.Object.Properties[0].Data != nil {
					verifyValue2 = rp2.Object.Properties[0].Data
					t.Logf("    ✓ 第二次验证: %v", verifyValue2)
				} else {
					t.Errorf("    ❌ Device %d %s:%d 第二次验证无数据", targetID, objType, instance)
					writeFailures++
					continue
				}

				if verifyValue1 == verifyValue2 && valuesEqual(verifyValue1, writeValue) {
					t.Logf("    ✅ 二次验证通过: 写入值=%v, 两次读取值均为=%v", writeValue, verifyValue1)
					totalWriteSuccess++
				} else {
					t.Errorf("    ❌ 二次验证失败: 写入值=%v, 第一次验证=%v, 第二次验证=%v", writeValue, verifyValue1, verifyValue2)
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
		t.Fatalf("❌ Phase 6 失败: %d 次写入失败", writeFailures)
	}

	// ─────────────────────────────────────────────────────────────
	// 汇总
	// ─────────────────────────────────────────────────────────────
	totalElapsed := time.Since(startAll)
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("            BACnet 驱动完整链路测试 — 汇总")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("")
	t.Logf("【测试结果】")
	t.Logf("  ├─ 总耗时: %s", totalElapsed.Round(time.Millisecond))
	t.Logf("  ├─ Phase 1 设备发现: ✅ %d/%d 成功", len(confirmedDevices), len(deviceConfigs))
	t.Logf("  ├─ Phase 2 设备注册: ✅ %d 台设备", len(deviceRegistry))
	t.Logf("  ├─ Phase 3 点位扫描: ✅ %d/%d 成功", len(scannedDevices), len(deviceConfigs))
	t.Logf("  ├─ Phase 4 点位注册: ✅ %d 个点位", len(pointRegistry))
	t.Logf("  ├─ Phase 5 实时轮询: ✅ %d 个点位中有 %d 个变化", len(snapshots), changedCount)
	t.Logf("  └─ Phase 6 可写点写入: ✅ %d/%d 成功", totalWriteSuccess, totalWriteAttempts)
	t.Logf("")
	t.Logf("✅ 所有测试阶段通过!")
	t.Log("═══════════════════════════════════════════════════════════════")
}
