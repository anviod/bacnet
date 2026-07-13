package bacnet

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/encoding"
)

func (c *client) WriteProperty(device btypes.Device, wp btypes.PropertyData) error {
	return c.writeProperty(device, wp, nil)
}

// WritePropertyBroadcast 以广播方式写入 BACnet 属性
// broadcastAddr: UDP 广播目标地址 (如 192.168.3.255:47808), 用于确保
// 共享同一端口的所有设备都能收到请求
// WritePropertyBroadcast writes a property using a broadcast UDP destination,
// useful when multiple BACnet devices share the same UDP port.
func (c *client) WritePropertyBroadcast(device btypes.Device, wp btypes.PropertyData, broadcastAddr btypes.Address) error {
	broadcastType := &SetBroadcastType{
		Set:     true,
		BacFunc: btypes.BacFuncBroadcast,
	}
	return c.writeProperty(device, wp, broadcastType, broadcastAddr)
}

func (c *client) writeProperty(device btypes.Device, wp btypes.PropertyData, broadcastType *SetBroadcastType, broadcastAddr ...btypes.Address) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, err := c.tsm.ID(ctx)
	if err != nil {
		return fmt.Errorf("unable to get an transaction id: %v", err)
	}
	defer c.tsm.Put(id)
	device.Addr.SetLength()
	npdu := &btypes.NPDU{
		Version:               btypes.ProtocolVersion,
		Destination:           &device.Addr,
		Source:                c.dataLink.GetMyAddress(),
		IsNetworkLayerMessage: false,
		ExpectingReply:        true,
		Priority:              btypes.Normal,
		HopCount:              btypes.DefaultHopCount,
	}
	enc := encoding.NewEncoder()
	enc.NPDU(npdu)
	enc.WriteProperty(uint8(id), wp)
	if enc.Error() != nil {
		return enc.Error()
	}
	// the value filled doesn't matter. it just needs to be non nil
	err = fmt.Errorf("go")
	for count := 0; err != nil && count < 2; count++ {
		var b []byte
		var raw interface{}
		dest := device.Addr
		if len(broadcastAddr) > 0 {
			dest = broadcastAddr[0]
		}
		_, err = c.Send(dest, npdu, enc.Bytes(), broadcastType)
		if err != nil {
			continue
		}
		raw, err = c.tsm.Receive(id, time.Duration(5)*time.Second)
		if err != nil {
			continue
		}
		switch v := raw.(type) {
		case error:
			if err == nil {
				err = raw.(error)
			}
			return err
		case []byte:
			b = v
		default:
			return fmt.Errorf("received unknown datatype %T", raw)
		}

		dec := encoding.NewDecoder(b)
		var apdu btypes.APDU
		if err = dec.APDU(&apdu); err != nil {
			continue
		}
		if apdu.Error.Class != 0 || apdu.Error.Code != 0 {
			err = fmt.Errorf("received error, class: %d, code: %d", apdu.Error.Class, apdu.Error.Code)
			continue
		}

		return err
	}
	return err
}
