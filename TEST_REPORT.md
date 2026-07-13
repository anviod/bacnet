# BACnet 客户端 / 服务端互操作测试报告

**日期**: 2026-07-13（本轮更新）  
**仓库**: `d:\code\GitHub\bacnet`  
**模块**: `github.com/anviod/bacnet`

---

## 一、测试环境

| 项目 | 值 |
|------|-----|
| 操作系统 | Windows 10 (22631) |
| Go 版本 | go1.26.4 windows/amd64 |
| 联调方式 | 本机环回 `127.0.0.1`，客户端与服务端不同 UDP 端口 |
| 自动化命令 | `go test ./... -count=1 -timeout 180s -skip TestRealDeviceAcceptanceFlow` |
| 互操作专项 | `go test ./server/ -run TestClientServerInterop_UDP -v` |
| 真机自测文档 | [`REAL_DEVICE_TEST.md`](REAL_DEVICE_TEST.md) |

---

## 二、验证范围

### 2.1 客户端能力

- Who-Is / I-Am 设备发现
- ReadProperty / WriteProperty
- ReadPropertyMultiple / WritePropertyMultiple
- Objects 对象扫描（ObjectList + 名称/描述）
- **SubscribeCOV / CancelSubscribeCOV / WaitCOVNotification**（本轮新增）
- TSM 确认服务事务

### 2.2 服务端能力（`server` 包）

- Who-Is → I-Am 自动响应
- ReadProperty / WriteProperty / RPM / WPM
- **SubscribeCOV**：订阅、初始通知、PresentValue 变更通知、取消订阅
- **Unconfirmed / Confirmed COV Notification** 发送
- 对象存储与 Device ObjectList
- BACnet Error PDU；分段请求 → **Abort (segmentation-not-supported)**
- **DeviceID=0 合法保留**（不再静默改成 1000）；`> MaxInstance` 返回错误

### 2.3 明确不支持 / 未完整验证

| 能力 | 状态 | 说明 |
|------|------|------|
| 真机联调 | 需现场自测 | 见 [`REAL_DEVICE_TEST.md`](REAL_DEVICE_TEST.md)；设备离线时跳过验收用例 |
| MS/TP | **未实现** | `datalink/mstp.go` 整文件注释，依赖 CGO/bacnet-stack，本架构未启用 |
| APDU 分段重组 | **不支持** | 默认 `NoSegmentation`；收到分段确认请求回复 Abort |
| 网络层路由服务 | **未实现（服务端）** | 服务端忽略 NPDU 网络层消息；客户端 `WhoIsRouterToNetwork` / `WhatIsNetworkNumber` 为 beta |
| SubscribeCOVProperty / 事件通知 | **未实现** | 仅对象级 SubscribeCOV |

---

## 三、本轮相对上一版的变更

1. **COV 打通**：编码层 SubscribeCOV / COV Notification；服务端订阅管理；客户端订阅与接收；UDP 环回互操作测试通过（含取消订阅）。
2. **DeviceID=0**：去掉 `DeviceID==0 → 1000` 静默改写；`nil`/`DefaultDeviceConfig()` 仍默认 1000；超额实例号报错；单测覆盖。
3. **分段**：服务端对分段确认请求发送 Abort；单测覆盖。
4. **文档**：新增真机自测指南；本报告同步更新。

---

## 四、测试执行结果

### 4.1 总体

| 包 | 结果 |
|----|------|
| `encoding`（含 COV/Abort 编解码） | PASS |
| `server`（含互操作 + DeviceID + 分段 Abort） | PASS |
| `network` / `btypes` / `tsm` / `utsm` / helpers | PASS |
| 根包 `TestRealDeviceAcceptanceFlow` | **需真机**（无设备时勿跑或会失败） |

**结论：除真机用例外，自动化测试全部通过。**

### 4.2 客户端 ↔ 服务端 UDP 互操作（`TestClientServerInterop_UDP`）

| 用例 | 结果 |
|------|------|
| WhoIs | PASS |
| ReadProperty_AnalogInput | PASS |
| WriteProperty_AnalogValue | PASS |
| ReadMultiProperty | PASS |
| WriteMultiProperty | PASS |
| ObjectList_Length | PASS |
| ObjectList_FullAndByIndex | PASS（整表 + index1 + 逐项） |
| ReadProperty_UnknownObject | PASS |
| Objects_Scan | PASS |
| **SubscribeCOV_Unconfirmed**（订阅→初始通知→变更通知→取消） | **PASS** |

### 4.3 其他新增单测

- `TestDeviceConfig_DeviceIDZeroIsPreserved` / `DeviceIDTooLarge`：PASS  
- `TestServer_SegmentedRequest_Abort`：PASS  
- `encoding`：`TestSubscribeCOV_*` / `TestCOVNotification_RoundTrip` / `TestAbort_RoundTrip`：PASS  

---

## 五、如何复现

```bash
cd d:/code/GitHub/bacnet

# 推荐：全量自动化（跳过真机）
go test ./... -count=1 -timeout 180s -skip TestRealDeviceAcceptanceFlow

# 仅互操作（含 COV）
go test ./server/ -run TestClientServerInterop_UDP -count=1 -v

# 真机（需设备在线，配置见 REAL_DEVICE_TEST.md）
go test . -run TestRealDeviceAcceptanceFlow -count=1 -v
```

---

## 六、已知限制（更新后）

1. **MS/TP**：未编译实现，需串口硬件 + 外部栈，超出当前 BACnet/IP 范围。  
2. **分段**：不重组、不发送分段；对端若强制分段会被 Abort。  
3. **网络层路由**：服务端不响应 Who-Is-Router 等；跨网段需外部路由器。  
4. **COV**：未实现 SubscribeCOVProperty、COV 增量阈值、Active_COV_Subscriptions 属性完整编码。  
5. **真机**：以 [`REAL_DEVICE_TEST.md`](REAL_DEVICE_TEST.md) 为准由用户自测回填。  

---

## 七、结论

- **本机 BACnet/IP（UDP）路径**：Who-Is/I-Am、单/多属性读写、ObjectList、对象扫描、**COV 订阅/通知/取消**、未知对象错误、分段 Abort、DeviceID=0 行为均已自动化验证。  
- **真机、MS/TP、完整分段与路由**：文档标明未验证或不支持，不假装完成。  

**总体评价：本机互操作（含 COV）已达成；真机请按 REAL_DEVICE_TEST.md 自测。**
