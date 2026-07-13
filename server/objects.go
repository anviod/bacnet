// Copyright 2024 The BACnet Authors. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package server

import (
	"fmt"
	"sort"
	"sync"

	"github.com/anviod/bacnet/btypes"
)

// ObjectStore provides thread-safe storage for BACnet device objects and their properties.
// It manages the complete object hierarchy: device → object type → object instance → properties.
//
// 中文说明：ObjectStore 提供线程安全的 BACnet 设备对象及其属性存储。
// 管理完整的对象层次结构：设备 → 对象类型 → 对象实例 → 属性。
type ObjectStore struct {
	mu      sync.RWMutex
	objects map[btypes.ObjectType]map[btypes.ObjectInstance]*btypes.Object
	// deviceProperties holds the device-level properties (object 8)
	deviceProperties map[btypes.PropertyType]interface{}
	deviceID         btypes.ObjectInstance
	deviceName       string
	vendorID         uint32
	modelName        string
	firmwareRevision string
	appSoftwareRev   string
}

// NewObjectStore creates a new ObjectStore with the given device configuration.
//
// 中文说明：NewObjectStore 使用给定的设备配置创建新的 ObjectStore。
func NewObjectStore(deviceID btypes.ObjectInstance, deviceName string, vendorID uint32) *ObjectStore {
	store := &ObjectStore{
		objects:          make(map[btypes.ObjectType]map[btypes.ObjectInstance]*btypes.Object),
		deviceProperties: make(map[btypes.PropertyType]interface{}),
		deviceID:         deviceID,
		deviceName:       deviceName,
		vendorID:         vendorID,
		modelName:        "BACnet-Go Server",
		firmwareRevision: "1.0.0",
		appSoftwareRev:   "1.0.0",
	}

	// Initialize device object properties
	store.deviceProperties[btypes.PROP_OBJECT_IDENTIFIER] = btypes.ObjectID{
		Type:     btypes.DeviceType,
		Instance: deviceID,
	}
	store.deviceProperties[btypes.PROP_OBJECT_NAME] = deviceName
	store.deviceProperties[btypes.PROP_OBJECT_TYPE] = btypes.Enumerated(btypes.DeviceType)
	store.deviceProperties[btypes.PROP_SYSTEM_STATUS] = btypes.Enumerated(0) // operational
	store.deviceProperties[btypes.PROP_VENDOR_IDENTIFIER] = vendorID
	store.deviceProperties[btypes.PropVendorName] = "BACnet-Go"
	store.deviceProperties[btypes.PROP_MODEL_NAME] = store.modelName
	store.deviceProperties[btypes.PROP_FIRMWARE_REVISION] = store.firmwareRevision
	store.deviceProperties[btypes.PROP_APPLICATION_SOFTWARE_VERSION] = store.appSoftwareRev
	store.deviceProperties[btypes.PROP_DESCRIPTION] = "BACnet-Go virtual device"
	store.deviceProperties[btypes.PROP_PROTOCOL_VERSION] = uint32(1)
	store.deviceProperties[btypes.PROP_PROTOCOL_REVISION] = uint32(24)
	store.deviceProperties[btypes.ProtocolServicesSupported] = store.buildServicesSupported()
	store.deviceProperties[btypes.PropProtocolObjectTypesSupported] = store.buildObjectTypesSupported()
	store.deviceProperties[btypes.PropMaxAPDU] = uint32(btypes.MaxAPDU)
	store.deviceProperties[btypes.PropSegmentationSupported] = btypes.Enumerated(3) // no-segmentation
	store.deviceProperties[btypes.PROP_APDU_TIMEOUT] = uint32(3000)
	store.deviceProperties[btypes.PROP_NUMBER_OF_APDU_RETRIES] = uint32(3)
	// Empty BACnet LIST — AppData(nil) encodes no elements between opening/closing tags.
	store.deviceProperties[btypes.PROP_DEVICE_ADDRESS_BINDING] = nil
	store.deviceProperties[btypes.PROP_DATABASE_REVISION] = uint32(0)

	return store
}

