// Copyright 2024 The BACnet Authors. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package server

import (
	"fmt"
	"sync"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/bacerr"
	"github.com/anviod/bacnet/btypes/segmentation"
	"github.com/anviod/bacnet/datalink"
	"github.com/anviod/bacnet/encoding"
	log "github.com/anviod/bacnet/helpers/log"
	"go.uber.org/zap"
)

const mtuHeaderLength = 4
const forwardHeaderLength = 10

// Server defines the interface for a BACnet server device.
// A BACnet server responds to WhoIs requests with IAm, and handles
// ReadProperty, WriteProperty, ReadPropertyMultiple, WritePropertyMultiple,
// and SubscribeCOV requests from BACnet clients.
//
// 中文说明：Server 定义了 BACnet 服务端设备接口。
// BACnet 服务端响应 WhoIs 请求并回复 IAm，处理客户端的 ReadProperty、
// WriteProperty、ReadPropertyMultiple、WritePropertyMultiple 和 SubscribeCOV 请求。
type Server interface {
	// Serve starts the server message loop. This method blocks until the server is stopped.
	// It should be called in a goroutine for non-blocking operation.
	// Serve 启动服务端消息循环。此方法阻塞直到服务端停止。
	// 应在 goroutine 中调用以实现非阻塞操作。
	Serve() error

	// Close stops the server and releases all resources.
	// Close 停止服务端并释放所有资源。
	Close() error

	// IsRunning returns true if the server message loop is running.
	// IsRunning 返回服务端消息循环是否正在运行。
	IsRunning() bool

	// AddObject adds a new object to the server's object store.
	// AddObject 向服务端对象存储中添加新对象。
	AddObject(obj btypes.Object) error

	// RemoveObject removes an object from the server's object store.
	// RemoveObject 从服务端对象存储中移除对象。
	RemoveObject(objType btypes.ObjectType, instance btypes.ObjectInstance) error

	// GetObject retrieves an object by type and instance.
	// GetObject 按类型和实例检索对象。
	GetObject(objType btypes.ObjectType, instance btypes.ObjectInstance) (*btypes.Object, bool)

	// SetProperty sets a specific property value on an object.
	// SetProperty 设置对象上的特定属性值。
	SetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType, data interface{}) error

	// GetProperty retrieves a specific property value from an object.
	// GetProperty 从对象中检索特定属性值。
	GetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType) (interface{}, bool)

	// ReadMultiProperty reads multiple properties from multiple objects in the local
	// object store. This is the server-side counterpart of Client.ReadMultiProperty —
	// instead of sending a BACnet ReadPropertyMultiple request over the network, it
	// directly queries the local ObjectStore. PROP_ALL/REQUIRED/OPTIONAL on Device
	// objects are automatically expanded to concrete properties. PresentValue is
	// normalized (Binary→Enumerated, Analog→float32) to match wire-format expectations.
	//
	// ReadMultiProperty 从本地对象存储中批量读取多个对象的多个属性。
	// 这是 Client.ReadMultiProperty 的服务端逆向对应——不走 BACnet 网络请求，
	// 直接查询本地 ObjectStore。Device 对象的 PROP_ALL/REQUIRED/OPTIONAL
	// 会自动展开为具体属性，PresentValue 会做规范化处理。
	ReadMultiProperty(data btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error)

	// GetObjectStore returns the underlying object store for direct access.
	// GetObjectStore 返回底层对象存储以供直接访问。
	GetObjectStore() *ObjectStore

	// GetDeviceID returns the server's device instance ID.
	// GetDeviceID 返回服务端的设备实例 ID。
	GetDeviceID() btypes.ObjectInstance
}

