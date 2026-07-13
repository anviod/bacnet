package server

import (
	"github.com/anviod/bacnet/btypes"
)

// mockDataLink implements datalink.DataLink for testing purposes.
// It captures sent data and allows injection of received data.
//
// 中文说明：mockDataLink 实现 datalink.DataLink 接口用于测试。
// 捕获发送的数据并允许注入接收的数据。
type mockDataLink struct {
	myAddr        *btypes.Address
	broadcastAddr *btypes.Address
	sentData      [][]byte
	sentNPDUs     []*btypes.NPDU
	sentDests     []*btypes.Address
	receiveCh     chan *receivePacket
	closed        bool
}

type receivePacket struct {
	data []byte
	addr *btypes.Address
	err  error
}

func newMockDataLink() *mockDataLink {
	return &mockDataLink{
		myAddr: &btypes.Address{
			Net:    0,
			MacLen: 6,
			Mac:    []byte{0, 0, 0, 0},
			Adr:    []byte{127, 0, 0, 1},
			Len:    4,
		},
		broadcastAddr: &btypes.Address{
			Net:    0,
			MacLen: 6,
			Mac:    []byte{0xFF, 0xFF, 0xFF, 0xFF},
			Adr:    []byte{255, 255, 255, 255},
			Len:    4,
		},
		receiveCh: make(chan *receivePacket, 10),
		sentData:  make([][]byte, 0),
		sentNPDUs: make([]*btypes.NPDU, 0),
		sentDests: make([]*btypes.Address, 0),
	}
}

func (m *mockDataLink) GetMyAddress() *btypes.Address {
	return m.myAddr
}

func (m *mockDataLink) GetBroadcastAddress() *btypes.Address {
	return m.broadcastAddr
}

func (m *mockDataLink) Send(data []byte, npdu *btypes.NPDU, dest *btypes.Address) (int, error) {
	d := make([]byte, len(data))
	copy(d, data)
	m.sentData = append(m.sentData, d)

	npduCopy := *npdu
	m.sentNPDUs = append(m.sentNPDUs, &npduCopy)

	destCopy := *dest
	m.sentDests = append(m.sentDests, &destCopy)

	return len(data), nil
}

func (m *mockDataLink) Receive(data []byte) (*btypes.Address, int, error) {
	if m.closed {
		return nil, 0, nil
	}
	pkt, ok := <-m.receiveCh
	if !ok {
		return nil, 0, nil
	}
	if pkt.err != nil {
		return nil, 0, pkt.err
	}
	n := copy(data, pkt.data)
	return pkt.addr, n, nil
}

func (m *mockDataLink) Close() error {
	m.closed = true
	close(m.receiveCh)
	return nil
}

// injectReceive injects a packet that will be returned by the next Receive call.
func (m *mockDataLink) injectReceive(addr *btypes.Address, data []byte) {
	m.receiveCh <- &receivePacket{
		addr: addr,
		data: data,
	}
}

// getLastSent returns the last sent data packet.
func (m *mockDataLink) getLastSent() []byte {
	if len(m.sentData) == 0 {
		return nil
	}
	return m.sentData[len(m.sentData)-1]
}

// getLastSentDest returns the last sent destination.
func (m *mockDataLink) getLastSentDest() *btypes.Address {
	if len(m.sentDests) == 0 {
		return nil
	}
	return m.sentDests[len(m.sentDests)-1]
}

// sentCount returns the number of sent packets.
func (m *mockDataLink) sentCount() int {
	return len(m.sentData)
}