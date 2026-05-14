## Documentation

- [English Documentation](README.md) (This file)
- [中文文档](README_CN.md)
  
# BACnet Protocol Stack

A Go implementation of the BACnet/IP protocol stack for building automation and control systems.

## Features

- **BACnet/IP Protocol**: Full support for BACnet/IP communication
- **Device Discovery**: Who-Is and I-Am services for network device discovery
- **Object Access**: ReadProperty, ReadMultipleProperty, WriteProperty, WriteMultipleProperty
- **Network Management**: What-Is-Network-Number, Who-Is-Router-To-Network
- **Transaction Management**: TSM (Transaction State Machine) for confirmed services
- **Concurrency**: Thread-safe design with connection pooling

## Installation

```bash
go get github.com/anviod/bacnet
```

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

### Read Point Value (Analog Input)

```go
// Read the present value of an analog input point (AI-1)
func readAnalogInput(client bacnet.Client, device btypes.Device) {
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogInput, // Object type: Analog Input
                Instance: 1,                  // Point number: AI-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue, // Read present value
                    ArrayIndex: btypes.ArrayAll,
                },
            },
        },
    })
    if err != nil {
        log.Printf("Failed to read AI-1: %v", err)
        return
    }

    // Get the value
    if len(result.Object.Properties) > 0 {
        fmt.Printf("AI-1 Present Value: %v\n", result.Object.Properties[0].Data)
    }
}
```

### Read Binary Input Point

```go
// Read the present value of a binary input point (BI-1)
func readBinaryInput(client bacnet.Client, device btypes.Device) {
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.BinaryInput, // Object type: Binary Input
                Instance: 1,                  // Point number: BI-1
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
        log.Printf("Failed to read BI-1: %v", err)
        return
    }

    if len(result.Object.Properties) > 0 {
        value := result.Object.Properties[0].Data
        state := "OFF"
        if value == true || value == uint8(1) {
            state = "ON"
        }
        fmt.Printf("BI-1 Present Value: %s (%v)\n", state, value)
    }
}
```

### Write Point Value (Analog Output)

```go
// Write a value to an analog output point (AO-1)
func writeAnalogOutput(client bacnet.Client, device btypes.Device, value float64) error {
    err := client.WriteProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogOutput, // Object type: Analog Output
                Instance: 1,                   // Point number: AO-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue,
                    ArrayIndex: btypes.ArrayAll,
                    Data:       value,             // Value to write (e.g., 25.5)
                    Priority:   btypes.Normal,    // Priority level
                },
            },
        },
    })
    if err != nil {
        log.Printf("Failed to write AO-1: %v", err)
        return err
    }

    fmt.Printf("Successfully wrote %.2f to AO-1\n", value)
    return nil
}
```

### Write Binary Output Point

```go
// Write a value to a binary output point (BO-1)
func writeBinaryOutput(client bacnet.Client, device btypes.Device, value bool) error {
    err := client.WriteProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.BinaryOutput, // Object type: Binary Output
                Instance: 1,                   // Point number: BO-1
            },
            Properties: []btypes.Property{
                {
                    Type:       btypes.PropPresentValue,
                    ArrayIndex: btypes.ArrayAll,
                    Data:       value,           // true = ON, false = OFF
                    Priority:   btypes.Normal,   // Priority level
                },
            },
        },
    })
    if err != nil {
        log.Printf("Failed to write BO-1: %v", err)
        return err
    }

    fmt.Printf("Successfully wrote %v to BO-1\n", value)
    return nil
}
```

### Read Multiple Properties at Once

