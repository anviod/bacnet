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

---

## 服务端模式 (Server / 从机)

`server` 包提供了完整的 BACnet/IP 服务端实现。它可以作为虚拟 BACnet 设备运行，自动响应 WhoIs→IAm，处理 ReadProperty、WriteProperty、ReadPropertyMultiple、WritePropertyMultiple 和 SubscribeCOV 请求。

### 快速开始 — 创建 BACnet 服务端

```go
package main

import (
    "log"
    "os"
    "os/signal"

    "github.com/anviod/bacnet/btypes"
    "github.com/anviod/bacnet/server"
)

func main() {
    // 1. 创建服务端配置
    cfg := &server.DeviceConfig{
        DeviceID:   1234,                     // BACnet 设备实例号
        DeviceName: "Room Simulator",         // 设备名称
        VendorID:   999,                      // 厂商标识
        Ip:         "192.168.3.115",          // 绑定 IP（空则 0.0.0.0）
        Port:       47810,                    // BACnet 端口 (默认 47808)
        SubnetCIDR: 24,                       // 子网掩码
        MaxPDU:     btypes.MaxAPDU,           // 最大 PDU (默认 1476)
    }

    // 2. 创建服务端
    srv, err := server.NewServer(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer srv.Close()

    // 3. 添加对象（温度传感器）
    srv.AddObject(btypes.Object{
        ID: btypes.ObjectID{
            Type:     btypes.AnalogInput,
            Instance: 1,
        },
        Properties: []btypes.Property{
            {
                Type:       btypes.PROP_PRESENT_VALUE,
                ArrayIndex: btypes.ArrayAll,
                Data:       float32(22.5),    // 当前温度 22.5°C
            },
            {
                Type:       btypes.PROP_OBJECT_NAME,
                ArrayIndex: btypes.ArrayAll,
                Data:       "Space Temperature",
            },
            {
                Type:       btypes.PROP_UNITS,
                ArrayIndex: btypes.ArrayAll,
                Data:       uint32(62),       // 摄氏度
            },
        },
    })

    // 4. 添加可写对象（温度设定值）
    srv.AddObject(btypes.Object{
        ID: btypes.ObjectID{
            Type:     btypes.AnalogValue,
            Instance: 1,
        },
        Properties: []btypes.Property{
            {
                Type:       btypes.PROP_PRESENT_VALUE,
                ArrayIndex: btypes.ArrayAll,
                Data:       float32(25.0),
            },
            {
                Type:       btypes.PROP_OBJECT_NAME,
                ArrayIndex: btypes.ArrayAll,
                Data:       "Temperature Setpoint",
            },
        },
    })

    // 5. 启动服务端
    go func() {
        log.Println("服务端启动中...")
        if err := srv.Serve(); err != nil {
            log.Printf("服务端错误: %v", err)
        }
    }()

    log.Printf("BACnet 服务端已启动。Device ID: %d, Name: %s", srv.GetDeviceID(), cfg.DeviceName)

    // 6. 动态更新传感器值（可选）
    // go func() {
    //     ticker := time.NewTicker(5 * time.Second)
    //     for range ticker.C {
    //         srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float32(20.0 + rand.Float32()*5.0))
    //     }
    // }()

    // 等待退出信号
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt)
    <-sig
    log.Println("正在停止服务端...")
}
```

### 运行内置房间模拟器

项目内置了一个完整的房间模拟器，可作为从机模式参考实现：

```bash
# 启动房间模拟器（Device ID 1234，端口 47810）
go run ./cmd/room-simulator -ip 192.168.3.115 -subnet 24 -port 47810 -device-id 1234

# 启用动态温度变化（便于观察 COV 通知）
go run ./cmd/room-simulator -ip 192.168.3.115 -subnet 24 -port 47810 -device-id 1234 -dynamic
```