// buildServicesSupported creates a BitString encoding of supported BACnet services.
func (s *ObjectStore) buildServicesSupported() *btypes.BitString {
	// BACnet protocol services supported - 40 bytes for full service bitmap
	bs := btypes.NewBitString(40)
	// Set bits for supported confirmed services
	bs.SetBit(5, true)  // SubscribeCOV
	bs.SetBit(12, true) // ReadProperty
	bs.SetBit(14, true) // ReadPropertyMultiple
	bs.SetBit(15, true) // WriteProperty
	bs.SetBit(16, true) // WritePropertyMultiple
	// Set bits for supported unconfirmed services
	bs.SetBit(28, true) // UnconfirmedCOVNotification (service choice 2 → bit position per ASHRAE bitmap varies; Index 25 in services.go)
	bs.SetBit(32, true) // IAm (bit 0 of unconfirmed services)
	bs.SetBit(40, true) // WhoIs (bit 8 of unconfirmed services)
	return bs
}

// buildObjectTypesSupported creates a BitString encoding of supported object types.
func (s *ObjectStore) buildObjectTypesSupported() *btypes.BitString {
	// Support for common object types
	bs := btypes.NewBitString(24)
	bs.SetBit(0, true)  // AnalogInput
	bs.SetBit(1, true)  // AnalogOutput
	bs.SetBit(2, true)  // AnalogValue
	bs.SetBit(3, true)  // BinaryInput
	bs.SetBit(4, true)  // BinaryOutput
	bs.SetBit(5, true)  // BinaryValue
	bs.SetBit(8, true)  // Device
	bs.SetBit(13, true) // MultiStateInput
	bs.SetBit(14, true) // MultiStateOutput
	bs.SetBit(19, true) // MultiStateValue
	return bs
}

// AddObject adds a new object to the store. Returns error if the object already exists.
//
// 中文说明：AddObject 向存储中添加新对象。如果对象已存在则返回错误。
func (s *ObjectStore) AddObject(obj btypes.Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	objType := obj.ID.Type
	objInstance := obj.ID.Instance

	if _, ok := s.objects[objType]; !ok {
		s.objects[objType] = make(map[btypes.ObjectInstance]*btypes.Object)
	}

	if _, exists := s.objects[objType][objInstance]; exists {
		return fmt.Errorf("object %s instance %d already exists", objType, objInstance)
	}

	// Add default properties if not provided
	if len(obj.Properties) == 0 {
		obj.Properties = s.defaultProperties(objType, objInstance)
	}

	objCopy := obj
	s.objects[objType][objInstance] = &objCopy
	s.incrementDatabaseRevision()
	return nil
}

// RemoveObject removes an object from the store.
//
// 中文说明：RemoveObject 从存储中移除对象。
func (s *ObjectStore) RemoveObject(objType btypes.ObjectType, instance btypes.ObjectInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if typeMap, ok := s.objects[objType]; ok {
		if _, exists := typeMap[instance]; exists {
			delete(typeMap, instance)
			s.incrementDatabaseRevision()
			return nil
		}
	}
	return fmt.Errorf("object %s instance %d not found", objType, instance)
}

// GetObject retrieves an object by type and instance.
//
// 中文说明：GetObject 按类型和实例检索对象。
func (s *ObjectStore) GetObject(objType btypes.ObjectType, instance btypes.ObjectInstance) (*btypes.Object, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if typeMap, ok := s.objects[objType]; ok {
		if obj, exists := typeMap[instance]; exists {
			objCopy := *obj
			return &objCopy, true
		}
	}
	return nil, false
}

// GetProperty retrieves a specific property from an object.
//
// 中文说明：GetProperty 从对象中检索特定属性。
func (s *ObjectStore) GetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType) (interface{}, bool) {
	return s.GetPropertyAt(objType, instance, propType, btypes.ArrayAll)
}

