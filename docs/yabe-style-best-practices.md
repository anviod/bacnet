# Yabe 风格 BACnet 设备扫描最佳实践

## 概述

本文档描述如何在 BACnet/IP 协议栈中实现与官方 Yabe（Yet Another BACnet Explorer）工具完全一致的设备发现机制。Yabe 是 BACnet 生态中最广泛使用的测试和诊断工具，其设备发现方式被业界视为事实标准。

## 核心原理

Yabe 的设备发现流程可以概括为三步：

1. **绑定 INADDR_ANY** — 将 UDP socket 绑定到 `0.0.0.0`（而非特定 IP），确保能接收所有发往 BACnet 端口的包，包括发往 `255.255.255.255` 全局广播地址的 IAm 响应
2. **广播 WhoIs** — 发送单次广播 WhoIs 请求到子网广播地址（如 `192.168.3.255:47808`），不指定设备 ID 范围（即发现所有设备）
3. **收集 IAm 响应** — 等待并收集所有设备的 IAm 响应，从响应中提取设备 ID、IP 地址、端口、MaxAPDU 等信息

## 关键实现细节

### 1. Socket 绑定：必须使用 INADDR_ANY

**问题**：部分 BACnet 模拟器（如 Yabe 模拟器 `Bacnet.Room.Simulator.exe`）会将 IAm 响应发送到 `255.255.255.255:47808`（全局广播地址），而不是 WhoIs 发起方的单播地址。如果 socket 绑定到特定 IP（如 `192.168.3.230:47808`），将无法接收这些全局广播包。

**解决方案**：将 UDP socket 绑定到 `0.0.0.0:47808`（INADDR_ANY），使 socket 能够接收所有目标地址的包，包括全局广播。

```go
// 正确：绑定到 INADDR_ANY
udpAddrStr := fmt.Sprintf("0.0.0.0:%d", port)

// 错误：绑定到特定 IP（会丢失全局广播包）
// udpAddrStr := fmt.Sprintf("%s:%d", ip.String(), port)
```

**注意**：NPDU 源地址仍需使用实际 IP，以保证 BACnet 设备能正确回复单播请求。代码中通过 `myAddress` 字段区分：

```go
return &udpDataLink{
    listener:         udpConn,                        // 绑定到 0.0.0.0
    myAddress:        IPPortToAddress(actualIP, port), // NPDU 源地址使用实际 IP
    broadcastAddress: IPPortToAddress(broadcast, port), // 子网广播地址
}
```

### 2. WhoIs 广播：无范围限制

发送 WhoIs 时不指定设备 ID 范围（即不加 Context Tag 0 和 Context Tag 1），表示"发现所有设备"：

```go
whoIsOpts := &WhoIsOpts{
    Low:             0,
    High:            0, // 0 表示无范围限制，发现所有设备
    GlobalBroadcast: false,
}
devices, err := client.WhoIs(whoIsOpts)
```

编码层实现：当 `low` 或 `high` 超出 `MaxInstance` 范围时，不编码 range tags，BACnet 标准将此解释为"无范围限制"。

### 3. 多网卡环境的 IP 选择

在多网卡环境中（如 ARM64 设备同时有 eth0、eth1、wlan0），必须选择与目标设备在同一子网的接口。Yabe 允许用户手动选择网络接口，我们的实现通过环境变量 `BACNET_IP` 支持相同功能：

```bash
# 指定使用 wlan0 接口的 IP
BACNET_IP=192.168.3.230 ./bacnet-yabe-test
```

### 4. IAm 响应处理：端口映射

Yabe 模拟器的多个设备共享同一端口（47808），但 IAm 响应中的源端口可能不同（如 56581、61926、61927 等）。因此：

- 不要假设 IAm 响应源端口等于设备配置端口
- 使用 IAm 响应中的实际源地址（IP:Port）作为后续通信的目标地址
- 每个设备独立维护其地址映射

## 测试验证

### 部署环境

- 目标设备：ARM64 Linux（192.168.3.230，wlan0）
- 模拟器设备：Windows（192.168.3.115，运行 Yabe 模拟器）
- 测试工具：Go test 二进制，交叉编译为 ARM64

### 测试用例

| 测试名称 | 目的 | 验证点 |
|---------|------|--------|
| `TestYabeStyleBroadcastOnly` | 纯广播 WhoIs 发现 | 5 台设备全部发现，3 轮一致性验证 |
| `TestYabeStyleWhoIsResponse` | 多轮广播一致性 | 每轮均发现相同的 5 台设备 |
| `TestYabeStyleDiscovery` | 完整链路测试 | 发现→对象扫描→点位读取→可写点写入 |
| `TestYabeStyleMultiDeviceReadWrite` | 多设备读写 | 3 台目标设备读写验证 |

### 测试结果（2026-07-20）

```
Phase 1 广播 WhoIs: ✅ 4/4 设备发现
Phase 2 对象扫描:   ✅ 4/4 成功
Phase 3 点位读取:   ✅ 50/50 成功
Phase 4 可写点写入: ✅ 18/24 成功（6 个只读点跳过）
```

### 已知限制

- Yabe 模拟器的某些 AnalogValue 点在写入后会自动恢复为原值（模拟器内部逻辑控制），这是模拟器行为，非协议栈问题
- 写入验证采用"首次验证通过即视为成功"的策略，二次验证用于检测自动恢复

## 部署流程

```bash
# 1. 交叉编译测试二进制
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o bin/bacnet-yabe-test-arm64 .

# 2. 部署到 ARM64 设备
scp bin/bacnet-yabe-test-arm64 root@192.168.3.230:/tmp/bacnet-yabe-test

# 3. 运行测试
ssh root@192.168.3.230 'BACNET_IP=192.168.3.230 /tmp/bacnet-yabe-test -test.run "TestYabeStyle" -test.v -test.timeout 300s'
```

## 与官方 Yabe 的对齐

| 特性 | 官方 Yabe | 本实现 |
|------|----------|--------|
| Socket 绑定 | INADDR_ANY | INADDR_ANY ✅ |
| 设备发现 | 广播 WhoIs | 广播 WhoIs ✅ |
| 接口选择 | 手动选择 | 环境变量 BACNET_IP ✅ |
| 默认端口 | 47808 | 47808 ✅ |
| IAm 源地址 | 使用实际源地址 | 使用实际源地址 ✅ |
| 对象扫描 | ReadMultiProperty | ReadMultiProperty + 降级 ✅ |
| 写入验证 | 无 | 首次验证 + 500ms 二次验证 ✅ |