// DeviceConfig contains the configuration for creating a BACnet server device.
//
// 中文说明：DeviceConfig 包含创建 BACnet 服务端设备的配置。
type DeviceConfig struct {
	// DeviceID is the BACnet device instance (0..4194302). Zero is a valid
	// instance ID; it is NOT rewritten. Use DefaultDeviceConfig() or a nil
	// cfg to get the conventional demo default of 1000.
	// DeviceID 为 BACnet 设备实例号（0..4194302）。0 是合法实例，不会被静默改写。
	DeviceID      btypes.ObjectInstance
	DeviceName    string                     // BACnet device name
	VendorID      uint32                     // BACnet vendor identifier
	Interface     string                     // Network interface name (e.g., "eth0")
	Ip            string                     // IP address to bind to
	Port          int                        // BACnet port (default: 47808)
	SubnetCIDR    int                        // Subnet CIDR (e.g., 24 for /24)
	MaxPDU        uint16                     // Maximum PDU size (default: 1476)
	MaxSegments   uint                       // Maximum segments accepted (default: 0 = no segmentation)
	Segmentation  segmentation.SegmentedType // Segmentation support (default: noSegmentation)
}

// DefaultDeviceConfig returns a DeviceConfig with sensible defaults.
//
// 中文说明：DefaultDeviceConfig 返回具有合理默认值的 DeviceConfig。
func DefaultDeviceConfig() *DeviceConfig {
	return &DeviceConfig{
		DeviceID:     1000,
		DeviceName:   "BACnet-Go-Server",
		VendorID:     999,
		Port:         datalink.DefaultPort,
		SubnetCIDR:   24,
		MaxPDU:       btypes.MaxAPDU,
		MaxSegments:  0,
		Segmentation: segmentation.NoSegmentation,
	}
}

type server struct {
	mu          sync.RWMutex
	dataLink    datalink.DataLink
	store       *ObjectStore
	config      *DeviceConfig
	cov         *covManager
	readBufPool sync.Pool
	running     bool
	stopCh      chan struct{}
}

// NewServer creates a new BACnet server with the given configuration.
// It initializes the data link layer, creates the object store, and
// prepares the server for handling BACnet requests.
//
// Parameters:
//   cfg - DeviceConfig containing the server configuration
//
// Returns:
//   A new Server instance and any error encountered during initialization.
//
// 中文说明：NewServer 使用给定的配置创建新的 BACnet 服务端。
// 初始化数据链路层，创建对象存储，并准备服务端以处理 BACnet 请求。
func NewServer(cfg *DeviceConfig) (Server, error) {
	if cfg == nil {
		cfg = DefaultDeviceConfig()
	}
	if err := normalizeDeviceConfig(cfg); err != nil {
		return nil, err
	}

	var dataLink datalink.DataLink
	var err error

	if cfg.Interface != "" {
		dataLink, err = datalink.NewUDPDataLink(cfg.Interface, cfg.Port)
	} else if cfg.Ip != "" {
		dataLink, err = datalink.NewUDPDataLinkFromIP(cfg.Ip, cfg.SubnetCIDR, cfg.Port)
	} else {
		dataLink, err = datalink.NewUDPDataLinkFromIP("0.0.0.0", cfg.SubnetCIDR, cfg.Port)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create data link: %w", err)
	}

	store := NewObjectStore(cfg.DeviceID, cfg.DeviceName, cfg.VendorID)

	srv := &server{
		dataLink: dataLink,
		store:    store,
		config:   cfg,
		cov:      newCOVManager(),
		readBufPool: sync.Pool{New: func() any {
			return make([]byte, cfg.MaxPDU)
		}},
		stopCh: make(chan struct{}),
	}

	return srv, nil
}

// NewServerWithDataLink creates a new BACnet server with an existing data link.
// This is primarily useful for testing with mock data links.
//
// 中文说明：NewServerWithDataLink 使用已有的数据链路创建新的 BACnet 服务端。
// 主要用于使用 mock 数据链路进行测试。
func NewServerWithDataLink(cfg *DeviceConfig, dl datalink.DataLink) (Server, error) {
	if cfg == nil {
		cfg = DefaultDeviceConfig()
	}
	if err := normalizeDeviceConfig(cfg); err != nil {
		return nil, err
	}

	store := NewObjectStore(cfg.DeviceID, cfg.DeviceName, cfg.VendorID)

	srv := &server{
		dataLink: dl,
		store:    store,
		config:   cfg,
		cov:      newCOVManager(),
		readBufPool: sync.Pool{New: func() any {
			return make([]byte, cfg.MaxPDU)
		}},
		stopCh: make(chan struct{}),
	}

	return srv, nil
}