```go
// Read multiple properties from multiple objects in one request
func readMultiplePoints(client bacnet.Client, device btypes.Device) {
    result, err := client.ReadMultiProperty(device, btypes.MultiplePropertyData{
        Objects: []btypes.Object{
            // Read AI-1 present value and units
            {
                ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 1},
                Properties: []btypes.Property{
                    {Type: btypes.PropPresentValue},
                    {Type: btypes.PropUnits},
                },
            },
            // Read AI-2 present value
            {
                ID: btypes.ObjectID{Type: btypes.AnalogInput, Instance: 2},
                Properties: []btypes.Property{
                    {Type: btypes.PropPresentValue},
                },
            },
            // Read BI-1 present value
            {
                ID: btypes.ObjectID{Type: btypes.BinaryInput, Instance: 1},
                Properties: []btypes.Property{
                    {Type: btypes.PropPresentValue},
                },
            },
        },
    })
    if err != nil {
        log.Printf("Failed to read multiple properties: %v", err)
        return
    }

    // Process results
    for _, obj := range result.Objects {
        fmt.Printf("Object: %s-%d\n", obj.ID.Type, obj.ID.Instance)
        for _, prop := range obj.Properties {
            fmt.Printf("  %s: %v\n", prop.Type, prop.Data)
        }
    }
}
```

### Scan All Objects in Device

```go
// Scan all objects in a device
func scanDeviceObjects(client bacnet.Client, device btypes.Device) error {
    // Get all objects from the device
    scannedDevice, err := client.Objects(device)
    if err != nil {
        return fmt.Errorf("failed to scan objects: %v", err)
    }

    fmt.Printf("Found %d objects in device %d\n", scannedDevice.Objects.Len(), device.DeviceID)

    // Iterate through all object types
    objectTypes := []btypes.ObjectType{
        btypes.AnalogInput,
        btypes.AnalogOutput,
        btypes.AnalogValue,
        btypes.BinaryInput,
        btypes.BinaryOutput,
        btypes.BinaryValue,
    }

    for _, objType := range objectTypes {
        objects := scannedDevice.Objects[objType]
        if len(objects) == 0 {
            continue
        }

        fmt.Printf("\n%s objects:\n", objType)
        for instance, obj := range objects {
            fmt.Printf("  Instance %d: Name=%q\n", instance, obj.Name)
        }
    }

    return nil
}
```

### Complete Device Integration Flow

```go
// Complete flow: Discover device -> Scan objects -> Read value -> Write value
func completeIntegration(client bacnet.Client) error {
    // Step 1: Discover devices
    devices, err := client.WhoIs(&bacnet.WhoIsOpts{
        Low:  2228316,
        High: 2228316,
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
    targetPoint, ok := aiObjects[0] // AnalogInput:0
    if !ok {
        return fmt.Errorf("target point AnalogInput:0 not found")
    }
    fmt.Printf("Found target point: %s\n", targetPoint.Name)

    // Step 4: Read present value
    result, err := client.ReadProperty(device, btypes.PropertyData{
        Object: btypes.Object{
            ID: btypes.ObjectID{
                Type:     btypes.AnalogInput,
                Instance: 0,
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

### Property Reading

```go
// Read a single property from a device
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

// Read multiple properties from multiple objects
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

### Property Writing

```go
// Write a property to a device
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

## Internal Architecture & Call Flow

### 1. Client Initialization Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     NewClient()                                 │
├─────────────────────────────────────────────────────────────────┤
│  1. Validate IP Address                                         │
│     └─ validation.ValidIP(ip)                                   │
│                                                                 │
│  2. Validate Port (default: 47808)                              │
│     └─ validation.ValidPort(port)                               │
│                                                                 │
│  3. Create DataLink Layer                                       │
│     ├─ NewUDPDataLink(iface, port)         // by interface      │
│     └─ NewUDPDataLinkFromIP(ip, subnet, port) // by IP          │
│                                                                 │
│  4. Initialize TSM (Transaction State Machine)                  │
│     └─ tsm.New(defaultStateSize)                                │
│                                                                 │
│  5. Initialize UTSM (Unconfirmed TSM)                           │
│     └─ utsm.NewManager(...)                                     │
│                                                                 │
│  6. Create Buffer Pool                                          │
│     └─ sync.Pool for receive buffers                            │
└─────────────────────────────────────────────────────────────────┘
```

