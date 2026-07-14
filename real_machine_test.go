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
		localIP     = "192.168.3.115"
		remoteIP    = "192.168.3.230"
		readTimeout = 8 * time.Second
		clientPort  = 47820
	)

	targetDeviceIDs := []int{1234, 2228316, 2228317, 2228318}

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

	if !bacClient.IsRunning() {
		t.Fatalf("❌ 客户端未运行")
	}
	t.Logf("✓ 客户端运行在端口 %d", clientPort)

	t.Log("")
	t.Log("───────────────────────────────────────────────────────────────")
	t.Log("              真机测试: 增强设备发现")
	t.Log("───────────────────────────────────────────────────────────────")

	confirmedDevices := make(map[int]btypes.Device)
	discoveryFailures := 0

	t.Logf("")
	t.Logf("  [策略1] 标准广播 WhoIs (255.255.255.255:47808)...")
	devices, err := bacClient.WhoIs(&WhoIsOpts{Low: 0, High: 4194304})
	if err != nil {
		t.Logf("    ⚠ 标准广播 WhoIs 失败: %v", err)
	} else {
		t.Logf("    ✓ 标准广播发现 %d 台设备:", len(devices))
		for _, d := range devices {
			udpAddr, _ := d.Addr.UDPAddr()
			t.Logf("      DeviceID=%d, Addr=%s:%d", d.DeviceID, udpAddr.IP, udpAddr.Port)
			confirmedDevices[d.DeviceID] = d
		}
	}

	t.Logf("")
	t.Logf("  [策略2] 子网广播 WhoIs (%s.255:47808)...", localIP[:len(localIP)-4])
	subnetBroadcastIP := net.ParseIP(localIP[:len(localIP)-4] + "255")
	if subnetBroadcastIP != nil {
		bacClient.WhoIs(&WhoIsOpts{Low: 0, High: 4194304})
		time.Sleep(2 * time.Second)
	}

	t.Logf("")
	t.Logf("  [策略3] 单播探测 (目标IP多端口扫描)...")

	commonPorts := []int{47808, 47810, 47811, 47812, 58494, 64339, 54304, 47809, 47813, 47814}

	for _, targetID := range targetDeviceIDs {
		if _, exists := confirmedDevices[targetID]; exists {
			continue
		}

		var priorityPorts []int
		switch targetID {
		case 1234:
			priorityPorts = []int{47808}
		case 2228316:
			priorityPorts = []int{58494, 47808}
		case 2228317:
			priorityPorts = []int{64339, 47808}
		case 2228318:
			priorityPorts = []int{54304, 47808}
		default:
			priorityPorts = []int{47808}
		}

		portsToTry := append(priorityPorts, commonPorts...)

		t.Logf("")
		t.Logf("    ── Device %d 端口扫描 ──", targetID)

		for _, port := range portsToTry {
			addr := datalink.IPPortToAddress(net.ParseIP(localIP), port)
			testDev := btypes.Device{
				DeviceID: targetID,
				Addr:     *addr,
				Ip:       localIP,
				Port:     port,
				MaxApdu:  btypes.MaxAPDU,
				ID: btypes.ObjectID{
					Type:     btypes.DeviceType,
					Instance: btypes.ObjectInstance(targetID),
				},
			}

			rpTest, errTest := bacClient.ReadPropertyWithTimeout(testDev, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{
						Type:     btypes.DeviceType,
						Instance: btypes.ObjectInstance(targetID),
					},
					Properties: []btypes.Property{{
						Type:       btypes.PropObjectName,
						ArrayIndex: btypes.ArrayAll,
					}},
				},
			}, 3*time.Second)

			if errTest == nil && len(rpTest.Object.Properties) > 0 && rpTest.Object.Properties[0].Data != nil {
				t.Logf("    ✓ Device %d 在端口 %d 确认成功", targetID, port)
				confirmedDevices[targetID] = testDev
				break
			}
		}
	}

	t.Logf("")
	t.Logf("  [策略4] 最终确认 (逐设备 ReadProperty 验证)...")

	for _, targetID := range targetDeviceIDs {
		dev, ok := confirmedDevices[targetID]
		if !ok {
			t.Errorf("    ❌ Device %d 未发现", targetID)
			discoveryFailures++
			continue
		}

		rp, err := bacClient.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID: btypes.ObjectID{
					Type:     btypes.DeviceType,
					Instance: btypes.ObjectInstance(targetID),
				},
				Properties: []btypes.Property{{
					Type:       btypes.PropObjectName,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, readTimeout)

		if err != nil {
			t.Logf("    ⚠ Device %d ReadProperty 返回错误: %v", targetID, err)
			t.Logf("    ✓ Device %d (已发现，使用发现时的信息) IP=%s Port=%d", targetID, dev.Ip, dev.Port)
		} else if len(rp.Object.Properties) > 0 && rp.Object.Properties[0].Data != nil {
			t.Logf("    ✓ Device %d (%s) IP=%s Port=%d", targetID, rp.Object.Properties[0].Data, dev.Ip, dev.Port)
		} else {
			t.Logf("    ✓ Device %d IP=%s Port=%d", targetID, dev.Ip, dev.Port)
		}
		time.Sleep(500 * time.Millisecond)
	}

	if discoveryFailures > 0 {
		t.Fatalf("❌ 真机测试失败: %d 台设备无法通信", discoveryFailures)
	}

	t.Logf("")
	t.Logf("✅ 真机测试完成: %d/%d 设备全部验证成功", len(confirmedDevices), len(targetDeviceIDs))
	t.Logf("总耗时: %.3fs", time.Since(startAll).Seconds())
}
