## 文档

- [英文文档](README.md)
- [中文文档](README_CN.md) (本文档)


# BACnet 协议栈

一个用 Go 语言实现的 BACnet/IP 协议栈，用于楼宇自动化和控制系统。

## 功能特性

- **BACnet/IP 协议**：完整支持 BACnet/IP 通信
- **设备发现**：Who-Is 和 I-Am 服务用于网络设备发现
- **对象访问**：ReadProperty、ReadMultipleProperty、WriteProperty、WriteMultipleProperty
- **网络管理**：What-Is-Network-Number、Who-Is-Router-To-Network
- **事务管理**：TSM（事务状态机）用于确认服务
- **并发安全**：线程安全设计，支持连接池

## 安装

```bash
go get github.com/anviod/bacnet
```

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

### 读取模拟输入点位 (AI)

```go
// 读取模拟输入点位的当前值 (AI-1)
func readAnalogInput(client bacnet.Client, device btypes.Device) {
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogInput, // 对象类型: 模拟输入
                Instance: 1,                  // 点位编号: AI-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue, // 读取当前值
                    ArrayIndex: btypes.ArrayAll,
                },
            },
        },
    })
    if err != nil {
        log.Printf("读取 AI-1 失败: %v", err)
        return
    }

    // 获取值
    if len(result.Object.Properties) > 0 {
        fmt.Printf("AI-1 当前值: %v\n", result.Object.Properties[0].Data)
    }
}
```

### 读取二进制输入点位 (BI)

```go
// 读取二进制输入点位的当前值 (BI-1)
func readBinaryInput(client bacnet.Client, device btypes.Device) {
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.BinaryInput, // 对象类型: 二进制输入
                Instance: 1,                  // 点位编号: BI-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue,
                    ArrayIndex: btypes.ArrayAll,
                },
            },
        },
    })
    if err != nil {
        log.Printf("读取 BI-1 失败: %v", err)
        return
    }

    if len(result.Object.Properties) > 0 {
        value := result.Object.Properties[0].Data
        state := "OFF"
        if value == true || value == uint8(1) {
            state = "ON"
        }
        fmt.Printf("BI-1 当前值: %s (%v)\n", state, value)
    }
}
```

### 写入模拟输出点位 (AO)

```go
// 向模拟输出点位写入值 (AO-1)
func writeAnalogOutput(client bacnet.Client, device btypes.Device, value float64) error {
    err := client.WriteProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogOutput, // 对象类型: 模拟输出
                Instance: 1,                   // 点位编号: AO-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue,
                    ArrayIndex: btypes.ArrayAll,
                    Data:       value,             // 要写入的值 (如 25.5)
                    Priority:   btypes.Normal,    // 优先级
                },
            },
        },
    })
    if err != nil {
        log.Printf("写入 AO-1 失败: %v", err)
        return err
    }

    fmt.Printf("成功写入 %.2f 到 AO-1\n", value)
    return nil
}
```

### 写入二进制输出点位 (BO)

```go
// 向二进制输出点位写入值 (BO-1)
func writeBinaryOutput(client bacnet.Client, device btypes.Device, value bool) error {
    err := client.WriteProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.BinaryOutput, // 对象类型: 二进制输出
                Instance: 1,                   // 点位编号: BO-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue,
                    ArrayIndex: btypes.ArrayAll,
                    Data:       value,           // true = ON, false = OFF
                    Priority:   btypes.Normal,   // 优先级
                },
            },
        },
    })
    if err != nil {
        log.Printf("写入 BO-1 失败: %v", err)
        return err
    }

    fmt.Printf("成功写入 %v 到 BO-1\n", value)
    return nil
}
```

### 批量读取多个点位属性

