# BACnet-Go 房间模拟器（Room Simulator）

对标常见的 `Bacnet.Room.Simulator.exe`（BACnet Room Simulator），在本机启动一个可被 **YABE** 扫描的 BACnet/IP 虚拟设备，预置一组房间相关对象（温度、设定点、占用、模式等）。

实现基于仓库 `server` 包（Who-Is→I-Am、Object_List、Read/WriteProperty、ReadPropertyMultiple、SubscribeCOV）。

---

## 编译与运行

在仓库根目录：

```bash
# 编译
go build -o room-simulator.exe ./cmd/room-simulator

# 同机有 YABE 时：务必用独立端口（推荐）
./room-simulator.exe -ip 192.168.3.115 -subnet 24 -port 47810 -device-id 1234

# 仅当本机 47808 上没有 YABE / 其它 BACnet 程序时，才可用默认端口
./room-simulator.exe -ip 192.168.3.115 -subnet 24 -port 47808 -device-id 1234
```

### 常用参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `-ip` | `0.0.0.0` | 本地绑定 IPv4；**建议显式填写局域网 IP** |
| `-iface` | 空 | 网卡名（与 `-ip` 二选一） |
| `-port` | `47808` | BACnet/IP UDP 端口；**同机 YABE 请改 47810** |
| `-subnet` | `24` | 子网 CIDR（配合 `-ip` 计算广播地址） |
| `-device-id` | `1234` | Device 实例号 |
| `-device-name` | `Room Simulator` | Device Object_Name |
| `-vendor-id` | `999` | Vendor Identifier |
| `-dynamic` | 关闭 | 缓慢变化 Space Temperature，便于看刷新/COV |

直接运行（不编译）：

```bash
# 推荐同机联调命令（避开 YABE 的 47808）
go run ./cmd/room-simulator -ip 192.168.3.115 -subnet 24 -port 47810
```

---

## Windows / YABE 注意点

1. **同网段**：YABE 所在 PC 与模拟器绑定的 IP 须在同一二层网段（或经 BACnet 路由可达）。
2. **多网卡**：本机若有 WLAN、有线、Tailscale、Hyper-V 等，务必用 `-ip` 指定正确网卡地址；不要依赖自动选址。
3. **防火墙**：放行 UDP **你设置的 `-port`**（47808 或 47810）入站/出站。
4. **同机端口冲突（当前最常见根因）**：
   - YABE 自身常占用本机 `192.168.x.x:47808`。
   - 若模拟器也绑同一 `IP:47808`，Windows UDP 可能让两个进程“同时”监听。
   - **症状**：Who-Is / I-Am 偶发成功，但展开 Object_List 失败；YABE 日志出现：
     ```text
     Sending ReadPropertyRequest ...
     ConfirmedServiceRequest
     Sending ErrorResponse ...
     Confirmed service not handled: SERVICE_CONFIRMED_READ_PROPERTY
     Didn't get response from 'Object List'
     ```
   - **含义**：这句话**不是**本仓库 Go 服务端打的。它来自 YABE 使用的 `System.IO.BACnet`（`BACnetClient`）：YABE 自己收到了本该发给模拟器的 ReadProperty，却没有注册 `OnReadPropertyRequest`，于是回 Reject/Error 并打上述日志。本模拟器的对应日志是 `unhandled confirmed service`（且 ReadProperty=12 已注册，不会走这条路径）。
   - **排查**：`netstat -ano | findstr 47808`，若同时看到 `Yabe.exe` 与 `room-simulator.exe`，即冲突。
   - **处理**：停掉旧模拟器后改用独立端口（见下方复测步骤）。本库已改为 **独占绑定**（Windows `SO_EXCLUSIVEADDRUSE` + 关闭 `SO_REUSEADDR`）：再与 YABE 抢同一 `IP:47808` 时会直接报
     `bind ... Only one usage of each socket address ... is normally permitted`，而不会再静默抢包。
5. **广播**：Who-Is 一般为子网广播；绑定错误网卡时 YABE 可能扫不到设备。

---

