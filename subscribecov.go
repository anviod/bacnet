package bacnet

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/encoding"
)

// SubscribeCOV sends a SubscribeCOV request and waits for SimpleAck.
// After success, the server typically sends an initial COV notification;
// use WaitCOVNotification (or drain COVNotifications) to receive it.
//
// 中文说明：SubscribeCOV 发送订阅请求并等待 SimpleAck。
func (c *client) SubscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error {
	return c.subscribeCOV(device, data)
}

// CancelSubscribeCOV cancels an existing COV subscription.
//
// 中文说明：CancelSubscribeCOV 取消已有 COV 订阅。
func (c *client) CancelSubscribeCOV(device btypes.Device, processID uint32, objectID btypes.ObjectID) error {
	return c.subscribeCOV(device, btypes.SubscribeCOVData{
		ProcessID:    processID,
		ObjectID:     objectID,
		Cancellation: true,
	})
}

func (c *client) subscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error {
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
	enc.SubscribeCOV(uint8(id), data)
	if enc.Error() != nil {
		return enc.Error()
	}

	err = fmt.Errorf("go")
	for count := 0; err != nil && count < 2; count++ {
		var b []byte
		var raw interface{}
		_, err = c.Send(device.Addr, npdu, enc.Bytes(), nil)
		if err != nil {
			continue
		}
		raw, err = c.tsm.Receive(id, 5*time.Second)
		if err != nil {
			continue
		}
		switch v := raw.(type) {
		case error:
			return v
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
		return nil
	}
	return err
}

// WaitCOVNotification waits up to timeout for the next COV notification.
// If processIDFilter >= 0, only notifications with that process ID are accepted
// (pass -1 to accept any).
//
// 中文说明：WaitCOVNotification 等待下一条 COV 通知；processIDFilter>=0 时按进程号过滤。
func (c *client) WaitCOVNotification(processIDFilter int64, timeout time.Duration) (btypes.COVNotification, error) {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return btypes.COVNotification{}, fmt.Errorf("COV notification timeout")
		case n := <-c.covCh:
			if processIDFilter >= 0 && int64(n.ProcessID) != processIDFilter {
				continue
			}
			return n, nil
		}
	}
}

func (c *client) publishCOV(n btypes.COVNotification) {
	select {
	case c.covCh <- n:
	default:
		// Drop if consumer is not waiting; avoids blocking the receive loop.
	}
}

func (c *client) handleConfirmedCOVNotification(src *btypes.Address, apdu *btypes.APDU, npdu *btypes.NPDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var n btypes.COVNotification
	if err := dec.COVNotification(&n); err != nil {
		return
	}
	n.Confirmed = true
	c.publishCOV(n)

	// SimpleAck for confirmed COV notification
	enc := encoding.NewEncoder()
	ack := btypes.APDU{
		DataType: btypes.SimpleAck,
		Service:  btypes.ServiceConfirmedCOVNotification,
		InvokeId: apdu.InvokeId,
	}
	_ = enc.APDU(ack)
	respNPDU := &btypes.NPDU{
		Version:               btypes.ProtocolVersion,
		IsNetworkLayerMessage: false,
		ExpectingReply:        false,
		Priority:              btypes.Normal,
		HopCount:              btypes.DefaultHopCount,
	}
	full := encoding.NewEncoder()
	full.NPDU(respNPDU)
	payload := append(full.Bytes(), enc.Bytes()...)
	_, _ = c.Send(*src, respNPDU, payload, nil)
	_ = npdu
}