### 2. WhoIs Device Discovery Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     WhoIs()                                     │
├─────────────────────────────────────────────────────────────────┤
│  1. Determine Broadcast Destination                             │
│     ├─ GetBroadcastAddress()                                    │
│     └─ Override with custom destination if provided             │
│                                                                 │
│  2. Encode NPDU (Network Protocol Data Unit)                    │
│     ├─ Version: 1                                               │
│     ├─ Destination: Broadcast address                           │
│     ├─ Source: Local address                                    │
│     └─ ExpectingReply: false (broadcast)                        │
│                                                                 │
│  3. Encode WhoIs Service Data                                   │
│     ├─ Low device ID                                            │
│     └─ High device ID                                           │
│                                                                 │
│  4. Subscribe to UTSM for IAm responses                         │
│     └─ utsm.Subscribe(start, end)                               │
│                                                                 │
│  5. Send Broadcast Request (async)                              │
│     └─ go c.Send(dest, npdu, data, nil)                         │
│                                                                 │
│  6. Collect and Deduplicate Responses                           │
│     ├─ Filter IAm responses                                     │
│     ├─ Deduplicate by device instance ID                        │
│     └─ Build device list                                        │
└─────────────────────────────────────────────────────────────────┘
```

### 3. ReadProperty Flow (Confirmed Service)

```
┌─────────────────────────────────────────────────────────────────┐
│                     ReadProperty()                              │
├─────────────────────────────────────────────────────────────────┤
│  1. Get Transaction ID from TSM                                 │
│     └─ tsm.ID(ctx)                                              │
│                                                                 │
│  2. Build NPDU                                                  │
│     ├─ Destination: Device address                              │
│     ├─ Source: Local address                                    │
│     └─ ExpectingReply: true                                     │
│                                                                 │
│  3. Encode APDU (Confirmed Service Request)                     │
│     ├─ DataType: ConfirmedServiceRequest                        │
│     ├─ Service: ServiceConfirmedReadProperty                    │
│     ├─ InvokeId: Transaction ID                                 │
│     └─ Service Data: Object ID + Property ID                    │
│                                                                 │
│  4. Send Request with Retry                                     │
│     ├─ c.Send(dest, npdu, data, nil)                            │
│     ├─ tsm.Receive(id, timeout)                                 │
│     └─ Retry up to retryCount times                             │
│                                                                 │
│  5. Decode Response                                             │
│     ├─ Decode APDU header                                       │
│     ├─ Check for errors                                         │
│     └─ Decode property value                                    │
│                                                                 │
│  6. Release Transaction ID                                      │
│     └─ tsm.Put(id)                                              │
└─────────────────────────────────────────────────────────────────┘
```

### 4. WriteProperty Flow (Confirmed Service)

```
┌─────────────────────────────────────────────────────────────────┐
│                     WriteProperty()                             │
├─────────────────────────────────────────────────────────────────┤
│  1. Get Transaction ID from TSM                                 │
│     └─ tsm.ID(ctx)                                              │
│                                                                 │
│  2. Build NPDU                                                  │
│     ├─ Destination: Device address                              │
│     ├─ Source: Local address                                    │
│     └─ ExpectingReply: true                                     │
│                                                                 │
│  3. Encode APDU (Confirmed Service Request)                     │
│     ├─ DataType: ConfirmedServiceRequest                        │
│     ├─ Service: ServiceConfirmedWriteProperty                   │
│     ├─ InvokeId: Transaction ID                                 │
│     └─ Service Data: Object ID + Property ID + Value            │
│                                                                 │
│  4. Send Request with Retry                                     │
│     ├─ c.Send(dest, npdu, data, nil)                            │
│     ├─ tsm.Receive(id, timeout)                                 │
│     └─ Retry up to 2 times                                      │
│                                                                 │
│  5. Decode Response                                             │
│     ├─ Decode APDU header                                       │
│     ├─ Check for SimpleAck (success)                            │
│     └─ Check for Error PDU                                      │
│                                                                 │
│  6. Release Transaction ID                                      │
│     └─ tsm.Put(id)                                              │
└─────────────────────────────────────────────────────────────────┘
```

### 5. Message Reception Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     ClientRun()                                 │
├─────────────────────────────────────────────────────────────────┤
│  Loop:                                                          │
│  1. Get buffer from pool                                        │
│     └─ readBufferPool.Get()                                     │
│                                                                 │
│  2. Receive data from DataLink                                  │
│     └─ dataLink.Receive(buffer)                                 │
│                                                                 │
│  3. Handle message concurrently                                 │
│     └─ go handleMsg(addr, data)                                 │
└─────────────────────────────────────────────────────────────────┘
```