// GetPropertyAt retrieves a property, honoring BACnet array indexes where applicable.
// For ObjectList: index 0 returns the array size, index N returns the Nth element (1-based).
//
// 中文说明：GetPropertyAt 检索属性，并支持 BACnet 数组索引。
// 对 ObjectList：索引 0 返回数组长度，索引 N 返回第 N 个元素（从 1 开始）。
func (s *ObjectStore) GetPropertyAt(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType, arrayIndex uint32) (interface{}, bool) {
	// Handle device object properties specially
	if objType == btypes.DeviceType {
		if instance != s.deviceID {
			return nil, false
		}
		return s.getDevicePropertyAt(propType, arrayIndex)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if typeMap, ok := s.objects[objType]; ok {
		if obj, exists := typeMap[instance]; exists {
			for _, prop := range obj.Properties {
				if prop.Type == propType {
					return prop.Data, true
				}
			}
		}
	}
	return nil, false
}

// DevicePropertyExists reports whether the device instance is known to this store.
func (s *ObjectStore) DevicePropertyExists(instance btypes.ObjectInstance) bool {
	return instance == s.deviceID
}

// ListDeviceProperties returns all readable device properties for PROP_ALL / REQUIRED.
// Object_List is included as the full BACnetARRAY of Object Identifiers.
func (s *ObjectStore) ListDeviceProperties() []btypes.Property {
	s.mu.RLock()
	defer s.mu.RUnlock()

	props := make([]btypes.Property, 0, len(s.deviceProperties)+1)
	for propType, data := range s.deviceProperties {
		props = append(props, btypes.Property{
			Type:       propType,
			ArrayIndex: btypes.ArrayAll,
			Data:       data,
		})
	}
	props = append(props, btypes.Property{
		Type:       btypes.PROP_OBJECT_LIST,
		ArrayIndex: btypes.ArrayAll,
		Data:       s.getObjectListIdsLocked(),
	})
	sort.Slice(props, func(i, j int) bool {
		return props[i].Type < props[j].Type
	})
	return props
}

// SetProperty sets a specific property on an object.
//
// 中文说明：SetProperty 设置对象上的特定属性。
func (s *ObjectStore) SetProperty(objType btypes.ObjectType, instance btypes.ObjectInstance, propType btypes.PropertyType, data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if typeMap, ok := s.objects[objType]; ok {
		if obj, exists := typeMap[instance]; exists {
			for i, prop := range obj.Properties {
				if prop.Type == propType {
					obj.Properties[i].Data = data
					s.incrementDatabaseRevision()
					return nil
				}
			}
			// Property not found, add it
			obj.Properties = append(obj.Properties, btypes.Property{
				Type:       propType,
				ArrayIndex: btypes.ArrayAll,
				Data:       data,
			})
			s.incrementDatabaseRevision()
			return nil
		}
	}
	return fmt.Errorf("object %s instance %d not found", objType, instance)
}

// GetObjectList returns all user objects in the store (excludes the Device object).
//
// 中文说明：GetObjectList 返回存储中的所有用户对象（不含 Device 对象）。
func (s *ObjectStore) GetObjectList() []btypes.ObjectID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []btypes.ObjectID
	for objType, typeMap := range s.objects {
		for instance := range typeMap {
			list = append(list, btypes.ObjectID{
				Type:     objType,
				Instance: instance,
			})
		}
	}
	return list
}

// GetAllObjects returns all objects in the store.
//
// 中文说明：GetAllObjects 返回存储中的所有对象。
func (s *ObjectStore) GetAllObjects() map[btypes.ObjectType]map[btypes.ObjectInstance]*btypes.Object {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[btypes.ObjectType]map[btypes.ObjectInstance]*btypes.Object)
	for objType, typeMap := range s.objects {
		result[objType] = make(map[btypes.ObjectInstance]*btypes.Object)
		for inst, obj := range typeMap {
			objCopy := *obj
			result[objType][inst] = &objCopy
		}
	}
	return result
}

// GetDeviceID returns the device instance ID.
func (s *ObjectStore) GetDeviceID() btypes.ObjectInstance {
	return s.deviceID
}

// GetDeviceName returns the device name.
func (s *ObjectStore) GetDeviceName() string {
	return s.deviceName
}

// GetVendorID returns the vendor ID.
func (s *ObjectStore) GetVendorID() uint32 {
	return s.vendorID
}

// SetDeviceProperty sets or replaces a device-object property (object type Device).
// Useful for tuning Model_Name / Description before clients browse the device.
//
// 中文说明：SetDeviceProperty 设置或替换 Device 对象属性（如 Model_Name、Description）。
func (s *ObjectStore) SetDeviceProperty(propType btypes.PropertyType, data interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceProperties[propType] = data
}