// normalizeDeviceConfig fills empty Port/MaxPDU/Name and validates DeviceID range.
// DeviceID 0 is preserved (valid BACnet instance).
func normalizeDeviceConfig(cfg *DeviceConfig) error {
	if cfg.Port == 0 {
		cfg.Port = datalink.DefaultPort
	}
	if cfg.MaxPDU == 0 {
		cfg.MaxPDU = btypes.MaxAPDU
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = "BACnet-Go-Server"
	}
	if uint32(cfg.DeviceID) > btypes.MaxInstance {
		return fmt.Errorf("DeviceID %d exceeds BACnet MaxInstance %d", cfg.DeviceID, btypes.MaxInstance)
	}
	return nil
}

// Serve starts the server message loop. It continuously receives packets
// from the data link layer and processes them in separate goroutines.
//
// 中文说明：Serve 启动服务端消息循环。持续从数据链路层接收数据包
// 并在独立的 goroutine 中处理。
func (s *server) Serve() error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	log.Logger.Info("BACnet server started",
		zap.Uint32("deviceID", uint32(s.config.DeviceID)),
		zap.String("deviceName", s.config.DeviceName),
		zap.Int("port", s.config.Port),
	)

	var err error
	for {
		select {
		case <-s.stopCh:
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			log.Logger.Info("BACnet server stopped")
			return nil
		default:
		}

		b := s.readBufPool.Get().([]byte)
		addr, n, recvErr := s.dataLink.Receive(b)
		if recvErr != nil {
			continue
		}
		go s.handleMsg(addr, b[:n])
		// Return the buffer to the pool after handling
		_ = err
	}
}

// Close stops the server and releases all resources.
func (s *server) Close() error {
	close(s.stopCh)
	if s.dataLink != nil {
		return s.dataLink.Close()
	}
	return nil
}

// IsRunning returns true if the server is running.
func (s *server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// AddObject adds an object to the store.
func (s *server) AddObject(obj btypes.Object) error {
	return s.store.AddObject(obj)
}

// RemoveObject removes an object from the store.
func (s *server) RemoveObject(objType btypes.ObjectType, instance btypes.ObjectInstance) error {
	return s.store.RemoveObject(objType, instance)
}

// GetObject retrieves an object from the store.
func (s *server) GetObject(objType btypes.ObjectType, instance btypes.ObjectInstance) (*btypes.Object, bool) {
	return s.store.GetObject(objType, instance)
}

// SetProperty sets a property on an object and notifies COV subscribers when PresentValue changes.
func (s *server) SetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType, data interface{}) error {
	err := s.store.SetProperty(objType, instance, propType, data)
	if err != nil {
		return err
	}
	if propType == btypes.PROP_PRESENT_VALUE {
		s.notifyCOV(objType, instance)
	}
	return nil
}

// GetProperty retrieves a property from an object.
func (s *server) GetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType) (interface{}, bool) {
	return s.store.GetProperty(objType, instance, propType)
}

