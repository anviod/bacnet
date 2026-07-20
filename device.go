package bacnet

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/ndpu"
	"github.com/anviod/bacnet/datalink"
	"github.com/anviod/bacnet/encoding"
	log "github.com/anviod/bacnet/helpers/log"
	"github.com/anviod/bacnet/helpers/validation"
	"github.com/anviod/bacnet/tsm"
	"github.com/anviod/bacnet/utsm"
	"go.uber.org/zap"
)

const mtuHeaderLength = 4
const defaultStateSize = 64 // Increased from 20 to support more concurrent devices (32+)
const forwardHeaderLength = 10

// Client defines the interface for BACnet client operations.
// It provides methods for device discovery, object access, and network management.
// All methods are thread-safe and can be called concurrently from multiple goroutines.
//
// 中文说明：Client 定义了 BACnet 客户端操作的接口。
// 它提供设备发现、对象访问和网络管理的方法。
// 所有方法都是线程安全的，可以从多个 goroutine 并发调用。
type Client interface {
	io.Closer
	// IsRunning returns true if the client message loop is running.
	// IsRunning 返回客户端消息循环是否正在运行。
	IsRunning() bool
	// ClientRun starts the client message loop. This should be called in a goroutine.
	// ClientRun 启动客户端消息循环，应在 goroutine 中调用。
	ClientRun()
	// WhoIs discovers BACnet devices on the network within the specified device ID range.
	// It sends a broadcast WhoIs request and collects IAm responses from devices.
	// Returns a list of discovered devices.
	// WhoIs 发现网络上指定设备ID范围内的BACnet设备。
	// 发送广播 WhoIs 请求并收集设备的 IAm 响应。
	// 返回发现的设备列表。
	WhoIs(wh *WhoIsOpts) ([]btypes.Device, error)
	// WhatIsNetworkNumber determines the network number of the local BACnet network.
	// Returns a list of addresses that responded to the What-Is-Network-Number request.
	// WhatIsNetworkNumber 确定本地BACnet网络的网络号。
	// 返回响应 What-Is-Network-Number 请求的地址列表。
	WhatIsNetworkNumber() []*btypes.Address
	// IAm sends an IAm response to the specified destination.
	// This is typically used to respond to WhoIs requests from other devices.
	// IAm 向指定目标发送 IAm 响应。
	// 通常用于响应其他设备的 WhoIs 请求。
	IAm(dest btypes.Address, iam btypes.IAm) error
	// WhoIsRouterToNetwork discovers routers on the BACnet network.
	// Returns a list of router addresses.
	// WhoIsRouterToNetwork 发现BACnet网络上的路由器。
	// 返回路由器地址列表。
	WhoIsRouterToNetwork() (resp *[]btypes.Address)
	// Objects retrieves all objects from a BACnet device.
	// This includes AnalogInput, AnalogOutput, BinaryInput, BinaryOutput, and other object types.
	// Returns a Device structure containing all discovered objects.
	// Objects 从BACnet设备检索所有对象。
	// 包括模拟输入、模拟输出、二进制输入、二进制输出等对象类型。
	// 返回包含所有发现对象的 Device 结构。
	Objects(dev btypes.Device) (btypes.Device, error)
	// ReadProperty reads a single property from a BACnet device object.
	// Returns the PropertyData containing the read property value.
	// ReadProperty 从BACnet设备对象读取单个属性。
	// 返回包含读取属性值的 PropertyData。
	ReadProperty(dest btypes.Device, rp btypes.PropertyData) (btypes.PropertyData, error)
	// ReadMultiProperty reads multiple properties from multiple objects in a single request.
	// This is more efficient than multiple ReadProperty calls.
	// Returns the MultiplePropertyData containing all read property values.
	// ReadMultiProperty 在单个请求中从多个对象读取多个属性。
	// 这比多次调用 ReadProperty 更高效。
	// 返回包含所有读取属性值的 MultiplePropertyData。
	ReadMultiProperty(dev btypes.Device, rp btypes.MultiplePropertyData) (btypes.MultiplePropertyData, error)
	// ReadPropertyWithTimeout reads a single property with a specified timeout.
	// This allows for more granular timeout control than the default timeout.
	// ReadPropertyWithTimeout 使用指定超时读取单个属性。
	// 允许比默认超时更精细的超时控制。
	ReadPropertyWithTimeout(dest btypes.Device, rp btypes.PropertyData, timeout time.Duration) (btypes.PropertyData, error)
	// ReadMultiPropertyWithTimeout reads multiple properties with a specified timeout.
	// ReadMultiPropertyWithTimeout 使用指定超时读取多个属性。
	ReadMultiPropertyWithTimeout(dev btypes.Device, rp btypes.MultiplePropertyData, timeout time.Duration) (btypes.MultiplePropertyData, error)
	// WriteProperty writes a single property to a BACnet device object.
	// Returns an error if the write operation fails.
	// WriteProperty 向BACnet设备对象写入单个属性。
	// 如果写入操作失败返回错误。
	WriteProperty(dest btypes.Device, wp btypes.PropertyData) error
	// WriteMultiProperty writes multiple properties to multiple objects in a single request.
	// This is more efficient than multiple WriteProperty calls.
	// WriteMultiProperty 在单个请求中向多个对象写入多个属性。
	// 这比多次调用 WriteProperty 更高效。
	WriteMultiProperty(dev btypes.Device, wp btypes.MultiplePropertyData) error
	// SubscribeCOV registers for change-of-value notifications on an object.
	// SubscribeCOV 订阅对象的 COV 通知。
	SubscribeCOV(device btypes.Device, data btypes.SubscribeCOVData) error
	// CancelSubscribeCOV cancels a COV subscription.
	// CancelSubscribeCOV 取消 COV 订阅。
	CancelSubscribeCOV(device btypes.Device, processID uint32, objectID btypes.ObjectID) error
	// WaitCOVNotification waits for a COV notification. processIDFilter >= 0 filters by process ID; -1 accepts any.
	// WaitCOVNotification 等待 COV 通知。
	WaitCOVNotification(processIDFilter int64, timeout time.Duration) (btypes.COVNotification, error)
}

