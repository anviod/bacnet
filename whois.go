package bacnet

import (
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/encoding"
)

type WhoIsOpts struct {
	Low             int             `json:"low"`
	High            int             `json:"high"`
	GlobalBroadcast bool            `json:"global_broadcast"`
	NetworkNumber   uint16          `json:"network_number"`
	Destination     *btypes.Address `json:"-"`
}

// WhoIs finds all devices with ids between the provided low and high values.
// Use constant ArrayAll for both fields to scan the entire network at once.
// Using ArrayAll is highly discouraged for most networks since it can lead
// to a highly congested network.
func (c *client) WhoIs(wh *WhoIsOpts) ([]btypes.Device, error) {
	dest := *c.dataLink.GetBroadcastAddress()
	if wh.Destination != nil {
		dest = *wh.Destination
	}

	enc := encoding.NewEncoder()
	low := wh.Low
	high := wh.High
	if wh.GlobalBroadcast {
		wh.NetworkNumber = btypes.GlobalBroadcast
	}
	if low <= 0 {
		low = 0
	}
	if high <= 0 {
		high = 4194304
	}

	dest.Net = wh.NetworkNumber
	npdu := &btypes.NPDU{
		Version:               btypes.ProtocolVersion,
		Destination:           &dest,
		Source:                c.dataLink.GetMyAddress(),
		IsNetworkLayerMessage: false,
		ExpectingReply:        false,
		Priority:              btypes.Normal,
		HopCount:              btypes.DefaultHopCount,
	}
	enc.NPDU(npdu)
	err := enc.WhoIs(int32(low), int32(high))
	if err != nil {
		return nil, err
	}

	var start, end int
	if low == -1 || high == -1 {
		start = 0
		end = maxInt
	} else {
		start = low
		end = high
	}

	errChan := make(chan error)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, err = c.Send(dest, npdu, enc.Bytes(), nil)
		errChan <- err
	}()
	values, err := c.utsm.Subscribe(start, end)
	if err != nil {
		return nil, err
	}
	err = <-errChan
	if err != nil {
		return nil, err
	}

	uniqueMap := make(map[btypes.ObjectInstance]btypes.Device)
	var uniqueList []btypes.Device

	for _, v := range values {
		iam, ok := v.(btypes.IAm)
		if !ok {
			continue
		}
		if _, ok := uniqueMap[iam.ID.Instance]; ok {
			continue
		}

		dev := btypes.Device{
			DeviceID:     int(iam.ID.Instance),
			Addr:         iam.Addr,
			ID:           iam.ID,
			MaxApdu:      iam.MaxApdu,
			Segmentation: iam.Segmentation,
			Vendor:       iam.Vendor,
		}

		// Some BACnet/IP stacks send I-Am from an ephemeral UDP source port while
		// still accepting confirmed services on the configured BACnet/IP port.
		// When the caller used a unicast Who-Is destination, keep that address as
		// the confirmed-service destination instead of blindly copying the I-Am
		// packet source address.
		if wh.Destination != nil && !wh.Destination.IsBroadcast() {
			dev.Addr = *wh.Destination
		}
		if udpAddr, err := dev.Addr.UDPAddr(); err == nil {
			dev.Ip = udpAddr.IP.String()
			dev.Port = udpAddr.Port
		}

		uniqueMap[iam.ID.Instance] = dev
		uniqueList = append(uniqueList, dev)
	}
	return uniqueList, err
}