// ReadMultiProperty reads multiple properties from multiple objects in the local
// object store. This is the server-side counterpart of Client.ReadMultiProperty —
// instead of sending a BACnet ReadPropertyMultiple request over the network, it
// directly queries the local ObjectStore.
//
// ReadMultiProperty 从本地对象存储中批量读取多个对象的多个属性。
// 这是 Client.ReadMultiProperty 的服务端逆向对应——不走 BACnet 网络请求，
// 直接查询本地 ObjectStore。
func (s *server) ReadMultiProperty(data btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error) {
	response := btypes.MultiplePropertyData{
		Objects: make([]btypes.Object, 0),
	}

	for _, reqObj := range data.Objects {
		objType := reqObj.ID.Type
		objInstance := reqObj.ID.Instance

		respObj := btypes.Object{
			ID: btypes.ObjectID{
				Type:     objType,
				Instance: objInstance,
			},
			Properties: make([]btypes.Property, 0),
		}

		for _, reqProp := range reqObj.Properties {
			// Expand ALL/REQUIRED/OPTIONAL on Device into concrete properties.
			if objType == btypes.DeviceType &&
				(reqProp.Type == btypes.PROP_ALL || reqProp.Type == btypes.PROP_REQUIRED || reqProp.Type == btypes.PROP_OPTIONAL) {
				if !s.store.DevicePropertyExists(objInstance) {
					continue
				}
				respObj.Properties = append(respObj.Properties, s.store.ListDeviceProperties()...)
				continue
			}

			value, found := s.store.GetPropertyAt(objType, objInstance, reqProp.Type, reqProp.ArrayIndex)
			if found {
				if reqProp.Type == btypes.PROP_PRESENT_VALUE {
					value = normalizePresentValue(objType, value)
				}
				respObj.Properties = append(respObj.Properties, btypes.Property{
					Type:       reqProp.Type,
					ArrayIndex: reqProp.ArrayIndex,
					Data:       value,
				})
			}
		}

		response.Objects = append(response.Objects, respObj)
	}

	return response, nil
}

// GetObjectStore returns the underlying object store.
func (s *server) GetObjectStore() *ObjectStore {
	return s.store
}

// GetDeviceID returns the device ID.
func (s *server) GetDeviceID() btypes.ObjectInstance {
	return s.store.GetDeviceID()
}

// handleMsg processes an incoming BACnet message.
// It decodes the BVLC, NPDU, and APDU layers, then dispatches to the appropriate handler.
//
// 中文说明：handleMsg 处理传入的 BACnet 消息。
// 解码 BVLC、NPDU 和 APDU 层，然后分发到相应的处理程序。
func (s *server) handleMsg(src *btypes.Address, b []byte) {
	var header btypes.BVLC
	var npdu btypes.NPDU
	var apdu btypes.APDU

	dec := encoding.NewDecoder(b)
	err := dec.BVLC(&header)
	if err != nil {
		log.Logger.Error("BVLC decode error", zap.Error(err))
		return
	}

	if header.Function == btypes.BacFuncBroadcast || header.Function == btypes.BacFuncUnicast || header.Function == btypes.BacFuncForwardedNPDU {
		b = b[mtuHeaderLength:]

		_, err := dec.NPDU(&npdu)
		if err != nil {
			log.Logger.Error("NPDU decode error", zap.Error(err))
			return
		}

		// Network-layer routing services are not implemented on the server.
		if npdu.IsNetworkLayerMessage {
			log.Logger.Debug("ignored network layer message (not supported)",
				zap.Uint8("type", uint8(npdu.NetworkLayerMessageType)))
			return
		}

		err = dec.APDU(&apdu)
		if err != nil {
			log.Logger.Error("APDU decode error", zap.Error(err))
			return
		}

		// Segmentation is not supported; abort segmented confirmed requests.
		if apdu.SegmentedMessage && apdu.DataType == btypes.ConfirmedServiceRequest {
			s.sendAbort(src, &npdu, apdu.InvokeId, encoding.AbortReasonSegmentationNotSupported)
			return
		}

		switch apdu.DataType {
		case btypes.UnconfirmedServiceRequest:
			s.handleUnconfirmed(src, &npdu, &apdu)

		case btypes.ConfirmedServiceRequest:
			s.handleConfirmed(src, &npdu, &apdu)

		default:
			log.Logger.Debug("ignored packet type", zap.Uint8("dataType", uint8(apdu.DataType)))
		}
	}
}

// handleUnconfirmed processes unconfirmed service requests (e.g., WhoIs).
//
// 中文说明：handleUnconfirmed 处理非确认服务请求（如 WhoIs）。
func (s *server) handleUnconfirmed(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	switch apdu.UnconfirmedService {
	case btypes.ServiceUnconfirmedWhoIs:
		s.handleWhoIs(src, npdu, apdu)
	default:
		log.Logger.Debug("unhandled unconfirmed service",
			zap.Uint8("service", uint8(apdu.UnconfirmedService)))
	}
}

