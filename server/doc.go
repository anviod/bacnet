// Copyright 2024 The BACnet Authors. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

// Package server implements a BACnet/IP server device.
//
// The server package provides a complete BACnet server implementation that can:
//   - Respond to WhoIs requests with IAm messages
//   - Handle ReadProperty, WriteProperty, ReadPropertyMultiple, WritePropertyMultiple
//   - Handle SubscribeCOV and emit Confirmed/Unconfirmed COV Notifications
//   - Manage BACnet objects and their properties in a thread-safe object store
//   - Send proper BACnet error / abort responses for invalid or unsupported requests
//
// Basic usage:
//
//	cfg := server.DefaultDeviceConfig()
//	srv, err := server.NewServer(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer srv.Close()
//
//	// Add some objects
//	srv.AddObject(btypes.Object{
//	    ID: btypes.ObjectID{
//	        Type:     btypes.AnalogInput,
//	        Instance: 1,
//	    },
//	})
//
//	// Start the server
//	srv.Serve() // Blocks until Close() is called
//
// Note: DeviceID 0 is a valid BACnet instance and is preserved. Use
// DefaultDeviceConfig() (or a nil cfg) when you want the demo default of 1000.
//
// 中文说明：server 包实现了 BACnet/IP 服务端设备，支持 WhoIs→IAm、
// 读写属性、SubscribeCOV/COV 通知、对象管理和错误/Abort 响应。
// DeviceID=0 为合法实例，不会被静默改写为 1000。
package server