预置对象清单：
| 对象类型 | Instance | 名称 | 可写 |
|----------|:--------:|------|:----:|
| AnalogInput | 1 | Space Temperature | — |
| AnalogInput | 2 | Outdoor Temperature | — |
| AnalogInput | 3 | Humidity | — |
| AnalogInput | 4 | Supply Air Temperature | — |
| AnalogValue | 1 | Temperature Setpoint | ✓ |
| AnalogValue | 2 | Cooling Setpoint | ✓ |
| AnalogValue | 3 | Heating Setpoint | ✓ |
| BinaryInput | 1 | Occupancy | — |
| BinaryInput | 2 | Window Status | — |
| BinaryOutput | 1 | Fan | ✓ |
| BinaryOutput | 2 | Light | ✓ |
| BinaryValue | 1 | Occupancy Override | ✓ |
| MultiStateValue | 1 | HVAC Mode (Off/Heat/Cool/Auto) | ✓ |
| MultiStateValue | 2 | Fan Speed (Off/Low/Med/High) | ✓ |

### 对象管理

```go
// 添加对象（自动填充默认属性）
srv.AddObject(btypes.Object{
    ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
})
// 自动创建: ObjectIdentifier, ObjectName, ObjectType,
//           PresentValue, StatusFlags, EventState, Reliability, OutOfService, Units

// 获取对象
obj, found := srv.GetObject(btypes.AnalogInput, 1)

// 移除对象
srv.RemoveObject(btypes.AnalogInput, 2)

// 设置属性值（自动触发 COV 通知）
srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float32(26.5))

// 获取属性值
value, found := srv.GetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE)
```

### 客户端 ↔ 服务端 完整交互示例

```go
// ═══════════ 服务端 ═══════════
srv, _ := server.NewServer(&server.DeviceConfig{
    DeviceID:   1000,
    DeviceName: "TestServer",
    VendorID:   999,
    Ip:         "0.0.0.0",
    Port:       47808,
    SubnetCIDR: 24,
})
go srv.Serve()
defer srv.Close()

// 添加模拟输入对象
srv.AddObject(btypes.Object{
    ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
    Properties: []btypes.Property{
        {Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float32(42.5)},
        {Type: btypes.PROP_OBJECT_NAME, ArrayIndex: btypes.ArrayAll, Data: "Temperature"},
    },
})

// ═══════════ 客户端 ═══════════
client, _ := bacnet.NewClient(&bacnet.ClientBuilder{
    Ip:         "192.168.1.100",
    SubnetCIDR: 24,
    Port:       47808,
})
go client.ClientRun()
defer client.Close()

// 发现设备
devices, _ := client.WhoIs(&bacnet.WhoIsOpts{Low: 0, High: 4194304})
// → 返回 Device{DeviceID: 1000, ...}

// 扫描对象
scanned, _ := client.Objects(devices[0])
// → scanned.Objects[AnalogInput][1].Name == "Temperature"

// 读取属性
result, _ := client.ReadProperty(devices[0], btypes.PropertyData{
    Object: btypes.Object{
        ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
        Properties: []btypes.Property{
            {Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
        },
    },
})
// → result.Object.Properties[0].Data == float32(42.5)

// 写入属性
client.WriteProperty(devices[0], btypes.PropertyData{
    Object: btypes.Object{
        ID: btypes.ObjectID{Type: btypes.AnalogValue, Instance: 1},
        Properties: []btypes.Property{
            {
                Type:       btypes.PROP_PRESENT_VALUE,
                ArrayIndex: btypes.ArrayAll,
                Data:       float32(267.0),
                Priority:   btypes.Normal,
            },
        },
    },
})
// → 服务端收到 SimpleAck，Present_Value 更新为 267.0
```

### COV (Change of Value) 订阅

```go
// 客户端订阅 COV 通知
err := client.SubscribeCOV(device, btypes.SubscribeCOVData{
    ProcessID:      1,
    ObjectID:       btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
    IssueConfirmed: false,      // false = UnconfirmedCOVNotification
    Lifetime:       0,          // 0 = 永久订阅
})

// 等待 COV 通知
notification, err := client.WaitCOVNotification(1, 30*time.Second)
// notification.MonitoredObjectIdentifier → 被监控的对象
// notification.ListOfValues[0].Value → 新值

// 取消订阅
err = client.CancelSubscribeCOV(device, 1, btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1})
```