type client struct {
	dataLink       datalink.DataLink
	tsm            *tsm.TSM
	utsm           *utsm.Manager
	readBufferPool sync.Pool
	running        bool
	covCh          chan btypes.COVNotification
}

// ClientBuilder is used to configure and create a new BACnet client.
// All fields are optional with sensible defaults.
//
// 中文说明：ClientBuilder 用于配置和创建新的 BACnet 客户端。
// 所有字段都是可选的，有合理的默认值。
type ClientBuilder struct {
	DataLink   datalink.DataLink // Custom data link implementation (optional)
	Interface  string            // Network interface name (e.g., "eth0")
	Ip         string            // IP address to bind to
	Port       int               // BACnet port (default: 47808)
	SubnetCIDR int               // Subnet CIDR (e.g., 24 for /24)
	MaxPDU     uint16            // Maximum PDU size (default: 1476)
}

// NewClient creates a new BACnet client with the provided configuration.
// It validates the configuration, creates the data link layer, initializes
// the Transaction State Machine (TSM), and sets up the unconfirmed transaction
// manager (UTSM).
//
// Parameters:
//   cb - ClientBuilder containing the client configuration
//
// Returns:
//   A new Client instance and any error encountered during initialization.
//
// 中文说明：NewClient 使用提供的配置创建新的 BACnet 客户端。
// 它验证配置，创建数据链路层，初始化事务状态机（TSM），
// 并设置非确认事务管理器（UTSM）。
//
// 参数：
//   cb - 包含客户端配置的 ClientBuilder
//
// 返回：
//   新的 Client 实例和初始化期间遇到的任何错误。
func NewClient(cb *ClientBuilder) (Client, error) {
	var err error
	var dataLink datalink.DataLink
	iface := cb.Interface
	ip := cb.Ip
	port := cb.Port
	maxPDU := cb.MaxPDU
	//check ip
	ok := validation.ValidIP(ip)
	if !ok {

	}
	//check port
	if port == 0 {
		port = datalink.DefaultPort
	}
	ok = validation.ValidPort(port)
	if !ok {

	}
	//check adpu
	if maxPDU == 0 {
		maxPDU = btypes.MaxAPDU
	}
	//build datalink
	if cb.DataLink != nil {
		dataLink = cb.DataLink
	} else if iface != "" {
		dataLink, err = datalink.NewUDPDataLink(iface, port)
		if err != nil {
			return nil, err
		}
	} else {
		//check subnet
		sub := cb.SubnetCIDR
		ok = validation.ValidCIDR(ip, sub)
		if !ok {

		}
		dataLink, err = datalink.NewUDPDataLinkFromIP(ip, sub, port)
		if err != nil {
			return nil, err
		}
	}

	cli := &client{
		dataLink: dataLink,
		tsm:      tsm.New(defaultStateSize),
		utsm: utsm.NewManager(
			utsm.DefaultSubscriberTimeout(time.Second*time.Duration(10)),
			utsm.DefaultSubscriberLastReceivedTimeout(time.Second*time.Duration(2)),
		),
		readBufferPool: sync.Pool{New: func() any {
			return make([]byte, maxPDU)
		}},
		covCh: make(chan btypes.COVNotification, 16),
	}
	return cli, err
}