// handleConfirmed processes confirmed service requests and sends appropriate responses.
//
// 中文说明：handleConfirmed 处理确认服务请求并发送适当的响应。
func (s *server) handleConfirmed(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	switch apdu.Service {
	case btypes.ServiceConfirmedReadProperty:
		s.handleReadProperty(src, npdu, apdu)
	case btypes.ServiceConfirmedWriteProperty:
		s.handleWriteProperty(src, npdu, apdu)
	case btypes.ServiceConfirmedReadPropMultiple:
		s.handleReadMultiProperty(src, npdu, apdu)
	case btypes.ServiceConfirmedWritePropMultiple:
		s.handleWriteMultiProperty(src, npdu, apdu)
	case btypes.ServiceConfirmedSubscribeCOV:
		s.handleSubscribeCOV(src, npdu, apdu)
	default:
		log.Logger.Debug("unhandled confirmed service",
			zap.Uint8("service", uint8(apdu.Service)))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.ServiceRequestDenied)
	}
}

// handleWhoIs responds to a WhoIs request with an IAm message.
// It checks if the server's device ID falls within the requested range.
//
// 中文说明：handleWhoIs 响应 WhoIs 请求，发送 IAm 消息。
// 检查服务端设备 ID 是否在请求范围内。
func (s *server) handleWhoIs(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var low, high int32
	dec.WhoIs(&low, &high)

	if low == -1 {
		low = 0
	}
	if high == -1 {
		high = int32(btypes.MaxInstance)
	}

	deviceID := int32(s.store.GetDeviceID())

	// Check if our device ID falls within the requested range
	if deviceID >= low && deviceID <= high {
		iam := btypes.IAm{
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: s.store.GetDeviceID(),
			},
			MaxApdu:      uint32(s.config.MaxPDU),
			Segmentation: btypes.Enumerated(s.config.Segmentation),
			Vendor:       s.store.GetVendorID(),
		}

		enc := encoding.NewEncoder()
		err := enc.IAm(iam)
		if err != nil {
			log.Logger.Error("failed to encode IAm", zap.Error(err))
			return
		}

		_, err = s.sendPacket(src, npdu, enc.Bytes(), false)
		if err != nil {
			log.Logger.Error("failed to send IAm", zap.Error(err))
		}
	}
}

