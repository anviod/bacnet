## 文档

- [英文文档](README.md)
- [中文文档](README_CN.md) (本文档)
- [房间模拟器 Room Simulator（YABE 扫描）](ROOM_SIMULATOR.md)

# BACnet 协议栈

一个用 Go 语言实现的 BACnet/IP 协议栈，用于楼宇自动化和控制系统。

## 功能特性

- **BACnet/IP 协议**：完整支持 BACnet/IP 通信
- **设备发现**：Who-Is 和 I-Am 服务用于网络设备发现
- **对象访问**：ReadProperty、ReadMultipleProperty、WriteProperty、WriteMultipleProperty
- **网络管理**：What-Is-Network-Number、Who-Is-Router-To-Network
- **事务管理**：TSM（事务状态机）用于确认服务
- **并发安全**：线程安全设计，支持连接池
- **房间模拟器**：`cmd/room-simulator`，便于 YABE 扫描的虚拟房间设备（见 [ROOM_SIMULATOR.md](ROOM_SIMULATOR.md)）

## 安装

```bash
go get github.com/anviod/bacnet
```

边缘 / 交叉编译（静态、禁用 cgo）：`./scripts/cross-build.sh` 或 `make cross`，例如 `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./...`（ARMv7+）。

## 快速开始

### 基础设备发现

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/anviod/bacnet"
    "github.com/anviod/bacnet/btypes"
)

