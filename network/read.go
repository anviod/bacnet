package network

import (
	"github.com/anviod/bacnet/btypes"
	log "github.com/anviod/bacnet/helpers/log"
	"go.uber.org/zap"
)

type Object struct {
	ObjectID   btypes.ObjectInstance `json:"object_id"`
	ObjectType btypes.ObjectType     `json:"object_type"`
	Prop       btypes.PropertyType   `json:"prop"`
	ArrayIndex uint32                `json:"array_index"`
}

func (device *Device) ReadMuti(data btypes.MultiplePropertyData) (out btypes.MultiplePropertyData, err error) {
	out, err = device.network.ReadMultiProperty(device.dev, data)
	if err != nil {
		log.Logger.Error("network.Read() failed", zap.Error(err))
		return out, err
	}
	return
}

func (device *Device) ReadSingle(data btypes.PropertyData) (out btypes.PropertyData, err error) {
	out, err = device.network.ReadProperty(device.dev, data)
	if err != nil {
		log.Logger.Error("network.Read() failed", zap.Error(err))
		return out, err
	}
	return out, nil
}

func (device *Device) Read(obj *Object) (out btypes.PropertyData, err error) {
	if obj == nil {
		return out, ObjectNil
	}
	//get object list
	rp := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     obj.ObjectType,
				Instance: obj.ObjectID,
			},
			Properties: []btypes.Property{
				{
					Type:       obj.Prop,
					ArrayIndex: obj.ArrayIndex, //bacnet.ArrayAll
				},
			},
		},
	}
	out, err = device.network.ReadProperty(device.dev, rp)
	if err != nil {
		if rp.Object.Properties[0].Type == btypes.PropObjectList {
			log.Logger.Error("network.Read(): PropObjectList reads may need to be broken up into multiple reads due to length. Read index 0 for array length", zap.Error(err))
		} else {
			log.Logger.Error("network.Read() failed", zap.Error(err))
		}
		return out, err
	}
	if len(out.Object.Properties) == 0 {
		log.Logger.Error("network.Read(): no values returned")
		return out, nil
	}
	return out, nil
}