### 服务端支持的确认服务

| 服务 | 描述 | 说明 |
|------|------|------|
| ReadProperty | 读取单个属性 | 支持 ObjectList 数组索引 |
| WriteProperty | 写入单个属性 | 自动触发 COV 通知 |
| ReadPropertyMultiple | 批量读取 | 支持 PROP_ALL/REQUIRED/OPTIONAL 展开 |
| WritePropertyMultiple | 批量写入 | 逐个处理，遇错继续 |
| SubscribeCOV | COV 订阅 | 支持带生命周期的订阅管理 |

### 服务端错误响应

| 错误条件 | 错误类 | 错误码 |
|----------|--------|--------|
| 未知对象 | ObjectError | UnknownObject |
| 未知属性 | PropertyError | UnknownProperty |
| 写入被拒绝 | PropertyError | WriteAccessDenied |
| 无效标签 | ServicesError | InvalidTag |
| 缺少参数 | ServicesError | MissingRequiredParameter |
| 不支持的服务 | ServicesError | ServiceRequestDenied |

### 服务端 Device 对象自动属性

服务端自动维护标准 BACnet Device 对象属性：

| 属性 | 值 |
|------|-----|
| ObjectIdentifier | 配置的 DeviceID |
| ObjectName | 配置的 DeviceName |
| VendorIdentifier | 配置的 VendorID |
| VendorName | "BACnet-Go" |
| ModelName | "BACnet-Go Server" |
| FirmwareRevision | "1.0.0" |
| ProtocolVersion | 1 |
| ProtocolRevision | 24 |
| ProtocolServicesSupported | BitString (ReadProperty, WriteProperty 等) |
| ProtocolObjectTypesSupported | BitString (AI, AO, AV, BI, BO, BV 等) |
| MaxAPDUAccepted | 1476 |
| DatabaseRevision | 每次修改自动递增 |

---

## API 参考

### 客户端接口 (Client)

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

    // COV 订阅
    SubscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error
    CancelSubscribeCOV(device btypes.Device, processID uint32, objectID btypes.ObjectID) error
    WaitCOVNotification(processIDFilter int64, timeout time.Duration) (btypes.COVNotification, error)
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

### 服务端接口 (Server)

```go
type Server interface {
    // 生命周期
    Serve() error
    Close() error
    IsRunning() bool

    // 对象管理
    AddObject(obj btypes.Object) error
    RemoveObject(objType btypes.ObjectType, instance btypes.ObjectInstance) error
    GetObject(objType btypes.ObjectType, instance btypes.ObjectInstance) (*btypes.Object, bool)
    SetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType, data interface{}) error
    GetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType) (interface{}, bool)
    GetObjectStore() *ObjectStore
    GetDeviceID() btypes.ObjectInstance
}
```

### DeviceConfig 配置

```go
type DeviceConfig struct {
    DeviceID      btypes.ObjectInstance          // 设备实例号 (0-4194304)
    DeviceName    string                         // BACnet 设备名称
    VendorID      uint32                         // 厂商标识
    Interface     string                         // 网卡名（如 "eth0"）
    Ip            string                         // 绑定 IP 地址
    Port          int                            // BACnet 端口 (默认: 47808)
    SubnetCIDR    int                            // 子网 CIDR (如 24 表示 /24)
    MaxPDU        uint16                         // 最大 PDU 大小
    MaxSegments   uint                           // 最大分段数
    Segmentation  segmentation.SegmentedType     // 分段支持类型
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

4. **Windows SO_BROADCAST**（v0.0.6+）：
   - Windows 要求 UDP 套接字显式开启 `SO_BROADCAST` 才能发送广播包
   - 本库 `datalink` 包已在 Windows 构建中自动设置，无需用户干预
   - 若自行实现底层 UDP 绑定，务必同时设置 `SO_REUSEADDR` 和 `SO_BROADCAST`

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