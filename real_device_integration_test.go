package bacnet

import (
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
)

/*
目标设备是本机 Bacnet.Room.Simulator.exe，它已经占用 47808。测试程序也绑定 47808 时，confirmed 单播会发生端口抢包。
已把真实测试改成：发现阶段用 47808，发现后关闭；ReadProperty/WriteProperty 阶段用 47809 发往设备 47808
go test . -run TestRealDeviceAcceptanceFlow -count=1 -v

PASS
Phase 1: 设备发现 3/3 成功
Phase 2: 扫描 13 个对象，找到 AnalogInput:0 Name="Temperature.Indoor"
Phase 3: 连续读取 10/10 成功，平均 RTT 1ms
Phase 4: 写入 AnalogValue:1 = 300，3/3 成功，Reliability=0
总耗时: 16.296s
*/
type realDeviceTestConfig struct {
	localIP              string
	localPort            int
	confirmedLocalPort   int
	subnetCIDR           int
	targetDeviceID       int
	targetIP             string
	targetPort           int
	targetPointName      string
	targetReadType       btypes.ObjectType
	targetReadInstance   btypes.ObjectInstance
	targetWriteName      string
	targetWriteType      btypes.ObjectType
	targetWriteInstance  btypes.ObjectInstance
	targetWriteValue     float32
	expectedPresentValue *float64
	readCount            int
	readInterval         time.Duration
	readTimeout          time.Duration
	scanMaxDuration      time.Duration
	maxAverageRTT        time.Duration
}