```
┌─────────────────────────────────────────────────────────────────┐
│                     handleMsg()                                 │
├─────────────────────────────────────────────────────────────────┤
│  1. Decode BVLC (BACnet Virtual Link Control)                   │
│     ├─ Type: BVLCTypeBacnetIP                                   │
│     ├─ Function: Broadcast/Unicast/ForwardedNPDU                │
│     └─ Length: Packet length                                    │
│                                                                 │
│  2. Decode NPDU                                                 │
│     ├─ Version                                                  │
│     ├─ Source/Destination addresses                             │
│     └─ Network layer message handling                           │
│                                                                 │
│  3. Decode APDU and Route to Handler                            │
│     ├─ UnconfirmedServiceRequest                                │
│     │   ├─ IAm → utsm.Publish()                                 │
│     │   └─ WhoIs → (ignore or respond)                          │
│     ├─ SimpleAck → tsm.Send()                                   │
│     ├─ ComplexAck → tsm.Send()                                  │
│     ├─ ConfirmedServiceRequest → tsm.Send()                     │
│     └─ Error → tsm.Send(error)                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 6. Transaction State Machine (TSM)

```
┌─────────────────────────────────────────────────────────────────┐
│                     TSM Architecture                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │
│   │   Caller    │────▶│  TSM.ID()   │────▶│   State     │      │
│   │  (Request)  │     │  (Get ID)   │     │  (Active)   │      │
│   └─────────────┘     └─────────────┘     └──────┬──────┘      │
│                                                  │              │
│                                                  ▼              │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │
│   │   Caller    │◀────│ TSM.Put()   │◀────│   State     │      │
│   │  (Cleanup)  │     │ (Release)   │     │ (Complete)  │      │
│   └─────────────┘     └─────────────┘     └─────────────┘      │
│                                                  ▲              │
│                                                  │              │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │
│   │   handleMsg │────▶│ TSM.Send()  │────▶│   Data      │      │
│   │  (Response) │     │ (Deliver)   │     │  Channel    │      │
│   └─────────────┘     └─────────────┘     └─────────────┘      │
│                                                                 │
│  Key Components:                                                │
│  - states: map[int]*state (active transactions)                 │
│  - free.id: channel (available invoke IDs 1-254)                │
│  - free.space: channel (concurrent transaction limit)           │
│  - pool: sync.Pool (state object reuse)                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 7. Protocol Stack Layering

```
┌─────────────────────────────────────────────────────────────────┐
│                    Application Layer                            │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Services: WhoIs, IAm, ReadProperty, WriteProperty      │   │
│  │  Objects: Device, AnalogInput, BinaryOutput, etc.       │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                   Presentation Layer                            │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Encoder: APDU, NPDU, BVLC encoding                     │   │
│  │  Decoder: APDU, NPDU, BVLC decoding                     │   │
│  │  Types: ObjectID, Property, Address                     │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                     Network Layer                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  NPDU: Network Protocol Data Unit                       │   │
│  │  - Source/Destination addressing                        │   │
│  │  - Hop count, priority, network numbers                 │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                   Data Link Layer                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  BVLC: BACnet Virtual Link Control                      │   │
│  │  UDP:  UDP socket communication                         │   │
│  │  MS/TP: Master-Slave/Token-Pass (optional)              │   │
│  └─────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                    Physical Layer                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Ethernet/IP network                                      │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Supported BACnet Services

### Confirmed Services
- ReadProperty (12)
- ReadPropertyMultiple (14)
- WriteProperty (15)
- WritePropertyMultiple (16)

### Unconfirmed Services
- IAm (0)
- WhoIs (8)

## Object Types

| Type | Code | Description |
|------|------|-------------|
| AnalogInput | 0 | Analog input point |
| AnalogOutput | 1 | Analog output point |
| AnalogValue | 2 | Analog value |
| BinaryInput | 3 | Binary input point |
| BinaryOutput | 4 | Binary output point |
| BinaryValue | 5 | Binary value |
| Device | 8 | BACnet device |
| MultiStateInput | 13 | Multi-state input |
| MultiStateOutput | 14 | Multi-state output |
| TrendLog | 20 | Trend log |

## Property Types (Common)

| Property | Code | Description |
|----------|------|-------------|
| PresentValue | 85 | Current value of the object |
| Units | 117 | Engineering units |
| Description | 28 | Object description |
| ObjectName | 77 | Object name |
| ObjectType | 79 | Object type |
| ObjectIdentifier | 75 | Object identifier |
| ObjectList | 76 | List of objects in device |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Application Layer                       │
│  WhoIs | ReadProperty | WriteProperty | Objects            │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                     Presentation Layer                      │
│           Encoder / Decoder (APDU/NPDU/BVLC)               │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                      Network Layer                          │
│              Addressing, Routing, Priority                  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                   Data Link Layer                          │
│                    UDP / MS/TP                             │
└─────────────────────────────────────────────────────────────┘
```