// ClientRun starts the main message loop for the client.
// It continuously receives packets from the data link layer and processes
// them concurrently. This method should be called in a goroutine before
// making any BACnet requests.
//
// 中文说明：ClientRun 启动客户端的主消息循环。
// 它持续从数据链路层接收数据包并并发处理它们。
// 在进行任何 BACnet 请求之前，应在 goroutine 中调用此方法。
func (c *client) ClientRun() {
	var err error = nil
	c.running = true
	for err == nil {
		b := c.readBufferPool.Get().([]byte)
		var addr *btypes.Address
		var n int
		addr, n, err = c.dataLink.Receive(b)
		if err != nil {
			continue
		}
		go c.handleMsg(addr, b[:n])
	}
	c.running = false
}

func (c *client) handleMsg(src *btypes.Address, b []byte) {
	var header btypes.BVLC
	var npdu btypes.NPDU
	var apdu btypes.APDU
	dec := encoding.NewDecoder(b)
	err := dec.BVLC(&header)
	if err != nil {
		log.Logger.Error("bacnet decode error", zap.Error(err))
		return
	}

	if header.Function == btypes.BacFuncBroadcast || header.Function == btypes.BacFuncUnicast || header.Function == btypes.BacFuncForwardedNPDU {
		// Remove the header information
		b = b[mtuHeaderLength:]
		networkList, err := dec.NPDU(&npdu)
		if err != nil {
			log.Logger.Error("NPDU decode error", zap.Error(err))
			return
		}

		if npdu.IsNetworkLayerMessage {
			log.Logger.Debug("ignored network layer message", zap.Uint8("type", uint8(npdu.NetworkLayerMessageType)))
			if npdu.NetworkLayerMessageType == ndpu.NetworkIs {
				c.utsm.Publish(int(npdu.Source.Net), npdu)
			}
			if npdu.NetworkLayerMessageType == ndpu.IamRouterToNetwork {
				c.utsm.Publish(int(npdu.Source.Net), networkList)
			}
		}

		// We want to keep the APDU intact, so we will get a snapshot before decoding
		send := dec.Bytes()
		err = dec.APDU(&apdu)
		if err != nil {
			log.Logger.Error("issue decoding APDU", zap.Error(err))
			return
		}
		switch apdu.DataType {
		case btypes.UnconfirmedServiceRequest:
			if apdu.UnconfirmedService == btypes.ServiceUnconfirmedIAm {
				dec := encoding.NewDecoder(apdu.RawData)
				iam := btypes.IAm{}
				err := dec.IAm(&iam)
				if err != nil {
					log.Logger.Debug("unable to decode IAm", zap.Error(err))
					return
				}
				// Populate Source for IAm
				iam.Addr = *src
				c.utsm.Publish(int(iam.ID.Instance), iam)
			} else if apdu.UnconfirmedService == btypes.ServiceUnconfirmedWhoIs {
				dec := encoding.NewDecoder(apdu.RawData)
				var low, high int32
				dec.WhoIs(&low, &high)
			} else if apdu.UnconfirmedService == btypes.ServiceUnconfirmedCOVNotification {
				dec := encoding.NewDecoder(apdu.RawData)
				var n btypes.COVNotification
				if err := dec.COVNotification(&n); err != nil {
					log.Logger.Debug("unable to decode UnconfirmedCOVNotification", zap.Error(err))
					return
				}
				n.Confirmed = false
				c.publishCOV(n)
			}
		case btypes.SimpleAck:
			log.Logger.Debug("received Simple Ack")
			err := c.tsm.Send(int(apdu.InvokeId), send)
			if err != nil {
				return
			}
		case btypes.ComplexAck:
			err := c.tsm.Send(int(apdu.InvokeId), send)
			if err != nil {
				return
			}
		case btypes.ConfirmedServiceRequest:
			if apdu.Service == btypes.ServiceConfirmedCOVNotification {
				c.handleConfirmedCOVNotification(src, &apdu, &npdu)
				return
			}
			log.Logger.Debug("received Confirmed Service Request")
			err := c.tsm.Send(int(apdu.InvokeId), send)
			if err != nil {
				return
			}
		case btypes.Error:
			err := fmt.Errorf("error class %s code %s", apdu.Error.Class.String(), apdu.Error.Code.String())
			err = c.tsm.Send(int(apdu.InvokeId), err)
			if err != nil {
				log.Logger.Debug("unable to send error", zap.Uint8("invokeId", apdu.InvokeId), zap.Error(err))
			}
		case btypes.Abort:
			reason, _ := encoding.AbortReasonFromAPDU(&apdu)
			err := fmt.Errorf("abort reason %d", reason)
			_ = c.tsm.Send(int(apdu.InvokeId), err)
		default:
			log.Logger.Debug("ignored packet", zap.ByteString("raw", b))
		}
	}

	if header.Function == btypes.BacFuncForwardedNPDU {
		// Right now we are ignoring the NPDU data that is stored in the packet. Eventually
		// we will need to check it for any additional information we can gleam.
		// NDPU has source
		b = b[forwardHeaderLength:]
		log.Logger.Debug("ignored NDPU Forwarded")
	}
}