## Object_List 行为（YABE 扫描）

Device `Object_List`（属性 76）按 BACnetARRAY of Object Identifier 实现：

| 请求 | 响应 |
|------|------|
| 无 array index（整表） | 连续 Application Tag Object Identifier；Device 自身为第 1 项 |
| `array index = 0` | Unsigned：数组长度 |
| `array index = N`（1-based） | 单个 Object Identifier |
| 越界 index | Error：Invalid Array Index |
| 整表 APDU 超过对端 Max-APDU 且本端不分段 | Abort：buffer-overflow（YABE 通常会改走 index 0 + 逐项读） |

### 已验证的编码（硬证据）

历史上 `ReadPropertyAck` 在 opening tag `3`（`0x3E`）之后错误地做了 `tagID++`，closing 写成了 tag `4`（`0x4F`）。本库解码器不校验 closing 编号，所以自测一直通过；**YABE 会拒绝**，表现为 `Didn't get response from 'Object List'`（Who-Is + 部分 ReadProperty 仍可能成功）。

正确形态（ASHRAE 135）：property value 用 **同一** context tag 3 的 opening/closing 包裹；数组元素是连续 **Application** Object Identifier（`0xC4`），**没有**额外的 context array 包裹。

最小 golden（Device 1234，5 个对象，invoke=1）：

```text
30 01 0C                          Complex-ACK, invoke 1, ReadProperty
0C 02 00 04 D2                    context[0] Device,1234
19 4C                             context[1] property 76
3E                                opening tag 3
C4 02 00 04 D2                    Device:1234
C4 00 00 00 01                    AI:1
C4 00 80 00 01                    AV:1
C4 00 C0 00 01                    BI:1
C4 04 C0 00 01                    MSV:1
3F                                closing tag 3   ← 必须是 3F，不能是 4F
```

失败即红测试：`go test ./encoding -run ObjectList_ReadPropertyAck_GoldenBytes -v`  
本地打印 hex：`go run ./cmd/objlist-probe`

服务端分发回归：`go test ./server -run 'ReadProperty_ObjectList_MustComplexAck|UnhandledConfirmedService_MustError' -count=1 -v`

### 若仍失败时的检查项

1. **先看 YABE 日志是不是 “Confirmed service not handled”** → 几乎一定是端口被 YABE 抢走；改 `-port 47810` 并重启（见下节）。
2. 确认正在跑的是**修复后重新编译/重新 go run** 的进程（旧 exe 仍会回 `…3E…4F`，或仍带 SO_REUSEADDR）。
3. Wireshark 过滤 `bacnet`，看 Object_List 响应：应是 **Complex-ACK**（PDU type `0x30`），末字节 `3f`；若是 Reject/`0x60` 或 Error/`0x50` 且来自 YABE 本机栈，则仍是抢包。
4. `netstat` 确认模拟器端口上**只有** `room-simulator`，YABE 只应占用它自己的本地端点（通常仍是 47808）。
5. 对象数很大导致整表超 Max-APDU 时，应看到 Abort；YABE 会改读 index 0 再逐项读（本模拟器 15 项远小于 1476）。

---

## 用 YABE 扫描（同机推荐复测步骤）

1. **停掉**所有旧 `room-simulator` / 官方 `Bacnet.Room.Simulator.exe`：
   ```bash
   # Git Bash / CMD
   netstat -ano | findstr 47808
   netstat -ano | findstr 47810
   # 对 room-simulator 的 PID：taskkill /PID <pid> /F
   # 或在跑模拟器的终端 Ctrl+C
   ```
2. **用独立端口启动**（YABE 继续用默认 47808 作本地端点即可；它会按 I-Am 源端口访问设备）：
   ```bash
   go run ./cmd/room-simulator -ip 192.168.3.115 -subnet 24 -port 47810
   ```
   若仍坚持 47808：先关掉 YABE，或接受 bind 失败（独占绑定）。