func TestRealDeviceAcceptanceFlow(t *testing.T) {
	// 测试开始日志
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("                    BACnet 真实设备集成测试开始")
	t.Log("═══════════════════════════════════════════════════════════════")

	cfg := realDeviceConfig()

	// 输出测试配置
	t.Logf("")
	t.Logf("【测试配置】")
	t.Logf("  ├─ 本地配置: IP=%s, Port=%d, ConfirmedPort=%d, SubnetCIDR=%d",
		cfg.localIP, cfg.localPort, cfg.confirmedLocalPort, cfg.subnetCIDR)
	t.Logf("  ├─ 目标设备: IP=%s:%d, DeviceID=%d", cfg.targetIP, cfg.targetPort, cfg.targetDeviceID)
	t.Logf("  ├─ 读取目标: Type=%s, Instance=%d, Name=%q",
		cfg.targetReadType, cfg.targetReadInstance, cfg.targetPointName)
	t.Logf("  ├─ 写入目标: Type=%s, Instance=%d, Name=%q, Value=%v",
		cfg.targetWriteType, cfg.targetWriteInstance, cfg.targetWriteName, cfg.targetWriteValue)
	t.Logf("  └─ 测试参数: ReadCount=%d, ReadInterval=%s, ReadTimeout=%s, ScanTimeout=%s, MaxAvgRTT=%s",
		cfg.readCount, cfg.readInterval, cfg.readTimeout, cfg.scanMaxDuration, cfg.maxAverageRTT)
	t.Logf("")

	startAll := time.Now()

	// Phase 0: Client 初始化 - 发现客户端
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("                    Phase 0: Client 初始化")
	t.Log("───────────────────────────────────────────────────────────────")

	t.Log("[Phase 0] 创建发现客户端...")
	discoveryClient, err := NewClient(&ClientBuilder{
		Ip:         cfg.localIP,
		Port:       cfg.localPort,
		SubnetCIDR: cfg.subnetCIDR,
		MaxPDU:     btypes.MaxAPDU1476,
	})
	if err != nil {
		t.Fatalf("[Phase 0] ❌ 客户端初始化失败: %v", err)
	}
	defer discoveryClient.Close()
	t.Log("[Phase 0] ✓ 客户端创建成功")

	t.Log("[Phase 0] 启动客户端接收循环...")
	go discoveryClient.ClientRun()
	time.Sleep(500 * time.Millisecond)
	if !discoveryClient.IsRunning() {
		t.Fatalf("[Phase 0] ❌ 客户端接收循环未启动")
	}
	t.Logf("[Phase 0] ✓ 客户端运行正常，耗时: %s", time.Since(startAll).Round(time.Millisecond))

	// Phase 1: 设备发现
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("                    Phase 1: 设备发现 (WhoIs)")
	t.Log("───────────────────────────────────────────────────────────────")

	device := discoverRealDevice(t, discoveryClient, cfg)

	t.Log("[Phase 1] 关闭发现客户端...")
	if err := discoveryClient.Close(); err != nil {
		t.Fatalf("[Phase 1] ❌ 关闭发现客户端失败: %v", err)
	}
	t.Log("[Phase 1] ✓ 发现客户端关闭成功")

	// Phase 0: Client 初始化 - 确认客户端
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 0: Confirmed Client 初始化")
	t.Log("───────────────────────────────────────────────────────────────")

	t.Logf("[Phase 0] 创建确认客户端 (端口: %d )...", cfg.confirmedLocalPort)
	confirmedClient, err := NewClient(&ClientBuilder{
		Ip:         cfg.localIP,
		Port:       cfg.confirmedLocalPort,
		SubnetCIDR: cfg.subnetCIDR,
		MaxPDU:     btypes.MaxAPDU1476,
	})
	if err != nil {
		t.Fatalf("[Phase 0] ❌ 确认客户端初始化失败: %v", err)
	}
	defer confirmedClient.Close()

	t.Log("[Phase 0] 启动确认客户端接收循环...")
	go confirmedClient.ClientRun()
	time.Sleep(500 * time.Millisecond)
	if !confirmedClient.IsRunning() {
		t.Fatalf("[Phase 0] ❌ 确认客户端接收循环未启动")
	}
	t.Log("[Phase 0] ✓ 确认客户端运行正常")

	// Phase 1 补充: 读取设备信息
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              Phase 1 补充: 读取设备元数据")
	t.Log("───────────────────────────────────────────────────────────────")
	readAndLogDeviceInfo(t, confirmedClient, device, cfg)

	// Phase 2: 点位扫描
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("                    Phase 2: 点位扫描 (Objects)")
	t.Log("───────────────────────────────────────────────────────────────")
	scanned := scanRealDeviceObjects(t, confirmedClient, device, cfg)

	// Phase 3: 值读取
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("                Phase 3: 值读取 (ReadProperty)")
	t.Log("───────────────────────────────────────────────────────────────")
	readRealDevicePresentValue(t, confirmedClient, scanned, cfg)

	// Phase 4: 值写入
	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("                Phase 4: 值写入 (WriteProperty)")
	t.Log("───────────────────────────────────────────────────────────────")
	writeRealDevicePresentValue(t, confirmedClient, scanned, cfg)

	// 测试结束
	totalElapsed := time.Since(startAll)
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("                    BACnet 真实设备集成测试完成")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Logf("")
	t.Logf("【测试结果汇总】")
	t.Logf("  ├─ 总耗时: %s", totalElapsed.Round(time.Millisecond))
	t.Logf("  ├─ 设备信息: ID=%d, IP=%s:%d", device.DeviceID, device.Ip, device.Port)
	t.Logf("  ├─ 读取点位: %s:%d, Name=%q", cfg.targetReadType, cfg.targetReadInstance, cfg.targetPointName)
	t.Logf("  └─ 写入点位: %s:%d, Name=%q, Value=%v", cfg.targetWriteType, cfg.targetWriteInstance, cfg.targetWriteName, cfg.targetWriteValue)
	t.Logf("")
	t.Logf("✅ 所有测试阶段通过!")
	t.Log("═══════════════════════════════════════════════════════════════")
}

func realDeviceConfig() realDeviceTestConfig {
	return realDeviceTestConfig{
		localIP:             "0.0.0.0",
		localPort:           47808,
		confirmedLocalPort:  47809,
		subnetCIDR:          24,
		targetDeviceID:      2228316,
		targetIP:            "192.168.3.113",
		targetPort:          47808,
		targetPointName:     "Temperature.Indoor",
		targetReadType:      btypes.AnalogInput,
		targetReadInstance:  0,
		targetWriteName:     "Setpoint.1",
		targetWriteType:     btypes.AnalogValue,
		targetWriteInstance: 1,
		targetWriteValue:    300,
		readCount:           10,
		readInterval:        time.Second,
		readTimeout:         5 * time.Second,
		scanMaxDuration:     5 * time.Second,
		maxAverageRTT:       300 * time.Millisecond,
	}
}