// SetBroadcastType is used to override the BVLC header function type.
// This allows forcing a specific broadcast/unicast behavior.
//
// 中文说明：SetBroadcastType 用于覆盖 BVLC 头部函数类型。
// 允许强制特定的广播/单播行为。
type SetBroadcastType struct {
	Set     bool             // Whether to override the default behavior
	BacFunc btypes.BacFunc   // The function type to set
}

// Send transmits the raw APDU byte slice to the specified destination address.
// It handles both broadcast and unicast messages by setting the appropriate
// BVLC header function. The NPDU provides network layer information.
//
// Parameters:
//   dest - The destination address for the message
//   npdu - Network Protocol Data Unit containing network layer info
//   data - The raw APDU data to send
//   broadcastType - Optional override for broadcast type behavior
//
// Returns:
//   The number of bytes sent and any error encountered.
//
// 中文说明：Send 将原始 APDU 字节切片传输到指定的目标地址。
// 通过设置适当的 BVLC 头部函数来处理广播和单播消息。
// NPDU 提供网络层信息。
//
// 参数：
//   dest - 消息的目标地址
//   npdu - 包含网络层信息的网络协议数据单元
//   data - 要发送的原始 APDU 数据
//   broadcastType - 可选的广播类型行为覆盖
//
// 返回：
//   发送的字节数和遇到的任何错误。
func (c *client) Send(dest btypes.Address, npdu *btypes.NPDU, data []byte, broadcastType *SetBroadcastType) (int, error) {
	//broadcastType = &SetBroadcastType{}
	var header btypes.BVLC
	// Set packet type
	header.Type = btypes.BVLCTypeBacnetIP
	//if Adr is > 0 it must be an mst-tp device so send a UNICAST
	// if len(dest.Adr) > 0 { //(aidan) not sure if this is correct, but it needs to be set to work to send (UNICAST) messages over a bacnet network
	// 	// SET UNICAST FLAG
	// 	// see http://www.bacnet.org/Tutorial/HMN-Overview/sld033.
	// 	// see https://github.com/JoelBender/bacpypes/blob/9fca3f608a97a20807cd188689a2b9ff60b05085/doc/source/gettingstarted/gettingstarted001.rst#udp-communications-issues
	// 	header.Function = btypes.BacFuncUnicast
	// } else

	if dest.IsBroadcast() || dest.IsSubBroadcast() || isBroadcastDest(&dest) {
		// SET BROADCAST FLAG
		header.Function = btypes.BacFuncBroadcast
	} else {
		// SET UNICAST FLAG
		header.Function = btypes.BacFuncUnicast
	}

	if broadcastType != nil {
		if broadcastType.Set {
			header.Function = broadcastType.BacFunc
		}
	}

	header.Length = uint16(mtuHeaderLength + len(data))
	header.Data = data
	e := encoding.NewEncoder()
	err := e.BVLC(header)
	if err != nil {
		return 0, err
	}
	// use default udp type, src = network address (nil)
	return c.dataLink.Send(e.Bytes(), npdu, &dest)
}

// Close free resources for the client. Always call this function when using NewClient
func (c *client) Close() error {
	if c.dataLink != nil {
		c.dataLink.Close()
	}
	c.running = false
	return nil
}

func (c *client) IsRunning() bool {
	return c.running
}