// handleReadProperty processes a ReadProperty confirmed service request.
//
// 中文说明：handleReadProperty 处理 ReadProperty 确认服务请求。
func (s *server) handleReadProperty(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var rpData btypes.PropertyData
	err := dec.ReadProperty(&rpData)
	if err != nil {
		log.Logger.Error("failed to decode ReadProperty", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.InvalidTag)
		return
	}

	if len(rpData.Object.Properties) == 0 {
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.MissingRequiredParameter)
		return
	}

	objType := rpData.Object.ID.Type
	objInstance := rpData.Object.ID.Instance
	propType := rpData.Object.Properties[0].Type
	arrayIndex := rpData.Object.Properties[0].ArrayIndex

	// Distinguish unknown object vs unknown property
	if objType == btypes.DeviceType {
		if !s.store.DevicePropertyExists(objInstance) {
			s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ObjectError, bacerr.UnknownObject)
			return
		}
	} else if _, found := s.store.GetObject(objType, objInstance); !found {
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ObjectError, bacerr.UnknownObject)
		return
	}

	// PROP_ALL / REQUIRED / OPTIONAL on Device → expand to concrete properties (YABE/RPM browsers).
	if objType == btypes.DeviceType && (propType == btypes.PROP_ALL || propType == btypes.PROP_REQUIRED || propType == btypes.PROP_OPTIONAL) {
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.PropertyError, bacerr.Other)
		// ReadProperty does not accept ALL; clients should use ReadPropertyMultiple.
		return
	}

	// Look up the property value (with array index support for ObjectList)
	value, found := s.store.GetPropertyAt(objType, objInstance, propType, arrayIndex)
	if !found {
		// Object_List out-of-range index → InvalidArrayIndex (not UnknownProperty).
		if objType == btypes.DeviceType && propType == btypes.PROP_OBJECT_LIST && arrayIndex != btypes.ArrayAll {
			s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.PropertyError, bacerr.InvalidArrayIndex)
			return
		}
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.PropertyError, bacerr.UnknownProperty)
		return
	}

	// Binary/analog present-value normalization for wire encoding
	if propType == btypes.PROP_PRESENT_VALUE {
		value = normalizePresentValue(objType, value)
	}

	// Build the response
	response := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     objType,
				Instance: objInstance,
			},
			Properties: []btypes.Property{
				{
					Type:       propType,
					ArrayIndex: arrayIndex,
					Data:       value,
				},
			},
		},
	}

	enc := encoding.NewEncoder()
	err = enc.ReadPropertyAck(apdu.InvokeId, response)
	if err != nil {
		log.Logger.Error("failed to encode ReadPropertyAck", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.DeviceError, bacerr.OperationalProblem)
		return
	}

	// No segmentation: if the APDU exceeds the client's advertised max, abort so
	// browsers (YABE) can fall back to Object_List array-index reads.
	maxAPDU := apdu.MaxApdu
	if maxAPDU == 0 {
		maxAPDU = btypes.MaxAPDU
	}
	if uint(len(enc.Bytes())) > maxAPDU {
		s.sendAbort(src, npdu, apdu.InvokeId, encoding.AbortReasonBufferOverflow)
		return
	}

	_, err = s.sendPacket(src, npdu, enc.Bytes(), false)
	if err != nil {
		log.Logger.Error("failed to send ReadPropertyAck", zap.Error(err))
	}
}

// handleWriteProperty processes a WriteProperty confirmed service request.
//
// 中文说明：handleWriteProperty 处理 WriteProperty 确认服务请求。
func (s *server) handleWriteProperty(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var wpData btypes.PropertyData
	err := dec.ReadProperty(&wpData)
	if err != nil {
		log.Logger.Error("failed to decode WriteProperty", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.InvalidTag)
		return
	}

	if len(wpData.Object.Properties) == 0 {
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.MissingRequiredParameter)
		return
	}

	objType := wpData.Object.ID.Type
	objInstance := wpData.Object.ID.Instance
	propType := wpData.Object.Properties[0].Type
	propValue := wpData.Object.Properties[0].Data

	// Check if object exists
	_, found := s.store.GetObject(objType, objInstance)
	if !found {
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ObjectError, bacerr.UnknownObject)
		return
	}

	// Write the property value
	err = s.store.SetProperty(objType, objInstance, propType, propValue)
	if err != nil {
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.PropertyError, bacerr.WriteAccessDenied)
		return
	}

	s.sendSimpleAck(src, npdu, apdu.InvokeId, btypes.ServiceConfirmedWriteProperty)

	if propType == btypes.PROP_PRESENT_VALUE {
		s.notifyCOV(objType, objInstance)
	}
}