func discoverRealDevice(t *testing.T, client Client, cfg realDeviceTestConfig) btypes.Device {
	t.Helper()

	targetIP := net.ParseIP(cfg.targetIP)
	if targetIP == nil {
		t.Fatalf("[Phase 1] ❌ 无效的目标IP地址: %q", cfg.targetIP)
	}

	t.Logf("[Phase 1] 开始设备发现，目标设备ID: %d, 目标地址: %s:%d", cfg.targetDeviceID, cfg.targetIP, cfg.targetPort)
	t.Logf("[Phase 1] 将执行 %d 次发现尝试...", 3)

	var found btypes.Device
	for attempt := 1; attempt <= 3; attempt++ {
		t.Logf("[Phase 1] ── 第 %d 次尝试 ──", attempt)
		start := time.Now()

		t.Logf("[Phase 1] 发送 WhoIs 请求 (Low=%d, High=%d)...", cfg.targetDeviceID, cfg.targetDeviceID)
		devices, err := client.WhoIs(&WhoIsOpts{
			Low:         cfg.targetDeviceID,
			High:        cfg.targetDeviceID,
			Destination: datalink.IPPortToAddress(targetIP, cfg.targetPort),
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("[Phase 1] ❌ 第 %d 次尝试失败: %v, 耗时: %s", attempt, err, elapsed.Round(time.Millisecond))
		}

		t.Logf("[Phase 1] ✓ WhoIs 请求完成，耗时: %s, 发现 %d 个设备", elapsed.Round(time.Millisecond), len(devices))

		var matched *btypes.Device
		for i := range devices {
			dev := devices[i]
			t.Logf("[Phase 1]   发现设备: ID=%d, IP=%s, Port=%d, MaxAPDU=%d, Segmentation=%v, Vendor=%v",
				dev.DeviceID, dev.Ip, dev.Port, dev.MaxApdu, dev.Segmentation, dev.Vendor)
			if dev.DeviceID == cfg.targetDeviceID {
				matched = &dev
				t.Logf("[Phase 1]   ✅ 找到目标设备!")
			}
		}

		if matched == nil {
			t.Fatalf("[Phase 1] ❌ 第 %d 次尝试未找到目标设备 %d，共发现 %d 个设备", attempt, cfg.targetDeviceID, len(devices))
		}
		found = *matched
		t.Logf("[Phase 1] ✓ 第 %d 次尝试成功匹配目标设备", attempt)
	}

	t.Logf("[Phase 1] ✅ 设备发现完成，3/3 次尝试全部成功")
	t.Logf("[Phase 1]   设备详情: ID=%d, IP=%s:%d, MaxAPDU=%d, Segmentation=%v, Vendor=%v",
		found.DeviceID, found.Ip, found.Port, found.MaxApdu, found.Segmentation, found.Vendor)

	return found
}

func scanRealDeviceObjects(t *testing.T, client Client, device btypes.Device, cfg realDeviceTestConfig) btypes.Device {
	t.Helper()

	t.Logf("[Phase 2] 开始扫描设备对象，目标: %s:%d", cfg.targetReadType, cfg.targetReadInstance)
	start := time.Now()

	t.Log("[Phase 2] 调用 client.Objects() 读取对象列表...")
	scanned, err := client.Objects(device)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("[Phase 2] ❌ 对象扫描失败: %v, 耗时: %s", err, elapsed.Round(time.Millisecond))
	}
	t.Logf("[Phase 2] ✓ 对象扫描完成，耗时: %s", elapsed.Round(time.Millisecond))

	if elapsed > cfg.scanMaxDuration {
		t.Fatalf("[Phase 2] ❌ 对象扫描超时: 实际耗时=%s, 阈值=%s", elapsed.Round(time.Millisecond), cfg.scanMaxDuration)
	}

	total := scanned.Objects.Len()
	t.Logf("[Phase 2] 扫描结果: 共发现 %d 个对象", total)

	// 检查读取目标
	t.Logf("[Phase 2] 查找读取目标: %s:%d (Name=%q)", cfg.targetReadType, cfg.targetReadInstance, cfg.targetPointName)
	readObjects := scanned.Objects[cfg.targetReadType]
	readObject, ok := readObjects[cfg.targetReadInstance]
	if !ok {
		t.Fatalf("[Phase 2] ❌ 未找到读取目标 %s:%d，扫描到 %d 个对象", cfg.targetReadType, cfg.targetReadInstance, total)
	}
	t.Logf("[Phase 2] ✓ 找到读取目标: Name=%q", readObject.Name)

	if readObject.Name != cfg.targetPointName {
		t.Fatalf("[Phase 2] ❌ 读取目标名称不匹配: 实际=%q, 期望=%q", readObject.Name, cfg.targetPointName)
	}
	t.Logf("[Phase 2] ✓ 读取目标名称匹配: %q", cfg.targetPointName)

	// 检查写入目标
	t.Logf("[Phase 2] 查找写入目标: %s:%d (Name=%q)", cfg.targetWriteType, cfg.targetWriteInstance, cfg.targetWriteName)
	writeObjects := scanned.Objects[cfg.targetWriteType]
	writeObject, ok := writeObjects[cfg.targetWriteInstance]
	if !ok {
		t.Fatalf("[Phase 2] ❌ 未找到写入目标 %s:%d，扫描到 %d 个对象", cfg.targetWriteType, cfg.targetWriteInstance, total)
	}
	t.Logf("[Phase 2] ✓ 找到写入目标: Name=%q", writeObject.Name)

	if writeObject.Name != "" && writeObject.Name != cfg.targetWriteName {
		t.Fatalf("[Phase 2] ❌ 写入目标名称不匹配: 实际=%q, 期望=%q", writeObject.Name, cfg.targetWriteName)
	}

	// 读取并记录对象元数据
	t.Log("[Phase 2] 读取对象元数据...")
	readAndLogObjectMetadata(t, client, scanned, cfg.targetReadType, cfg.targetReadInstance, "读取目标")
	readAndLogObjectMetadata(t, client, scanned, cfg.targetWriteType, cfg.targetWriteInstance, "写入目标")

	t.Logf("[Phase 2] ✅ 对象扫描完成，扫描 %d 个对象，找到目标点位 %s:%d Name=%q，耗时: %s",
		total, cfg.targetReadType, cfg.targetReadInstance, readObject.Name, elapsed.Round(time.Millisecond))

	return scanned
}

