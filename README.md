## Documentation

- [English Documentation](README.md) (This file)
- [中文文档](README_CN.md)
- [房间模拟器 / Room Simulator（YABE）](ROOM_SIMULATOR.md)
  
# BACnet Protocol Stack

A Go implementation of the BACnet/IP protocol stack for building automation and control systems.

## Features

- **BACnet/IP Protocol**: Full support for BACnet/IP communication
- **Device Discovery**: Who-Is and I-Am services for network device discovery
- **Object Access**: ReadProperty, ReadMultipleProperty, WriteProperty, WriteMultipleProperty
- **Network Management**: What-Is-Network-Number, Who-Is-Router-To-Network
- **Transaction Management**: TSM (Transaction State Machine) for confirmed services
- **Concurrency**: Thread-safe design with connection pooling
- **BACnet Server**: Full server-side implementation with object store, automatic WhoIs→IAm responses, and confirmed service handling
- **Room Simulator**: `cmd/room-simulator` — YABE-friendly virtual room device (see [ROOM_SIMULATOR.md](ROOM_SIMULATOR.md))

## Installation

```bash
go get github.com/anviod/bacnet
```

Edge / cross-compile (static, no cgo): `./scripts/cross-build.sh` or `make cross`, e.g. `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./...` (ARMv7+).

## Quick Start

### Basic Device Discovery

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/anviod/bacnet"
    "github.com/anviod/bacnet/btypes"
)