```go
// 一次请求读取多个对象的多个属性
func readMultiplePoints(client bacnet.Client, device btypes.Device) {
    result, err := client.ReadMultiProperty(device, btypes.MultiplePropertyData{
        Objects: []btypes.Object{
            // 读取 AI-1 的当前值和单位
            {
                ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
                Properties: []btypes.Property{
                    {Type: btypes.PropPresentValue},
                    {Type: btypes.PropUnits},
                },
            },
            // 读取 AI-2 的当前值
            {
                ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
                Properties: []btypes.Property{
                    {Type: btypes.PropPresentValue},
                },
            },
            // 读取 BI-1 的当前值
            {
                ID: btypes.ObjectID{Type: btypes.BinaryInput, Instance: 1},
                Properties: []btypes.Property{
                    {Type: btypes.PropPresentValue},
                },
            },
        },
    })
    if err != nil {
        log.Printf("批量读取属性失败: %v", err)
        return
    }

    // 处理结果
    for _, obj := range result.Objects {
        fmt.Printf("对象: %s-%d\n", obj.ID.Type, obj.ID.Instance)
        for _, prop := range obj.Properties {
            fmt.Printf("  %s: %v\n", prop.Type, prop.Data)
        }
    }
}
```

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
    Destination     *btypes.Address // 特定目标地址 (可选)
}
```

### 属性读取

```go
// 从设备读取单个属性
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