func readRealDevicePresentValue(t *testing.T, client Client, device btypes.Device, cfg realDeviceTestConfig) {
	t.Helper()

	t.Logf("[Phase 3] 开始连续读取测试，目标: %s:%d，读取次数: %d，间隔: %s，超时: %s",
		cfg.targetReadType, cfg.targetReadInstance, cfg.readCount, cfg.readInterval, cfg.readTimeout)

	var successes int
	var totalRTT time.Duration
	var minRTT, maxRTT time.Duration

	for i := 1; i <= cfg.readCount; i++ {
		t.Logf("[Phase 3] ── 第 %d/%d 次读取 ──", i, cfg.readCount)
		start := time.Now()

		t.Logf("[Phase 3] 读取 PresentValue...")
		value, err := readPropertyValue(client, device, cfg.targetReadType, cfg.targetReadInstance, btypes.PropPresentValue, cfg.readTimeout)
		rtt := time.Since(start)

		if err != nil {
			t.Fatalf("[Phase 3] ❌ 第 %d/%d 次读取失败: %v, RTT: %s", i, cfg.readCount, err, rtt.Round(time.Millisecond))
		}

		// 初始化 min/max RTT
		if i == 1 {
			minRTT = rtt
			maxRTT = rtt
		} else {
			if rtt < minRTT {
				minRTT = rtt
			}
			if rtt > maxRTT {
				maxRTT = rtt
			}
		}

		num, ok := numericValue(value)
		if !ok {
			t.Fatalf("[Phase 3] ❌ 第 %d/%d 次读取返回非数值类型: %T=%v", i, cfg.readCount, value, value)
		}

		if cfg.expectedPresentValue != nil && math.Abs(num-*cfg.expectedPresentValue) > 0.5 {
			t.Fatalf("[Phase 3] ❌ 第 %d/%d 次读取值偏差过大: 实际=%.3f, 期望=%.3f, 容差=0.5",
				i, cfg.readCount, num, *cfg.expectedPresentValue)
		}

		successes++
		totalRTT += rtt
		t.Logf("[Phase 3] ✓ 第 %d/%d 次读取成功: PresentValue=%v (%T), RTT=%s",
			i, cfg.readCount, value, value, rtt.Round(time.Millisecond))

		if i < cfg.readCount {
			t.Logf("[Phase 3] 等待 %s 后进行下一次读取...", cfg.readInterval)
			time.Sleep(cfg.readInterval)
		}
	}

	avg := totalRTT / time.Duration(successes)
	t.Logf("[Phase 3] 读取统计: 成功=%d/%d, 总耗时=%s, 平均RTT=%s, 最小RTT=%s, 最大RTT=%s",
		successes, cfg.readCount, totalRTT.Round(time.Millisecond), avg.Round(time.Millisecond),
		minRTT.Round(time.Millisecond), maxRTT.Round(time.Millisecond))

	if avg > cfg.maxAverageRTT {
		t.Fatalf("[Phase 3] ❌ 平均RTT超过阈值: 平均=%s, 阈值=%s", avg.Round(time.Millisecond), cfg.maxAverageRTT)
	}

	t.Logf("[Phase 3] ✅ 连续读取测试通过，成功率=100%%，平均RTT=%s (阈值=%s)",
		avg.Round(time.Millisecond), cfg.maxAverageRTT)
}