// handleReadMultiProperty processes a ReadPropertyMultiple confirmed service request.
//
// 中文说明：handleReadMultiProperty 处理 ReadPropertyMultiple 确认服务请求。
func (s *server) handleReadMultiProperty(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var rmpData btypes.MultiplePropertyData
	err := dec.ReadMultipleProperty(&rmpData)
	if err != nil {
		log.Logger.Error("failed to decode ReadMultipleProperty", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.InvalidTag)
		return
	}

	// Build response with actual values
	response := btypes.MultiplePropertyData{
		Objects: make([]btypes.Object, 0),
	}

	for _, reqObj := range rmpData.Objects {
		objType := reqObj.ID.Type
		objInstance := reqObj.ID.Instance

		respObj := btypes.Object{
			ID: btypes.ObjectID{
				Type:     objType,
				Instance: objInstance,
			},
			Properties: make([]btypes.Property, 0),
		}

		for _, reqProp := range reqObj.Properties {
			// Expand ALL/REQUIRED/OPTIONAL on Device into concrete properties.
			if objType == btypes.DeviceType &&
				(reqProp.Type == btypes.PROP_ALL || reqProp.Type == btypes.PROP_REQUIRED || reqProp.Type == btypes.PROP_OPTIONAL) {
				if !s.store.DevicePropertyExists(objInstance) {
					continue
				}
				respObj.Properties = append(respObj.Properties, s.store.ListDeviceProperties()...)
				continue
			}

			value, found := s.store.GetPropertyAt(objType, objInstance, reqProp.Type, reqProp.ArrayIndex)
			if found {
				if reqProp.Type == btypes.PROP_PRESENT_VALUE {
					value = normalizePresentValue(objType, value)
				}
				respObj.Properties = append(respObj.Properties, btypes.Property{
					Type:       reqProp.Type,
					ArrayIndex: reqProp.ArrayIndex,
					Data:       value,
				})
			}
		}

		response.Objects = append(response.Objects, respObj)
	}

	enc := encoding.NewEncoder()
	err = enc.ReadMultiplePropertyAck(apdu.InvokeId, response)
	if err != nil {
		log.Logger.Error("failed to encode ReadMultiplePropertyAck", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.DeviceError, bacerr.OperationalProblem)
		return
	}

	maxAPDU := apdu.MaxApdu
	if maxAPDU == 0 {
		maxAPDU = btypes.MaxAPDU
	}
	if uint(len(enc.Bytes())) > maxAPDU {
		s.sendAbort(src, npdu, apdu.InvokeId, encoding.AbortReasonBufferOverflow)
		return
	}

	_, err = s.sendPacket(src, npdu, enc.Bytes(), false)
	if err != nil {
		log.Logger.Error("failed to send ReadMultiplePropertyAck", zap.Error(err))
	}
}

// handleWriteMultiProperty processes a WritePropertyMultiple confirmed service request.
//
// 中文说明：handleWriteMultiProperty 处理 WritePropertyMultiple 确认服务请求。
func (s *server) handleWriteMultiProperty(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var wmpData btypes.MultiplePropertyData
	err := dec.WriteMultipleProperty(&wmpData)
	if err != nil {
		log.Logger.Error("failed to decode WriteMultipleProperty", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.InvalidTag)
		return
	}

	for _, obj := range wmpData.Objects {
		objType := obj.ID.Type
		objInstance := obj.ID.Instance

		// Check if object exists
		_, found := s.store.GetObject(objType, objInstance)
		if !found {
			s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ObjectError, bacerr.UnknownObject)
			return
		}

		changedPV := false
		for _, prop := range obj.Properties {
			err = s.store.SetProperty(objType, objInstance, prop.Type, prop.Data)
			if err != nil {
				s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.PropertyError, bacerr.WriteAccessDenied)
				return
			}
			if prop.Type == btypes.PROP_PRESENT_VALUE {
				changedPV = true
			}
		}
		if changedPV {
			s.notifyCOV(objType, objInstance)
		}
	}

	s.sendSimpleAck(src, npdu, apdu.InvokeId, btypes.ServiceConfirmedWritePropMultiple)
}

// sendAbort sends a BACnet Abort PDU (e.g. segmentation-not-supported).
func (s *server) sendAbort(dest *btypes.Address, npdu *btypes.NPDU, invokeID uint8, reason encoding.AbortReason) {
	// Abort PDU: type(0x70)|server(0x01), invokeID, reason
	payload := []byte{byte(btypes.Abort) | 0x01, invokeID, uint8(reason)}
	if _, err := s.sendPacket(dest, npdu, payload, false); err != nil {
		log.Logger.Error("failed to send Abort PDU", zap.Error(err))
	}
}