// getDeviceProperty retrieves a device-level property.
func (s *ObjectStore) getDeviceProperty(propType btypes.PropertyType) (interface{}, bool) {
	return s.getDevicePropertyAt(propType, btypes.ArrayAll)
}

func (s *ObjectStore) getDevicePropertyAt(propType btypes.PropertyType, arrayIndex uint32) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if propType == btypes.PROP_OBJECT_LIST {
		list := s.getObjectListIdsLocked()
		switch {
		case arrayIndex == 0:
			return uint32(len(list)), true
		case arrayIndex == btypes.ArrayAll:
			return list, true
		case arrayIndex >= 1 && int(arrayIndex) <= len(list):
			return list[arrayIndex-1], true
		default:
			return nil, false
		}
	}

	if val, ok := s.deviceProperties[propType]; ok {
		return val, true
	}

	return nil, false
}

// getObjectListIds returns the list of object IDs for the device.
func (s *ObjectStore) getObjectListIds() []btypes.ObjectID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getObjectListIdsLocked()
}

func (s *ObjectStore) getObjectListIdsLocked() []btypes.ObjectID {
	list := []btypes.ObjectID{
		{
			Type:     btypes.DeviceType,
			Instance: s.deviceID,
		},
	}
	for objType, typeMap := range s.objects {
		for instance := range typeMap {
			list = append(list, btypes.ObjectID{
				Type:     objType,
				Instance: instance,
			})
		}
	}
	// Stable order helps browsers (YABE) and tests: Device first, then type, then instance.
	sort.Slice(list[1:], func(i, j int) bool {
		a, b := list[i+1], list[j+1]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Instance < b.Instance
	})
	return list
}

// incrementDatabaseRevision bumps the database revision counter.
func (s *ObjectStore) incrementDatabaseRevision() {
	if rev, ok := s.deviceProperties[btypes.PROP_DATABASE_REVISION].(uint32); ok {
		s.deviceProperties[btypes.PROP_DATABASE_REVISION] = rev + 1
	}
}

// defaultProperties returns default properties for a newly created object.
func (s *ObjectStore) defaultProperties(objType btypes.ObjectType, instance btypes.ObjectInstance) []btypes.Property {
	props := []btypes.Property{
		{
			Type:       btypes.PROP_OBJECT_IDENTIFIER,
			ArrayIndex: btypes.ArrayAll,
			Data: btypes.ObjectID{
				Type:     objType,
				Instance: instance,
			},
		},
		{
			Type:       btypes.PROP_OBJECT_NAME,
			ArrayIndex: btypes.ArrayAll,
			Data:       fmt.Sprintf("%s-%d", objType.String(), instance),
		},
		{
			Type:       btypes.PROP_OBJECT_TYPE,
			ArrayIndex: btypes.ArrayAll,
			Data:       btypes.Enumerated(objType),
		},
		{
			Type:       btypes.PROP_DESCRIPTION,
			ArrayIndex: btypes.ArrayAll,
			Data:       fmt.Sprintf("%s instance %d", objType.String(), instance),
		},
		{
			Type:       btypes.PROP_PRESENT_VALUE,
			ArrayIndex: btypes.ArrayAll,
			Data:       float64(0.0),
		},
		{
			Type:       btypes.PROP_STATUS_FLAGS,
			ArrayIndex: btypes.ArrayAll,
			Data:       btypes.NewBitString(4),
		},
		{
			Type:       btypes.PROP_EVENT_STATE,
			ArrayIndex: btypes.ArrayAll,
			Data:       uint32(0), // normal
		},
		{
			Type:       btypes.PROP_RELIABILITY,
			ArrayIndex: btypes.ArrayAll,
			Data:       uint32(0), // no fault detected
		},
		{
			Type:       btypes.PROP_OUT_OF_SERVICE,
			ArrayIndex: btypes.ArrayAll,
			Data:       false,
		},
		{
			Type:       btypes.PROP_Propertybtypes,
			ArrayIndex: btypes.ArrayAll,
			Data:       uint32(95), // no-units
		},
	}

	return props
}