func writeRealDevicePresentValue(t *testing.T, client Client, device btypes.Device, cfg realDeviceTestConfig) {
	t.Helper()

	t.Logf("[Phase 4] 开始写入测试，目标: %s:%d，写入值: %v，验证次数: 3",
		cfg.targetWriteType, cfg.targetWriteInstance, cfg.targetWriteValue)

	for i := 1; i <= 3; i++ {
		t.Logf("[Phase 4] ── 第 %d/3 次写入 ──", i)
		start := time.Now()

		t.Logf("[Phase 4] 写入 PresentValue=%v...", cfg.targetWriteValue)
		err := client.WriteProperty(device, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{
					Type:     cfg.targetWriteType,
					Instance: cfg.targetWriteInstance,
				},
				Properties: []btypes.Property{
					{
						Type:       btypes.PropPresentValue,
						ArrayIndex: btypes.ArrayAll,
						Data:       cfg.targetWriteValue,
						Priority:   btypes.Normal,
					},
				},
			},
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("[Phase 4] ❌ 第 %d/3 次写入失败: %v, 耗时: %s", i, err, elapsed.Round(time.Millisecond))
		}
		t.Logf("[Phase 4] ✓ 写入成功，耗时: %s", elapsed.Round(time.Millisecond))

		// 验证写入值
		t.Log("[Phase 4] 验证写入值...")
		value, err := readPropertyValue(client, device, cfg.targetWriteType, cfg.targetWriteInstance, btypes.PropPresentValue, cfg.readTimeout)
		if err != nil {
			t.Fatalf("[Phase 4] ❌ 第 %d/3 次读取验证失败: %v", i, err)
		}
		num, ok := numericValue(value)
		if !ok {
			t.Fatalf("[Phase 4] ❌ 第 %d/3 次读取验证返回非数值: %T=%v", i, value, value)
		}
		if math.Abs(num-float64(cfg.targetWriteValue)) > 0.001 {
			t.Fatalf("[Phase 4] ❌ 第 %d/3 次读取验证值不匹配: 实际=%.3f, 期望=%.3f", i, num, cfg.targetWriteValue)
		}
		t.Logf("[Phase 4] ✓ 写入值验证成功: PresentValue=%v", value)

		// 验证 Reliability
		t.Log("[Phase 4] 验证 Reliability...")
		reliability, err := readPropertyValue(client, device, cfg.targetWriteType, cfg.targetWriteInstance, btypes.PropReliability, cfg.readTimeout)
		if err != nil {
			t.Fatalf("[Phase 4] ❌ 第 %d/3 次 Reliability 读取失败: %v", i, err)
		}
		if !isNoFaultReliability(reliability) {
			t.Fatalf("[Phase 4] ❌ 第 %d/3 次 Reliability 不是 'No Fault Detected': %T=%v", i, reliability, reliability)
		}
		t.Logf("[Phase 4] ✓ Reliability 验证成功: %v", reliability)

		t.Logf("[Phase 4] ✅ 第 %d/3 次写入验证完成，耗时: %s", i, elapsed.Round(time.Millisecond))
	}

	t.Log("[Phase 4] ✅ 写入测试全部通过，3/3 次写入验证成功")
}