// sendError sends a BACnet Error PDU response to the client.
//
// 中文说明：sendError 向客户端发送 BACnet 错误 PDU 响应。
func (s *server) sendError(dest *btypes.Address, npdu *btypes.NPDU, invokeID uint8, service btypes.ServiceConfirmed, errorClass bacerr.ErrorClass, errorCode bacerr.ErrorCode) {
	enc := encoding.NewEncoder()
	a := btypes.APDU{
		DataType: btypes.Error,
		Service:  service,
		InvokeId: invokeID,
	}
	a.Error.Class = errorClass
	a.Error.Code = errorCode
	err := enc.APDU(a)
	if err != nil {
		log.Logger.Error("failed to encode Error PDU", zap.Error(err))
		return
	}

	_, err = s.sendPacket(dest, npdu, enc.Bytes(), false)
	if err != nil {
		log.Logger.Error("failed to send Error PDU", zap.Error(err))
	}
}

// sendPacket sends a raw APDU payload to the specified destination.
// It wraps the data with NPDU + BVLC header and sends it via the data link layer.
//
// 中文说明：sendPacket 向指定目标发送原始 APDU 负载。
// 使用 NPDU + BVLC 头部包装数据并通过数据链路层发送。
func (s *server) sendPacket(dest *btypes.Address, reqNPDU *btypes.NPDU, data []byte, isBroadcast bool) (int, error) {
	respNPDU := &btypes.NPDU{
		Version:               btypes.ProtocolVersion,
		IsNetworkLayerMessage: false,
		ExpectingReply:        false,
		Priority:              btypes.Normal,
		HopCount:              btypes.DefaultHopCount,
	}
	if reqNPDU != nil {
		respNPDU.Priority = reqNPDU.Priority
	}

	enc := encoding.NewEncoder()
	enc.NPDU(respNPDU)
	npduBytes := enc.Bytes()
	payload := make([]byte, 0, len(npduBytes)+len(data))
	payload = append(payload, npduBytes...)
	payload = append(payload, data...)

	var header btypes.BVLC
	header.Type = btypes.BVLCTypeBacnetIP

	if isBroadcast || (dest != nil && (dest.IsBroadcast() || dest.IsSubBroadcast())) {
		header.Function = btypes.BacFuncBroadcast
	} else {
		header.Function = btypes.BacFuncUnicast
	}

	header.Length = uint16(mtuHeaderLength + len(payload))
	header.Data = payload

	bvlcEnc := encoding.NewEncoder()
	err := bvlcEnc.BVLC(header)
	if err != nil {
		return 0, err
	}

	return s.dataLink.Send(bvlcEnc.Bytes(), respNPDU, dest)
}

// sendUnconfirmed sends an unconfirmed service request to the specified destination.
// This is used for sending IAm, WhoIs, and other unconfirmed services.
//
// 中文说明：sendUnconfirmed 向指定目标发送非确认服务请求。
// 用于发送 IAm、WhoIs 和其他非确认服务。
func (s *server) sendUnconfirmed(dest *btypes.Address, npdu *btypes.NPDU, data []byte, isBroadcast bool) (int, error) {
	return s.sendPacket(dest, npdu, data, isBroadcast)
}

// normalizePresentValue converts stored values into BACnet wire-friendly types.
func normalizePresentValue(objType btypes.ObjectType, value interface{}) interface{} {
	switch objType {
	case btypes.BinaryOutput, btypes.BinaryValue, btypes.BinaryInput:
		switch v := value.(type) {
		case uint32:
			return btypes.Enumerated(v)
		case float64:
			return btypes.Enumerated(uint32(v))
		case float32:
			return btypes.Enumerated(uint32(v))
		case bool:
			if v {
				return btypes.Enumerated(1)
			}
			return btypes.Enumerated(0)
		}
	case btypes.AnalogInput, btypes.AnalogOutput, btypes.AnalogValue:
		switch v := value.(type) {
		case float64:
			return float32(v)
		case float32:
			return v
		case int:
			return float32(v)
		case uint32:
			return float32(v)
		}
	case btypes.MultiStateInput, btypes.MultiStateOutput, btypes.MultiStateValue:
		switch v := value.(type) {
		case uint32:
			return v
		case float64:
			return uint32(v)
		case float32:
			return uint32(v)
		case int:
			return uint32(v)
		case btypes.Enumerated:
			return uint32(v)
		}
	}
	return value
}