// 从多个对象读取多个属性
result, err := client.ReadMultiProperty(device, btypes.MultiplePropertyData{
    Objects: []btypes.Object{
        {
            ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
            Properties: []btypes.Property{
                {Type: btypes.PropPresentValue},
                {Type: btypes.PropUnits},
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

### 属性写入

```go
// 向设备写入属性
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

## 内部架构与调用流程

### 1. 客户端初始化流程

```
┌─────────────────────────────────────────────────────────────────┐
│                     NewClient()                                 │
├─────────────────────────────────────────────────────────────────┤
│  1. 验证 IP 地址                                                │
│     └─ validation.ValidIP(ip)                                   │
│                                                                 │
│  2. 验证端口 (默认: 47808)                                      │
│     └─ validation.ValidPort(port)                               │
│                                                                 │
│  3. 创建数据链路层                                              │
│     ├─ NewUDPDataLink(iface, port)         // 通过接口名        │
│     └─ NewUDPDataLinkFromIP(ip, subnet, port) // 通过 IP        │
│                                                                 │
│  4. 初始化 TSM (事务状态机)                                     │
│     └─ tsm.New(defaultStateSize)                                │
│                                                                 │
│  5. 初始化 UTSM (非确认事务管理器)                              │
│     └─ utsm.NewManager(...)                                     │
│                                                                 │
│  6. 创建缓冲区池                                                │
│     └─ sync.Pool 用于接收缓冲区                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 2. WhoIs 设备发现流程

```
┌─────────────────────────────────────────────────────────────────┐
│                     WhoIs()                                     │
├─────────────────────────────────────────────────────────────────┤
│  1. 确定广播目标地址                                            │
│     ├─ GetBroadcastAddress()                                    │
│     └─ 如果提供了自定义目标则覆盖                               │
│                                                                 │
│  2. 编码 NPDU (网络层协议数据单元)                              │
│     ├─ Version: 1                                               │
│     ├─ Destination: 广播地址                                    │
│     ├─ Source: 本地地址                                         │
│     └─ ExpectingReply: false (广播)                             │
│                                                                 │
│  3. 编码 WhoIs 服务数据                                         │
│     ├─ 低设备 ID                                                │
│     └─ 高设备 ID                                                │
│                                                                 │
│  4. 订阅 UTSM 接收 IAm 响应                                     │
│     └─ utsm.Subscribe(start, end)                               │
│                                                                 │
│  5. 异步发送广播请求                                            │
│     └─ go c.Send(dest, npdu, data, nil)                         │
│                                                                 │
│  6. 收集并去重响应                                              │
│     ├─ 过滤 IAm 响应                                            │
│     ├─ 按设备实例 ID 去重                                       │
│     └─ 构建设备列表                                             │
└─────────────────────────────────────────────────────────────────┘
```

### 3. ReadProperty 流程 (确认服务)

```
┌─────────────────────────────────────────────────────────────────┐
│                     ReadProperty()                              │
├─────────────────────────────────────────────────────────────────┤
│  1. 从 TSM 获取事务 ID                                          │
│     └─ tsm.ID(ctx)                                              │
│                                                                 │
│  2. 构建 NPDU                                                   │
│     ├─ Destination: 设备地址                                    │
│     ├─ Source: 本地地址                                         │
│     └─ ExpectingReply: true                                     │
│                                                                 │
│  3. 编码 APDU (确认服务请求)                                    │
│     ├─ DataType: ConfirmedServiceRequest                        │
│     ├─ Service: ServiceConfirmedReadProperty                    │
│     ├─ InvokeId: 事务 ID                                        │
│     └─ Service Data: 对象 ID + 属性 ID                          │
│                                                                 │
│  4. 发送请求并重试                                              │
│     ├─ c.Send(dest, npdu, data, nil)                            │
│     ├─ tsm.Receive(id, timeout)                                 │
│     └─ 最多重试 retryCount 次                                   │
│                                                                 │
│  5. 解码响应                                                    │
│     ├─ 解码 APDU 头部                                           │
│     ├─ 检查错误                                                 │
│     └─ 解码属性值                                               │
│                                                                 │
│  6. 释放事务 ID                                                 │
│     └─ tsm.Put(id)                                              │
└─────────────────────────────────────────────────────────────────┘
```

### 4. WriteProperty 流程 (确认服务)

```
┌─────────────────────────────────────────────────────────────────┐
│                     WriteProperty()                             │
├─────────────────────────────────────────────────────────────────┤
│  1. 从 TSM 获取事务 ID                                          │
│     └─ tsm.ID(ctx)                                              │
│                                                                 │
│  2. 构建 NPDU                                                   │
│     ├─ Destination: 设备地址                                    │
│     ├─ Source: 本地地址                                         │
│     └─ ExpectingReply: true                                     │
│                                                                 │
│  3. 编码 APDU (确认服务请求)                                    │
│     ├─ DataType: ConfirmedServiceRequest                        │
│     ├─ Service: ServiceConfirmedWriteProperty                   │
│     ├─ InvokeId: 事务 ID                                        │
│     └─ Service Data: 对象 ID + 属性 ID + 值                     │
│                                                                 │
│  4. 发送请求并重试                                              │
│     ├─ c.Send(dest, npdu, data, nil)                            │
│     ├─ tsm.Receive(id, timeout)                                 │
│     └─ 最多重试 2 次                                            │
│                                                                 │
│  5. 解码响应                                                    │
│     ├─ 解码 APDU 头部                                           │
│     ├─ 检查 SimpleAck (成功)                                    │
│     └─ 检查 Error PDU                                           │
│                                                                 │
│  6. 释放事务 ID                                                 │
│     └─ tsm.Put(id)                                              │
└─────────────────────────────────────────────────────────────────┘
```

### 5. 消息接收流程

```
┌─────────────────────────────────────────────────────────────────┐
│                     ClientRun()                                 │
├─────────────────────────────────────────────────────────────────┤
│  循环:                                                          │
│  1. 从池中获取缓冲区                                            │
│     └─ readBufferPool.Get()                                     │
│                                                                 │
│  2. 从数据链路层接收数据                                        │
│     └─ dataLink.Receive(buffer)                                 │
│                                                                 │
│  3. 并发处理消息                                                │
│     └─ go handleMsg(addr, data)                                 │
└─────────────────────────────────────────────────────────────────┘
```

```
┌─────────────────────────────────────────────────────────────────┐
│                     handleMsg()                                 │
├─────────────────────────────────────────────────────────────────┤
│  1. 解码 BVLC (BACnet 虚拟链路控制)                             │
│     ├─ Type: BVLCTypeBacnetIP                                   │
│     ├─ Function: Broadcast/Unicast/ForwardedNPDU                │
│     └─ Length: 数据包长度                                       │
│                                                                 │
│  2. 解码 NPDU                                                   │
│     ├─ Version                                                  │
│     ├─ 源/目标地址                                              │
│     └─ 网络层消息处理                                           │
│                                                                 │
│  3. 解码 APDU 并路由到处理器                                    │
│     ├─ UnconfirmedServiceRequest                                │
│     │   ├─ IAm → utsm.Publish()                                 │
│     │   └─ WhoIs → (忽略或响应)                                 │
│     ├─ SimpleAck → tsm.Send()                                   │
│     ├─ ComplexAck → tsm.Send()                                  │
│     ├─ ConfirmedServiceRequest → tsm.Send()                     │
│     └─ Error → tsm.Send(error)                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 6. 事务状态机 (TSM)

```
┌─────────────────────────────────────────────────────────────────┐
│                     TSM 架构                                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │
│   │   调用者    │────▶│  TSM.ID()   │────▶│   状态      │      │
│   │  (请求)     │     │  (获取 ID)  │     │  (活动)     │      │
│   └─────────────┘     └─────────────┘     └──────┬──────┘      │
│                                                  │              │
│                                                  ▼              │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │
│   │   调用者    │◀────│ TSM.Put()   │◀────│   状态      │      │
│   │  (清理)     │     │ (释放)      │     │ (完成)      │      │
│   └─────────────┘     └─────────────┘     └─────────────┘      │
│                                                  ▲              │
│                                                  │              │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │
│   │   handleMsg │────▶│ TSM.Send()  │────▶│   数据      │      │
│   │  (响应)     │     │ (传递)      │     │  通道       │      │
│   └─────────────┘     └─────────────┘     └─────────────┘      │
│                                                                 │
│  关键组件:                                                      │
│  - states: map[int]*state (活动事务)                            │
│  - free.id: channel (可用调用 ID 1-254)                         │
│  - free.space: channel (并发事务限制)                           │
│  - pool: sync.Pool (状态对象复用)                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 7. 协议栈分层

```
┌─────────────────────────────────────────────────────────────────┐
│                    应用层                                       │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  服务: WhoIs, IAm, ReadProperty, WriteProperty          │   │
│  │  对象: Device, AnalogInput, BinaryOutput, 等            │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                   表示层                                        │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  编码器: APDU, NPDU, BVLC 编码                          │   │
│  │  解码器: APDU, NPDU, BVLC 解码                          │   │
│  │  类型: ObjectID, Property, Address                      │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                     网络层                                      │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  NPDU: 网络层协议数据单元                               │   │
│  │  - 源/目标寻址                                          │   │
│  │  - 跳数、优先级、网络号                                 │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                   数据链路层                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  BVLC: BACnet 虚拟链路控制                              │   │
│  │  UDP:  UDP 套接字通信                                   │   │
│  │  MS/TP: 主从/令牌传递 (可选)                            │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                    物理层                                       │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  以太网/IP 网络                                         │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## 支持的 BACnet 服务

### 确认服务
- ReadProperty (12) - 读取属性
- ReadPropertyMultiple (14) - 批量读取属性
- WriteProperty (15) - 写入属性
- WritePropertyMultiple (16) - 批量写入属性

### 非确认服务
- IAm (0) - 我是（设备响应）
- WhoIs (8) - 谁是（设备发现）

## 对象类型

| 类型 | 代码 | 描述 |
|------|------|------|
| AnalogInput | 0 | 模拟输入点 |
| AnalogOutput | 1 | 模拟输出点 |
| AnalogValue | 2 | 模拟值 |
| BinaryInput | 3 | 二进制输入点 |
| BinaryOutput | 4 | 二进制输出点 |
| BinaryValue | 5 | 二进制值 |
| Device | 8 | BACnet 设备 |
| MultiStateInput | 13 | 多状态输入 |
| MultiStateOutput | 14 | 多状态输出 |
| TrendLog | 20 | 趋势日志 |

## 属性类型 (常用)

| 属性 | 代码 | 描述 |
|------|------|------|
| PresentValue | 85 | 对象当前值 |
| Units | 117 | 工程单位 |
| Description | 28 | 对象描述 |
| ObjectName | 77 | 对象名称 |
| ObjectType | 79 | 对象类型 |
| ObjectIdentifier | 75 | 对象标识符 |
| ObjectList | 76 | 设备中的对象列表 |

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                     应用层                                  │
│  WhoIs | ReadProperty | WriteProperty | Objects            │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                     表示层                                  │
│           编码器 / 解码器 (APDU/NPDU/BVLC)                 │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                     网络层                                  │
│              寻址、路由、优先级                             │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                   数据链路层                                │
│                    UDP / MS/TP                              │
└─────────────────────────────────────────────────────────────┘
```

## 事务管理

### TSM (事务状态机)
处理需要确认的确认服务。使用基于通道的通信进行请求/响应匹配。

### UTSM (非确认事务状态机)
处理非确认服务如 WhoIs/IAm。使用发布/订阅模式处理广播响应。

## 配置

### ClientBuilder 选项

```go
type ClientBuilder struct {
    DataLink   datalink.DataLink // 自定义数据链路 (可选)
    Interface  string            // 网络接口名 (如 "eth0")
    Ip         string            // IP 地址
    Port       int               // BACnet 端口 (默认: 47808)
    SubnetCIDR int               // 子网 CIDR (如 24 表示 /24)
    MaxPDU     uint16            // 最大 PDU 大小 (默认: 1476)
}
```

## 常量

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