func main() {
    // Create a BACnet client
    client, err := bacnet.NewClient(&bacnet.ClientBuilder{
        Ip:         "192.168.1.100",
        SubnetCIDR: 24,
        Port:       47808, // Default BACnet port (0xBAC0)
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Start the client message loop
    go client.ClientRun()

    // Discover all devices on the network
    devices, err := client.WhoIs(&bacnet.WhoIsOpts{
        Low:  0,
        High: 4194304, // Max BACnet device ID
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Discovered %d devices\n", len(devices))
    for _, dev := range devices {
        fmt.Printf("Device ID: %d, IP: %s:%d\n", dev.DeviceID, dev.Ip, dev.Port)
    }
}
```

---

## Data Collection Flow (采集流程)

The BACnet data collection process consists of six key steps:

### Step 1: Client Initialization

Before any communication can occur, a BACnet client must be created with appropriate network configuration.

```go
client, err := bacnet.NewClient(&bacnet.ClientBuilder{
    Ip:         "192.168.1.100",  // Local IP address
    SubnetCIDR: 24,                // Subnet mask (e.g., /24)
    Port:       47808,             // BACnet port (default: 47808)
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

**Configuration Options:**
- `Ip`: Local IP address to bind to
- `Interface`: Network interface name (alternative to Ip)
- `SubnetCIDR`: Subnet CIDR notation (e.g., 24 for 255.255.255.0)
- `Port`: BACnet UDP port (default: 47808 = 0xBAC0)
- `MaxPDU`: Maximum PDU size (default: 1476)

### Step 2: Start Message Loop

The client message loop must be started in a goroutine to handle incoming messages:

```go
go client.ClientRun()
```

**Important Notes:**
- Must be called before making any requests
- Runs continuously until the client is closed
- Handles message decoding and routing

### Step 3: Device Discovery (WhoIs)

Discover BACnet devices on the network using the WhoIs service:

```go
devices, err := client.WhoIs(&bacnet.WhoIsOpts{
    Low:  0,             // Device ID lower bound
    High: 4194304,       // Device ID upper bound (max)
})
```

**Discovery Options:**
- `Low`: Lower bound of device ID range (0 to 4194304)
- `High`: Upper bound of device ID range
- `GlobalBroadcast`: Use global broadcast address (0xFFFF)
- `Destination`: Specific target address for unicast discovery

**Best Practices:**
- Use narrow ID ranges for targeted discovery to reduce network traffic
- Avoid using full range (0-4194304) on large networks
- Cache discovered devices to avoid repeated discovery

### Step 4: Object Discovery

Retrieve all objects from a discovered device:

```go
scannedDevice, err := client.Objects(devices[0])
if err != nil {
    log.Printf("Failed to scan objects: %v", err)
    return
}

// Access specific object types
aiObjects := scannedDevice.Objects[btypes.AnalogInput]
biObjects := scannedDevice.Objects[btypes.BinaryInput]
aoObjects := scannedDevice.Objects[btypes.AnalogOutput]
boObjects := scannedDevice.Objects[btypes.BinaryOutput]
```

**Supported Object Types:**
- `AnalogInput` (0): Analog input points (e.g., temperature sensors)
- `AnalogOutput` (1): Analog output points (e.g., valves, dampers)
- `AnalogValue` (2): Analog value objects
- `BinaryInput` (3): Binary input points (e.g., contact sensors)
- `BinaryOutput` (4): Binary output points (e.g., relays)
- `BinaryValue` (5): Binary value objects
- `Device` (8): BACnet device objects
- `MultiStateInput` (13): Multi-state input points
- `MultiStateOutput` (14): Multi-state output points
- `TrendLog` (20): Trend log objects

### Step 5: Data Reading

Read property values from device objects.

#### Read Single Property

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

#### Read Multiple Properties (Batch)

For better performance, use ReadMultiProperty to read multiple properties in one request:

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

**Common Properties:**
- `PropPresentValue` (85): Current value of the object
- `PropUnits` (117): Engineering units
- `PropDescription` (28): Object description
- `PropObjectName` (77): Object name
- `PropObjectType` (79): Object type
- `PropObjectIdentifier` (75): Object identifier
- `PropObjectList` (76): List of objects in device

### Step 6: Data Writing

Write values to device objects.

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

**Write Priority Levels:**
- `LifeSafety` (3): Life safety operations
- `CriticalEquipment` (2): Critical equipment control
- `Urgent` (1): Urgent operations
- `Normal` (0): Normal operations

---

## Advanced Usage

### Complete Integration Flow

```go
func completeIntegration(client bacnet.Client) error {
    // Step 1: Discover devices
    devices, err := client.WhoIs(&bacnet.WhoIsOpts{
        Low:  0,
        High: 4194304,
    })
    if err != nil {
        return fmt.Errorf("whois failed: %v", err)
    }
    if len(devices) == 0 {
        return fmt.Errorf("no devices found")
    }

    device := devices[0]
    fmt.Printf("Found device: ID=%d, IP=%s:%d\n", device.DeviceID, device.Ip, device.Port)

    // Step 2: Scan objects
    scannedDevice, err := client.Objects(device)
    if err != nil {
        return fmt.Errorf("object scan failed: %v", err)
    }

    // Step 3: Find target point
    aiObjects := scannedDevice.Objects[btypes.AnalogInput]
    targetPoint, ok := aiObjects[1]
    if !ok {
        return fmt.Errorf("target point not found")
    }
    fmt.Printf("Found target point: %s\n", targetPoint.Name)

    // Step 4: Read present value
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
        return fmt.Errorf("read property failed: %v", err)
    }
    fmt.Printf("Present Value: %v\n", result.Object.Properties[0].Data)

    // Step 5: Write to AnalogValue
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
        return fmt.Errorf("write property failed: %v", writeErr)
    }
    fmt.Println("Write successful")

    return nil
}
```

### Read with Timeout

Use timeout variants for better control over request timing:

```go
result, err := client.ReadPropertyWithTimeout(device, propertyData, 5*time.Second)
```

### Error Handling Patterns

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
        // Handle specific error types
        if strings.Contains(err.Error(), "timeout") {
            return nil, fmt.Errorf("device %d did not respond", device.DeviceID)
        }
        if strings.Contains(err.Error(), "no such object") {
            return nil, fmt.Errorf("object %s not found", objID.Type)
        }
        return nil, err
    }
    
    if len(result.Object.Properties) == 0 {
        return nil, fmt.Errorf("no properties returned")
    }
    
    return result.Object.Properties[0].Data, nil
}
```

---

## API Reference

### Client Interface

```go
type Client interface {
    io.Closer
    IsRunning() bool
    ClientRun()
    
    // Device Discovery
    WhoIs(wh *WhoIsOpts) ([]btypes.Device, error)
    IAm(dest btypes.Address, iam btypes.IAm) error
    
    // Network Management
    WhatIsNetworkNumber() []*btypes.Address
    WhoIsRouterToNetwork() (resp *[]btypes.Address)
    
    // Object Access
    Objects(dev btypes.Device) (btypes.Device, error)
    ReadProperty(dest btypes.Device, rp btypes.PropertyData) (btypes.PropertyData, error)
    ReadMultiProperty(dev btypes.Device, rp btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error)
    WriteProperty(dest btypes.Device, wp btypes.PropertyData) error
    WriteMultiProperty(dev btypes.Device, wp btypes.MultiplePropertyData) error
    
    // Timeout variants
    ReadPropertyWithTimeout(dest btypes.Device, rp btypes.PropertyData, timeout time.Duration) (btypes.PropertyData, error)
    ReadMultiPropertyWithTimeout(dev btypes.Device, rp btypes.MultiplePropertyData, timeout time.Duration) (btypes.MultiplePropertyData, error)
}
```

### WhoIs Options

```go
type WhoIsOpts struct {
    Low             int             // Device ID lower bound (0 to 4194304)
    High            int             // Device ID upper bound
    GlobalBroadcast bool            // Use global broadcast (0xFFFF)
    NetworkNumber   uint16          // Target network number
    Destination     *btypes.Address // Specific destination (optional)
}
```

---

## Configuration

### ClientBuilder Options

```go
type ClientBuilder struct {
    DataLink   datalink.DataLink // Custom data link (optional)
    Interface  string            // Network interface name (e.g., "eth0")
    Ip         string            // IP address
    Port       int               // BACnet port (default: 47808)
    SubnetCIDR int               // Subnet CIDR (e.g., 24 for /24)
    MaxPDU     uint16            // Maximum PDU size (default: 1476)
}
```

### Constants

```go
// Protocol
const DefaultPort = 0xBAC0 // 47808
const MaxAPDU = 1476

// Network
const GlobalBroadcast = 0xFFFF
const DefaultHopCount = 255

// Priorities
const (
    LifeSafety        = 3
    CriticalEquipment = 2
    Urgent            = 1
    Normal            = 0
)
```

---

## Best Practices & Recommendations

### Network Considerations

1. **Port Binding**:
   - Default BACnet port is 47808 (0xBAC0)
   - Use different ports for testing to avoid conflicts
   - Bind to `0.0.0.0` to listen on all interfaces

2. **IP Address Binding**:
   - Avoid binding to the target device's IP address
   - For multi-subnet environments, configure subnet CIDR properly

3. **Broadcast Behavior**:
   - WhoIs uses broadcast by default
   - Use `Destination` for unicast requests
   - Broadcast may not work across VLANs or subnets

### Performance Optimization

1. **Batch Operations**:
   - Use `ReadMultiProperty` for reading multiple properties
   - Reduce network round-trips
   - Limit batch size based on device's MaxAPDU setting

2. **Concurrency**:
   - Client is thread-safe for concurrent operations
   - TSM limits concurrent confirmed transactions (default: 10)
   - Consider rate limiting for high-frequency operations

3. **Memory Management**:
   - Use buffer pool for efficient memory usage
   - Release resources promptly with `client.Close()`

### Error Handling

1. **Timeout Handling**:
   - Use `ReadPropertyWithTimeout` for explicit timeout control
   - Confirmed services include retry logic with exponential backoff
   - Implement application-level retry for critical operations

2. **Common Errors**:
   - `timeout`: Device did not respond within timeout
   - `invalid argument`: Invalid object type or property ID
   - `no such object`: Requested object does not exist
   - `access denied`: Insufficient permissions for write operations

---

## Common Issues & Troubleshooting

### Issue 1: No Devices Discovered

**Possible Causes:**
- Incorrect IP address or subnet configuration
- Firewall blocking BACnet port (47808)
- Devices on different VLAN/subnet
- Client not running (`ClientRun()` not called)

**Solutions:**
- Verify network configuration
- Check firewall rules
- Use Wireshark to monitor BACnet traffic
- Ensure `ClientRun()` is called before `WhoIs()`

### Issue 2: ReadProperty Fails with Timeout

**Possible Causes:**
- Device not responding
- Incorrect device address
- Network connectivity issues
- Device busy or overloaded

**Solutions:**
- Verify device is reachable via ping
- Check device address (some devices use different ports for confirmed services)
- Increase timeout value
- Implement retry logic

### Issue 3: WriteProperty Returns "Access Denied"

**Possible Causes:**
- Insufficient permissions
- Write protection enabled on device
- Incorrect priority level

**Solutions:**
- Check device configuration for write permissions
- Verify priority level (use appropriate priority)
- Contact device manufacturer for access rights

### Issue 4: High Network Traffic

**Possible Causes:**
- Frequent WhoIs requests with full ID range
- Large batch operations exceeding MTU
- Broadcast storms

**Solutions:**
- Use targeted WhoIs with narrow ID ranges
- Limit batch size to stay within MaxAPDU
- Implement device discovery caching

---

## BACnet Server (服务端)

The `server` package provides a complete BACnet/IP server implementation. It can act as a virtual BACnet device, responding to client requests with real data.

### 中文说明

`server` 包提供了完整的 BACnet/IP 服务端实现。它可以作为虚拟 BACnet 设备运行，响应客户端请求并返回真实数据。

### Quick Start - Server

```go
package main

import (
    "log"
    "os"
    "os/signal"

    "github.com/anviod/bacnet/server"
    "github.com/anviod/bacnet/btypes"
)

func main() {
    // Create server configuration
    cfg := &server.DeviceConfig{
        DeviceID:   1000,                     // BACnet device instance ID
        DeviceName: "My BACnet Server",       // BACnet device name
        VendorID:   999,                      // Vendor identifier
        Ip:         "0.0.0.0",               // Bind to all interfaces
        Port:       47808,                    // BACnet port
        SubnetCIDR: 24,                      // Subnet mask
    }

    // Create the server
    srv, err := server.NewServer(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer srv.Close()

    // Add a temperature sensor object
    srv.AddObject(btypes.Object{
        ID: btypes.ObjectID{
            Type:     btypes.AnalogInput,
            Instance: 1,
        },
        Properties: []btypes.Property{
            {
                Type:       btypes.PROP_PRESENT_VALUE,
                ArrayIndex: btypes.ArrayAll,
                Data:       float64(23.5),    // Current temperature: 23.5°C
            },
            {
                Type:       btypes.PROP_OBJECT_NAME,
                ArrayIndex: btypes.ArrayAll,
                Data:       "Room Temperature",
            },
            {
                Type:       btypes.PROP_Propertybtypes,
                ArrayIndex: btypes.ArrayAll,
                Data:       uint32(62),       // Degrees Celsius
            },
            {
                Type:       btypes.PROP_DESCRIPTION,
                ArrayIndex: btypes.ArrayAll,
                Data:       "Office room temperature sensor",
            },
        },
    })

    // Add a binary output object (relay control)
    srv.AddObject(btypes.Object{
        ID: btypes.ObjectID{
            Type:     btypes.BinaryOutput,
            Instance: 1,
        },
        Properties: []btypes.Property{
            {
                Type:       btypes.PROP_PRESENT_VALUE,
                ArrayIndex: btypes.ArrayAll,
                Data:       uint32(0),        // Off
            },
            {
                Type:       btypes.PROP_OBJECT_NAME,
                ArrayIndex: btypes.ArrayAll,
                Data:       "Fan Control",
            },
        },
    })

    // Handle graceful shutdown
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)

    // Start the server in a goroutine
    go func() {
        log.Println("BACnet server starting...")
        if err := srv.Serve(); err != nil {
            log.Printf("Server error: %v", err)
        }
    }()

    log.Printf("BACnet server started. Device ID: %d, Name: %s", srv.GetDeviceID(), cfg.DeviceName)
    log.Println("Press Ctrl+C to stop")

    // Update values dynamically (simulate sensor readings)
    // go func() {
    //     for {
    //         time.Sleep(5 * time.Second)
    //         srv.SetProperty(
    //             btypes.AnalogInput, 1,
    //             btypes.PROP_PRESENT_VALUE,
    //             float64(20.0 + rand.Float64()*10.0),
    //         )
    //     }
    // }()

    <-c
    log.Println("Shutting down server...")
}
```

### Server Features

#### 1. Object Management

The server maintains a thread-safe object store for BACnet objects:

```go
// Add an object
srv.AddObject(btypes.Object{
    ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
})

// Get an object
obj, found := srv.GetObject(btypes.AnalogInput, 1)

// Remove an object
srv.RemoveObject(btypes.AnalogInput, 1)

// Set a property value
srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, float64(25.0))

// Get a property value
value, found := srv.GetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE)
```

#### 2. Automatic WhoIs → IAm Response

When a BACnet client sends a WhoIs broadcast, the server automatically responds with an IAm message if its device ID falls within the requested range:

```go
// Client sends: WhoIs(0, 4194304) → all devices
// Server responds: IAm(DeviceID=1000, ...)

// Client sends: WhoIs(2000, 3000) → devices in range
// Server (DeviceID=1000) does NOT respond (out of range)
```

#### 3. Supported Confirmed Services

| Service | Description | Status |
|---------|-------------|--------|
| ReadProperty | 读取单个属性值 | Supported |
| WriteProperty | 写入单个属性值 | Supported |
| ReadPropertyMultiple | 批量读取多个属性 | Supported |
| WritePropertyMultiple | 批量写入多个属性 | Supported |

#### 4. BACnet Error Responses

The server returns proper BACnet error responses for invalid requests:

| Error Condition | Error Class | Error Code |
|----------------|-------------|------------|
| Unknown object | ObjectError | UnknownObject |
| Unknown property | PropertyError | UnknownProperty |
| Write access denied | PropertyError | WriteAccessDenied |
| Invalid request tag | ServicesError | InvalidTag |
| Missing parameter | ServicesError | MissingRequiredParameter |
| Unsupported service | ServicesError | ServiceRequestDenied |

#### 5. Device Object Properties

The server automatically maintains the standard BACnet Device object properties:

| Property | Value |
|----------|-------|
| ObjectIdentifier | Configured device ID |
| ObjectName | Configured device name |
| VendorIdentifier | Configured vendor ID |
| VendorName | "BACnet-Go" |
| ModelName | "BACnet-Go Server" |
| FirmwareRevision | "1.0.0" |
| ProtocolVersion | 1 |
| ProtocolRevision | 24 |
| ProtocolServicesSupported | BitString (ReadProperty, WriteProperty, etc.) |
| ProtocolObjectTypesSupported | BitString (AI, AO, AV, BI, BO, BV, etc.) |
| MaxAPDUAccepted | 1476 |
| DatabaseRevision | Auto-incremented on changes |

#### 6. Default Object Properties

When adding an object without specifying properties, the server creates default properties:

```go
// Adding an object with no properties gets defaults
srv.AddObject(btypes.Object{
    ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
})
// Automatically creates: ObjectIdentifier, ObjectName, ObjectType,
// PresentValue, StatusFlags, EventState, Reliability, OutOfService, Units
```

### Client ↔ Server Interaction Example

```go
// —— Server Side ——
srv, _ := server.NewServer(&server.DeviceConfig{
    DeviceID: 1000, DeviceName: "TestServer", VendorID: 999,
    Ip: "0.0.0.0", Port: 47808, SubnetCIDR: 24,
})
go srv.Serve()
defer srv.Close()

srv.AddObject(btypes.Object{
    ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
    Properties: []btypes.Property{
        {Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(42.5)},
    },
})

// —— Client Side ——
client, _ := bacnet.NewClient(&bacnet.ClientBuilder{
    Ip: "192.168.1.100", SubnetCIDR: 24, Port: 47808,
})
go client.ClientRun()
defer client.Close()

// Discover the server
devices, _ := client.WhoIs(&bacnet.WhoIsOpts{Low: 0, High: 4194304})

// Read property from the server
result, _ := client.ReadProperty(devices[0], btypes.PropertyData{
    Object: btypes.Object{
        ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
        Properties: []btypes.Property{
            {Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll},
        },
    },
})
// result.Object.Properties[0].Data == float64(42.5)

// Write property to the server
client.WriteProperty(devices[0], btypes.PropertyData{
    Object: btypes.Object{
        ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
        Properties: []btypes.Property{
            {Type: btypes.PROP_PRESENT_VALUE, ArrayIndex: btypes.ArrayAll, Data: float64(99.9)},
        },
    },
})
```

### Server API Reference

```go
type Server interface {
    // Lifecycle
    Serve() error
    Close() error
    IsRunning() bool

    // Object Management
    AddObject(obj btypes.Object) error
    RemoveObject(objType btypes.ObjectType, instance btypes.ObjectInstance) error
    GetObject(objType btypes.ObjectType, instance btypes.ObjectInstance) (*btypes.Object, bool)
    SetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType, data interface{}) error
    GetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType) (interface{}, bool)
    GetObjectStore() *ObjectStore
    GetDeviceID() btypes.ObjectInstance
}
```

### DeviceConfig

```go
type DeviceConfig struct {
    DeviceID      btypes.ObjectInstance          // Device instance ID (0-4194304)
    DeviceName    string                         // BACnet device name
    VendorID      uint32                         // Vendor identifier
    Interface     string                         // Network interface (e.g., "eth0")
    Ip            string                         // IP address to bind
    Port          int                            // BACnet port (default: 47808)
    SubnetCIDR    int                            // Subnet CIDR (e.g., 24)
    MaxPDU        uint16                         // Maximum PDU size
    MaxSegments   uint                           // Maximum segments
    Segmentation  segmentation.SegmentedType     // Segmentation support
}
```

### Supported Object Types

| Object Type | ID | Description |
|-------------|-----|-------------|
| AnalogInput | 0 | 模拟输入 (e.g., temperature sensors) |
| AnalogOutput | 1 | 模拟输出 (e.g., valves, dampers) |
| AnalogValue | 2 | 模拟值 |
| BinaryInput | 3 | 二进制输入 (e.g., contact sensors) |
| BinaryOutput | 4 | 二进制输出 (e.g., relays) |
| BinaryValue | 5 | 二进制值 |
| Device | 8 | BACnet 设备对象 |
| MultiStateValue | 19 | 多状态值 |

---

## Testing

```bash
# Run all tests
go test ./...

# Run server tests
go test ./server/...

# Run server tests with coverage
go test -cover ./server/...

# Run specific test
go test -v ./network/...

# Run acceptance tests
go test -v -run Acceptance

# Run real device integration test
go test . -run TestRealDeviceAcceptanceFlow -count=1 -v
```

## License

MIT License

## References

- [ANSI/ASHRAE Standard 135-2020](https://www.ashrae.org/standards-research/standards/ashrae-standard-135)
- [BACnet Protocol Specification](http://www.bacnet.org/)