func readAndLogDeviceInfo(t *testing.T, client Client, device btypes.Device, cfg realDeviceTestConfig) {
	t.Helper()

	t.Log("[Phase 1] 读取设备元数据...")
	props := []btypes.PropertyType{
		btypes.PropObjectName,
		btypes.PropModelName,
		btypes.PROP_FIRMWARE_REVISION,
		btypes.PropVendorName,
		btypes.PropVendorIdentifier,
		btypes.PropMaxAPDU,
		btypes.PropSegmentationSupported,
	}

	for _, prop := range props {
		t.Logf("[Phase 1]   读取属性 %s...", btypes.String(prop))
		value, err := readPropertyValue(client, device, btypes.DeviceType, btypes.ObjectInstance(cfg.targetDeviceID), prop, cfg.readTimeout)
		if err != nil {
			t.Logf("[Phase 1]   ⚠️ 属性 %s 不可用: %v", btypes.String(prop), err)
			continue
		}
		t.Logf("[Phase 1]   ✓ 属性 %s=%v (%T)", btypes.String(prop), value, value)
	}
}

func readAndLogObjectMetadata(t *testing.T, client Client, device btypes.Device, objectType btypes.ObjectType, instance btypes.ObjectInstance, label string) {
	t.Helper()

	t.Logf("[Phase 2]   读取 %s 元数据...", label)
	props := []btypes.PropertyType{
		btypes.PropObjectName,
		btypes.PropDescription,
		btypes.PropObjectType,
		btypes.PropUnits,
		btypes.PropPresentValue,
		btypes.PropReliability,
		btypes.PropStatusFlags,
	}

	for _, prop := range props {
		t.Logf("[Phase 2]     读取属性 %s...", btypes.String(prop))
		value, err := readPropertyValue(client, device, objectType, instance, prop, 5*time.Second)
		if err != nil {
			t.Logf("[Phase 2]     ⚠️ 属性 %s 不可用: %v", btypes.String(prop), err)
			continue
		}
		t.Logf("[Phase 2]     ✓ 属性 %s=%v (%T)", btypes.String(prop), value, value)
	}
}

func readPropertyValue(client Client, device btypes.Device, objectType btypes.ObjectType, instance btypes.ObjectInstance, property btypes.PropertyType, timeout time.Duration) (interface{}, error) {
	resp, err := client.ReadPropertyWithTimeout(device, btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     objectType,
				Instance: instance,
			},
			Properties: []btypes.Property{
				{
					Type:       property,
					ArrayIndex: btypes.ArrayAll,
				},
			},
		},
	}, timeout)
	if err != nil {
		return nil, err
	}
	if len(resp.Object.Properties) == 0 {
		return nil, fmt.Errorf("response contains no properties")
	}
	return resp.Object.Properties[0].Data, nil
}

func numericValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case btypes.Enumerated:
		return float64(v), true
	default:
		return 0, false
	}
}

func isNoFaultReliability(value interface{}) bool {
	num, ok := numericValue(value)
	if ok {
		return num == 0
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(value)), "no fault")
}
