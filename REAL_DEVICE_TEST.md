# BACnet 真机联调自测指南

本文档供你在真实 BACnet 设备在线时自行验证本仓库客户端能力，并回填结果。

**关联报告**：`TEST_REPORT.md`（本机 UDP 环回自动化）  
**历史真机记录**：`test/real-device-test-report.md`

---

## 一、环境准备

| 项目 | 要求 |
|------|------|
| 操作系统 | Windows / Linux / macOS（本仓库在 Windows 上开发验证） |
| Go | 1.21+（建议与本机一致） |
| 网络 | 本机与目标设备同一二层网段（或可达的 BACnet/IP 路由） |
| 目标设备 | 已上电，UDP `47808`（或实际端口）可响应 Who-Is |
| 防火墙 | 允许本机 UDP 出站/入站（尤其 `47808`、`47809`） |
| 工具（可选） | YABE / Wireshark 对照抓包 |

### 注意：端口冲突

若目标设备（或本机模拟器）已占用 `47808`，本仓库验收测试采用：

- **发现阶段**：本地绑定 `47808`
- **确认服务读写**：本地绑定 `47809`，发往设备 `47808`

详见 `real_device_integration_test.go` 顶部注释。

---

## 二、配置项

验收测试配置集中在 `real_device_integration_test.go` 的 `realDeviceConfig()`：

| 字段 | 默认值 | 含义 |
|------|--------|------|
| `localIP` | `0.0.0.0` | 本机绑定 IP |
| `localPort` | `47808` | 发现用本地端口 |
| `confirmedLocalPort` | `47809` | 读写用本地端口 |
| `subnetCIDR` | `24` | 子网 CIDR |
| `targetDeviceID` | `2228316` | 目标设备实例号 |
| `targetIP` | `192.168.3.114` | 目标 IP |
| `targetPort` | `47808` | 目标端口 |
| `targetPointName` | `Temperature.Indoor` | 扫描时期望的点名 |
| `targetReadType/Instance` | `AnalogInput` / `0` | 读点 |
| `targetWriteType/Instance/Value` | `AnalogValue` / `1` / `300` | 写点 |

**自测前请按现场设备修改上述值后保存。**

---

## 三、运行自动化真机验收（推荐）

在仓库根目录：

```bash
cd d:/code/GitHub/bacnet
go test . -run TestRealDeviceAcceptanceFlow -count=1 -v
```

### 预期阶段

| 阶段 | 内容 | 通过标准（示例） |
|------|------|------------------|
| Phase 0 | 创建客户端并 `ClientRun` | 客户端运行中 |
| Phase 1 | Who-Is / I-Am 发现 | 发现目标 DeviceID |
| Phase 2 | `Objects()` 扫描 | 找到目标点名（如 Temperature.Indoor） |
| Phase 3 | 连续 ReadProperty | 约定次数成功，平均 RTT 达标 |
| Phase 4 | WriteProperty | 写入成功并可读回 |

设备离线或网段不通时，测试会失败（不会被默认 skip）。日常 CI / 本机无真机时请使用：

```bash
go test ./... -count=1 -timeout 180s -skip TestRealDeviceAcceptanceFlow
```

---

## 四、用本库客户端手动验证（可选）

### 4.1 启动本库服务端（环回对照，非真机）

```go
cfg := server.DefaultDeviceConfig()
cfg.Ip = "127.0.0.1"
cfg.Port = 47808
cfg.DeviceID = 2001
srv, _ := server.NewServer(cfg)
go srv.Serve()
```

自动化环回互操作已覆盖：`go test ./server/ -run TestClientServerInterop_UDP -v`

### 4.2 对真机发 Who-Is

```go
client, _ := bacnet.NewClient(&bacnet.ClientBuilder{
    Ip: "0.0.0.0", Port: 47809, SubnetCIDR: 24, MaxPDU: btypes.MaxAPDU1476,
})
go client.ClientRun()

devices, err := client.WhoIs(&bacnet.WhoIsOpts{
    Low:  targetID, High: targetID,
    Destination: datalink.IPPortToAddress(net.ParseIP("192.168.x.x"), 47808),
})
```

### 4.3 读写与 COV

- `ReadProperty` / `WriteProperty` / `ReadMultiProperty` / `WriteMultiProperty`
- `Objects(dev)` 扫描 ObjectList
- `SubscribeCOV` + `WaitCOVNotification` + `CancelSubscribeCOV`（需设备支持 SubscribeCOV）

---

## 五、服务覆盖清单（请勾选）

将结果记入下方「结果回填」表。

| 服务 / 能力 | 本库支持 | 真机验证 | 备注 |
|-------------|----------|----------|------|
| Who-Is / I-Am | 是 | ☐ | 验收 Phase 1 |
| ReadProperty | 是 | ☐ | Phase 3 |
| WriteProperty | 是 | ☐ | Phase 4 |
| ReadPropertyMultiple | 是 | ☐ | 可用客户端 API 补测 |
| WritePropertyMultiple | 是 | ☐ | 可用客户端 API 补测 |
| ObjectList / Objects 扫描 | 是 | ☐ | Phase 2 |
| SubscribeCOV + COV Notification | 是（本机环回已测） | ☐ | 依赖设备是否实现 COV |
| MS/TP | **否**（源码注释掉，需 CGO/串口） | ☐ N/A | 见 TEST_REPORT |
| APDU 分段收发 | **否**（服务端 Abort：segmentation-not-supported） | ☐ N/A | |
| 网络层路由（Who-Is-Router / What-Is-Network-Number） | 客户端 beta；服务端忽略 | ☐ | 需路由器场景 |

---

## 六、失败排查

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| Who-Is 无设备 | 设备离线、网段不通、广播被抑制 | ping/抓包；改用单播 Destination |
| 发现成功但读写超时 | 本地端口与设备抢包（同绑 47808） | 使用 `confirmedLocalPort=47809` |
| Error UnknownObject | 实例号/类型与现场不一致 | 先 `Objects()` 确认 |
| Write 被拒 | 优先级、只读点、设备权限 | 换可写 AV/BV，查 Reliability |
| COV 无通知 | 设备不支持或需不同 lifetime | 用 YABE 对照；本机可用环回服务端验证客户端 |
| Abort segmentation-not-supported | 对端发了分段 APDU | 本库不支持分段，属预期 |

抓包建议过滤：`udp port 47808 or udp port 47809`。

---

## 七、结果回填模板

复制到 issue / 本地笔记：

```text
日期:
测试人:
本机 IP / OS / Go 版本:
目标设备: IP=  Port=  DeviceID=  型号/固件=

TestRealDeviceAcceptanceFlow: PASS / FAIL
  Phase1 Who-Is:
  Phase2 Objects:
  Phase3 Read:
  Phase4 Write:

补充:
  RPM/WPM:
  COV:
  备注 / 抓包文件:
```

历史通过示例（模拟器）见 `real_device_integration_test.go` 文件头注释与 `test/real-device-test-report.md`。

---

## 八、与本机自动化的关系

| 场景 | 命令 |
|------|------|
| 无真机，全绿自动化 | `go test ./... -count=1 -timeout 180s -skip TestRealDeviceAcceptanceFlow` |
| 仅环回客户端↔服务端 | `go test ./server/ -run TestClientServerInterop_UDP -v` |
| 有真机 | `go test . -run TestRealDeviceAcceptanceFlow -count=1 -v` |