3. 打开 YABE → 选择与模拟器同网段的网卡 → **Who-Is**。
4. 期望看到 Device `1234` / `Room Simulator`；展开后应列出 Object_List 中的 AI/AV/BI/BO/BV/MSV。
5. 本库客户端冒烟（不依赖 YABE）：
   ```bash
   go test ./encoding -run 'ObjectList_ReadPropertyAck_GoldenBytes|ObjectList_Index' -count=1 -v
   go test ./server -run 'ReadProperty_ObjectList_MustComplexAck|UnhandledConfirmedService_MustError' -count=1 -v
   go test ./cmd/room-simulator -run TestRoomSimulatorSmoke -count=1 -v
   ```

---

## 预置对象对照表

| 类型 | 实例 | Object_Name | 默认 Present_Value | 说明 |
|------|------|-------------|-------------------|------|
| Device | （`-device-id`） | Room Simulator | — | 设备对象 |
| AnalogInput | 1 | Space Temperature | 22.5 °C | 室内温度 |
| AnalogInput | 2 | Outdoor Temperature | 18.0 °C | 室外温度 |
| AnalogInput | 3 | Humidity | 45.0 %RH | 相对湿度 |
| AnalogInput | 4 | Supply Air Temperature | 14.0 °C | 送风温度 |
| AnalogValue | 1 | Temperature Setpoint | 22.0 °C | 温度设定点（可写） |
| AnalogValue | 2 | Cooling Setpoint | 24.0 °C | 制冷设定点（可写） |
| AnalogValue | 3 | Heating Setpoint | 20.0 °C | 制热设定点（可写） |
| BinaryInput | 1 | Occupancy | 1 (Active) | 占用 |
| BinaryInput | 2 | Window Status | 0 (Inactive) | 窗磁 |
| BinaryOutput | 1 | Fan | 0 | 风机（可写） |
| BinaryOutput | 2 | Light | 0 | 灯光（可写） |
| BinaryValue | 1 | Occupancy Override | 0 | 占用覆盖（可写） |
| MultiStateValue | 1 | HVAC Mode | 4 | 1=Off 2=Heat 3=Cool 4=Auto（可写） |
| MultiStateValue | 2 | Fan Speed | 1 | 1=Off 2=Low 3=Med 4=High（可写） |

YABE 扫描通常依赖：Who-Is → I-Am、Device `Object_List`、各对象 `Object_Name` / `Present_Value`，以及 Device 的 `Object_Name`、`Model_Name` 等。

---

## 与官方 Bacnet.Room.Simulator.exe 的差异

本模拟器是**功能相近的演示设备**，不是官方二进制的协议级复刻。诚实差异包括：

| 项目 | 本模拟器 | 典型官方 Room Simulator |
|------|----------|-------------------------|
| 厂商 / Vendor ID | BACnet-Go / `999`（可改） | Chipkin 等厂商 ID 与品牌信息 |
| 对象集合 | 上表固定 14 个点位 | 版本不同，点位名/数量可能更多或命名略异 |
| GUI | 无图形界面，仅 CLI | 通常带房间/点位图形界面 |
| State_Text | Multi-state 仅提供 `Number_Of_States`，未编码完整 `State_Text` 数组 | 常带状态文本 |
| 协议范围 | 基于本库已实现服务（RP/RPM/WP/WPM/COV、Who-Is/I-Am） | 可能含更多可选服务/对象类型 |
| 动态仿真 | 可选 `-dynamic` 仅缓变 Space Temperature | 可能有更完整的房间热力学/联动逻辑 |
| 分段 / 路由 | 无 APDU 分段；超 Max-APDU 时 Abort(buffer-overflow)，Object_List 支持 index 逐项读 | 视具体版本而定 |

若你需要与某一版官方 exe **逐对象、逐属性完全一致**，请提供该版本的 Object_List 导出或抓包，可再对齐命名与实例号。

---

## 相关文档

- [README_CN.md](README_CN.md) — 协议栈中文说明
- [REAL_DEVICE_TEST.md](REAL_DEVICE_TEST.md) — 真机 / YABE 联调注意点
- `server` 包 — 服务端实现与单元/互通测试