func main() {
    // 创建 BACnet 客户端
    client, err := bacnet.NewClient(&bacnet.ClientBuilder{
        Ip:         "192.168.1.100",
        SubnetCIDR: 24,
        Port:       47808, // 默认 BACnet 端口 (0xBAC0)
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // 启动客户端消息循环
    go client.ClientRun()

    // 发现网络中的所有设备
    devices, err := client.WhoIs(&bacnet.WhoIsOpts{
        Low:  0,
        High: 4194304, // BACnet 最大设备 ID
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("发现 %d 个设备\n", len(devices))
    for _, dev := range devices {
        fmt.Printf("设备 ID: %d, IP: %s:%d\n", dev.DeviceID, dev.Ip, dev.Port)
    }
}
```

---

## 数据采集流程

BACnet 数据采集过程包含六个关键步骤：

### 步骤 1：客户端初始化

在进行任何通信之前，必须使用适当的网络配置创建 BACnet 客户端。

```go
client, err := bacnet.NewClient(&bacnet.ClientBuilder{
    Ip:         "192.168.1.100",  // 本地 IP 地址
    SubnetCIDR: 24,                // 子网掩码（如 /24）
    Port:       47808,             // BACnet 端口（默认：47808）
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**配置选项：**
- `Ip`：要绑定的本地 IP 地址
- `Interface`：网络接口名称（替代 Ip）
- `SubnetCIDR`：子网 CIDR 表示法（例如，24 表示 /24）
- `Port`：BACnet UDP 端口（默认：47808 = 0xBAC0）
- `MaxPDU`：最大 PDU 大小（默认：1476）

### 步骤 2：启动消息循环

必须在 goroutine 中启动客户端消息循环来处理传入消息：

```go
go client.ClientRun()
```

**重要注意事项：**
- 必须在进行任何请求之前调用
- 持续运行直到客户端关闭
- 处理消息解码和路由

### 步骤 3：设备发现（WhoIs）

使用 WhoIs 服务发现网络上的 BACnet 设备：

```go
devices, err := client.WhoIs(&bacnet.WhoIsOpts{
    Low:  0,             // 设备 ID 下限
    High: 4194304,       // 设备 ID 上限（最大值）
})
```

**发现选项：**
- `Low`：设备 ID 范围的下限（0 到 4194304）
- `High`：设备 ID 范围的上限
- `GlobalBroadcast`：使用全局广播地址（0xFFFF）
- `Destination`：单播发现的特定目标地址

**最佳实践：**
- 使用窄 ID 范围进行目标发现以减少网络流量
- 在大型网络上避免使用全范围（0-4194304）
- 缓存发现的设备以避免重复发现

### 步骤 4：对象发现

从发现的设备检索所有对象：

```go
scannedDevice, err := client.Objects(devices[0])
if err != nil {
    log.Printf("扫描对象失败: %v", err)
    return
}

// 访问特定对象类型
aiObjects := scannedDevice.Objects[btypes.AnalogInput]
biObjects := scannedDevice.Objects[btypes.BinaryInput]
aoObjects := scannedDevice.Objects[btypes.AnalogOutput]
boObjects := scannedDevice.Objects[btypes.BinaryOutput]
```

**支持的对象类型：**
- `AnalogInput` (0)：模拟输入点（如温度传感器）
- `AnalogOutput` (1)：模拟输出点（如阀门、风门）
- `AnalogValue` (2)：模拟值对象
- `BinaryInput` (3)：二进制输入点（如触点传感器）
- `BinaryOutput` (4)：二进制输出点（如继电器）
- `BinaryValue` (5)：二进制值对象
- `Device` (8)：BACnet 设备对象
- `MultiStateInput` (13)：多状态输入点
- `MultiStateOutput` (14)：多状态输出点
- `TrendLog` (20)：趋势日志对象

### 步骤 5：数据读取

从设备对象读取属性值。

#### 读取单个属性

```go
result, err := client.ReadProperty(device, btypes.PropertyData{
    Object: btypes.Object{
        ID: btypes.ObjectID{
            Type:     btypes.AnalogInput,
            Instance: 1,
        },
        Properties: []btypes.Property{
            {
                Type:       btypes.PropPresentValue,
                ArrayIndex: btypes.ArrayAll,
            },
        },
    },
})
```

#### 批量读取多个属性

为了获得更好的性能，使用 ReadMultiProperty 在一个请求中读取多个属性：

```go
result, err := client.ReadMultiProperty(device, btypes.MultiplePropertyData{
    Objects: []btypes.Object{
        {
            ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
            Properties: []btypes.Property{
                {Type: btypes.PropPresentValue},
                {Type: btypes.PropUnits},
                {Type: btypes.PropDescription},
            },
        },
        {
            ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
            Properties: []btypes.Property{
                {Type: btypes.PropPresentValue},
            },
        },
    },
})
```

**常用属性：**
- `PropPresentValue` (85)：对象的当前值
- `PropUnits` (117)：工程单位
- `PropDescription` (28)：对象描述
- `PropObjectName` (77)：对象名称
- `PropObjectType` (79)：对象类型
- `PropObjectIdentifier` (75)：对象标识符
- `PropObjectList` (76)：设备中的对象列表

### 步骤 6：数据写入

向设备对象写入值。

```go
err := client.WriteProperty(device, btypes.PropertyData{
    Object: btypes.Object{
        ID: btypes.ObjectID{
            Type:     btypes.AnalogOutput,
            Instance: 1,
        },
        Properties: []btypes.Property{
            {
                Type:       btypes.PropPresentValue,
                ArrayIndex: btypes.ArrayAll,
                Data:       float64(25.5),
                Priority:   btypes.Normal,
            },
        },
    },
})
```

**写入优先级级别：**
- `LifeSafety` (3)：生命安全操作
- `CriticalEquipment` (2)：关键设备控制
- `Urgent` (1)：紧急操作
- `Normal` (0)：正常操作

---

## 高级用法

### 完整集成流程

```go
func completeIntegration(client bacnet.Client) error {
    // 步骤 1：发现设备
    devices, err := client.WhoIs(&bacnet.WhoIsOpts{
        Low:  0,
        High: 4194304,
    })
    if err != nil {
        return fmt.Errorf("WhoIs 失败: %v", err)
    }
    if len(devices) == 0 {
        return fmt.Errorf("未发现设备")
    }

    device := devices[0]
    fmt.Printf("发现设备: ID=%d, IP=%s:%d\n", device.DeviceID, device.Ip, device.Port)

    // 步骤 2：扫描对象
    scannedDevice, err := client.Objects(device)
    if err != nil {
        return fmt.Errorf("对象扫描失败: %v", err)
    }

    // 步骤 3：查找目标点位
    aiObjects := scannedDevice.Objects[btypes.AnalogInput]
    targetPoint, ok := aiObjects[1]
    if !ok {
        return fmt.Errorf("未找到目标点位")
    }
    fmt.Printf("发现目标点位: %s\n", targetPoint.Name)

    // 步骤 4：读取当前值
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogInput,
                Instance: 1,
            },
            Properties: []btypes.Property{
                {Type: btypes.PropPresentValue},
            },
        },
    })
    if err != nil {
        return fmt.Errorf("读取属性失败: %v", err)
    }
    fmt.Printf("当前值: %v\n", result.Object.Properties[0].Data)

    // 步骤 5：写入 AnalogValue
    writeErr := client.WriteProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogValue,
                Instance: 1,
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue,
                    ArrayIndex: btypes.ArrayAll,
                    Data:       float64(25.5),
                    Priority:   btypes.Normal,
                },
            },
        },
    })
    if writeErr != nil {
        return fmt.Errorf("写入属性失败: %v", writeErr)
    }
    fmt.Println("写入成功")

    return nil
}
```

### 带超时的读取

使用超时变体更好地控制请求时序：

```go
result, err := client.ReadPropertyWithTimeout(device, propertyData, 5*time.Second)
```

### 错误处理模式

```go
func safeReadProperty(client bacnet.Client, device btypes.Device, objID btypes.ObjectID) (interface{}, error) {
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: objID,
            Properties: []btypes.Property{
                {Type: btypes.PropPresentValue},
            },
        },
    })
    
    if err != nil {
        // 处理特定错误类型
        if strings.Contains(err.Error(), "timeout") {
            return nil, fmt.Errorf("设备 %d 未响应", device.DeviceID)
        }
        if strings.Contains(err.Error(), "no such object") {
            return nil, fmt.Errorf("对象 %s 未找到", objID.Type)
        }
        return nil, err
    }
    
    if len(result.Object.Properties) == 0 {
        return nil, fmt.Errorf("未返回属性")
    }
    
    return result.Object.Properties[0].Data, nil
}
```

---

## API 参考

### 客户端接口

```go
type Client interface {
    io.Closer
    IsRunning() bool
    ClientRun()
    
    // 设备发现
    WhoIs(wh *WhoIsOpts) ([]btypes.Device, error)
    IAm(dest btypes.Address, iam btypes.IAm) error
    
    // 网络管理
    WhatIsNetworkNumber() []*btypes.Address
    WhoIsRouterToNetwork() (resp *[]btypes.Address)
    
    // 对象访问
    Objects(dev btypes.Device) (btypes.Device, error)
    ReadProperty(dest btypes.Device, rp btypes.PropertyData) (btypes.PropertyData, error)
    ReadMultiProperty(dev btypes.Device, rp btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error)
    WriteProperty(dest btypes.Device, wp btypes.PropertyData) error
    WriteMultiProperty(dev btypes.Device, wp btypes.MultiplePropertyData) error
    
    // 带超时的变体
    ReadPropertyWithTimeout(dest btypes.Device, rp btypes.PropertyData, timeout time.Duration) (btypes.PropertyData, error)
    ReadMultiPropertyWithTimeout(dev btypes.Device, rp btypes.MultiplePropertyData, timeout time.Duration) (btypes.MultiplePropertyData, error)
}
```

### WhoIs 选项

```go
type WhoIsOpts struct {
    Low             int             // 设备 ID 下限 (0 到 4194304)
    High            int             // 设备 ID 上限
    GlobalBroadcast bool            // 使用全局广播 (0xFFFF)
    NetworkNumber   uint16          // 目标网络号
    Destination     *btypes.Address // 特定目标地址（可选）
}
```

---

## 配置

### ClientBuilder 选项

```go
type ClientBuilder struct {
    DataLink   datalink.DataLink // 自定义数据链路（可选）
    Interface  string            // 网络接口名（如 "eth0"）
    Ip         string            // IP 地址
    Port       int               // BACnet 端口（默认：47808）
    SubnetCIDR int               // 子网 CIDR（如 24 表示 /24）
    MaxPDU     uint16            // 最大 PDU 大小（默认：1476）
}
```

### 常量

```go
// 协议
const DefaultPort = 0xBAC0 // 47808
const MaxAPDU = 1476

// 网络
const GlobalBroadcast = 0xFFFF
const DefaultHopCount = 255

// 优先级
const (
    LifeSafety        = 3  // 生命安全
    CriticalEquipment = 2  // 关键设备
    Urgent            = 1  // 紧急
    Normal            = 0  // 正常
)
```

---

## 最佳实践与建议

### 网络注意事项

1. **端口绑定**：
   - 默认 BACnet 端口是 47808 (0xBAC0)
   - 测试时使用不同端口避免冲突
   - 绑定到 `0.0.0.0` 监听所有接口

2. **IP 地址绑定**：
   - 避免绑定到目标设备的 IP 地址
   - 对于多子网环境，正确配置子网 CIDR

3. **广播行为**：
   - WhoIs 默认使用广播
   - 使用 `Destination` 进行单播请求
   - 广播可能无法跨 VLAN 或子网工作

### 性能优化

1. **批量操作**：
   - 使用 `ReadMultiProperty` 读取多个属性
   - 减少网络往返次数
   - 根据设备的 MaxAPDU 设置限制批处理大小

2. **并发**：
   - 客户端支持并发操作的线程安全设计
   - TSM 限制并发确认事务数（默认：10）
   - 考虑对高频操作进行速率限制

3. **内存管理**：
   - 使用缓冲池提高内存使用效率
   - 使用 `client.Close()` 及时释放资源

### 错误处理

1. **超时处理**：
   - 使用 `ReadPropertyWithTimeout` 进行显式超时控制
   - 确认服务包含带指数退避的重试逻辑
   - 为关键操作实现应用级重试

2. **常见错误**：
   - `timeout`：设备未在超时时间内响应
   - `invalid argument`：无效的对象类型或属性 ID
   - `no such object`：请求的对象不存在
   - `access denied`：写入操作权限不足

---

## 常见问题与故障排除

### 问题 1：未发现设备

**可能原因：**
- IP 地址或子网配置错误
- 防火墙阻止了 BACnet 端口（47808）
- 设备在不同的 VLAN/子网
- 客户端未运行（`ClientRun()` 未调用）

**解决方案：**
- 验证网络配置
- 检查防火墙规则
- 使用 Wireshark 监控 BACnet 流量
- 确保在 `WhoIs()` 之前调用 `ClientRun()`

### 问题 2：ReadProperty 超时失败

**可能原因：**
- 设备未响应
- 设备地址不正确
- 网络连接问题
- 设备繁忙或过载

**解决方案：**
- 通过 ping 验证设备可达性
- 检查设备地址（某些设备使用不同端口进行确认服务）
- 增加超时值
- 实现重试逻辑

### 问题 3：WriteProperty 返回 "Access Denied"

**可能原因：**
- 权限不足
- 设备上启用了写保护
- 优先级级别不正确

**解决方案：**
- 检查设备配置中的写权限
- 验证优先级级别（使用适当的优先级）
- 联系设备制造商获取访问权限

### 问题 4：网络流量过高

**可能原因：**
- 使用全 ID 范围的频繁 WhoIs 请求
- 批量操作超过 MTU
- 广播风暴

**解决方案：**
- 使用窄 ID 范围的目标 WhoIs
- 将批处理大小限制在 MaxAPDU 范围内
- 实现设备发现缓存

---

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定测试
go test -v ./network/...

# 运行验收测试
go test -v -run Acceptance
```

## 许可证

MIT 许可证

## 参考

- [ANSI/ASHRAE Standard 135-2020](https://www.ashrae.org/standards-research/standards/ashrae-standard-135)
- [BACnet 协议规范](http://www.bacnet.org/)