## Transaction Management

### TSM (Transaction State Machine)
Handles confirmed services that require acknowledgment. Uses channel-based communication for request/response matching.

### UTSM (Unconfirmed Transaction State Machine)
Handles unconfirmed services like WhoIs/IAm. Uses publish/subscribe pattern for broadcast responses.

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

## Constants

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

## Usage Notes

### Network Considerations

1. **Port Binding**: 
   - Default BACnet port is 47808 (0xBAC0)
   - When testing with a simulator on the same machine, use different ports for discovery and confirmed services to avoid port conflicts
   - Example: Use port 47808 for WhoIs discovery, then switch to 47809 for ReadProperty/WriteProperty operations

2. **IP Address Binding**:
   - Bind to `0.0.0.0` to listen on all interfaces
   - Avoid binding to the target device's IP address
   - For multi-subnet environments, ensure proper subnet CIDR configuration

3. **Broadcast Behavior**:
   - WhoIs uses broadcast by default
   - Use `WhoIsOpts.Destination` for unicast WhoIs requests
   - Broadcast may not work across VLANs or subnets

### Error Handling

1. **Timeout Handling**:
   - Use `ReadPropertyWithTimeout` for explicit timeout control
   - Confirmed services include retry logic with exponential backoff
   - Unconfirmed services have no retry mechanism

2. **Common Errors**:
   - `timeout`: Device did not respond within the timeout period
   - `invalid argument`: Invalid object type or property ID
   - `no such object`: Requested object does not exist on the device
   - `access denied`: Insufficient permissions for write operations

3. **Retry Strategy**:
   - Confirmed services are retried up to 2 times by default
   - Consider implementing application-level retry for critical operations

### Performance Considerations

1. **Batch Operations**:
   - Use `ReadMultiProperty` for reading multiple properties at once
   - This reduces network round-trips and improves performance
   - Limit batch size based on device's MaxAPDU setting

2. **Concurrency**:
   - Client is thread-safe for concurrent operations
   - TSM limits concurrent confirmed transactions (default: 10)
   - Consider rate limiting for high-frequency operations

3. **Memory Management**:
   - Use buffer pool for efficient memory usage
   - Release resources promptly with `client.Close()`

### Best Practices

1. **Client Lifecycle**:
   - Always use `defer client.Close()` to release resources
   - Start `client.ClientRun()` in a goroutine before making requests
   - Verify `client.IsRunning()` before sending requests

2. **Device Communication**:
   - Cache device addresses after discovery
   - Validate device responses for expected data types
   - Handle device reboots and network interruptions gracefully

3. **Testing**:
   - Use the provided integration test (`TestRealDeviceAcceptanceFlow`) for validation
   - Test with both real devices and simulators
   - Monitor response times and error rates in production

## Testing

```bash
# Run all tests
go test ./...

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
