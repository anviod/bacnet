package encoding

import (
	"fmt"

	"github.com/anviod/bacnet/btypes"
)

// SubscribeCOV encodes a SubscribeCOV confirmed service request.
//
// ASN.1:
//
//	subscriberProcessIdentifier [0] Unsigned32
//	monitoredObjectIdentifier   [1] BACnetObjectIdentifier
//	issueConfirmedNotifications [2] BOOLEAN        -- absent on cancel
//	lifetime                    [3] Unsigned32     -- absent on cancel
func (e *Encoder) SubscribeCOV(invokeID uint8, data btypes.SubscribeCOVData) error {
	a := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedSubscribeCOV,
		MaxSegs:  0,
		MaxApdu:  MaxAPDU,
		InvokeId: invokeID,
	}
	e.APDU(a)

	e.contextUnsigned(0, data.ProcessID)
	e.contextObjectID(1, data.ObjectID.Type, data.ObjectID.Instance)

	if !data.Cancellation {
		e.contextBoolean(2, data.IssueConfirmedNotifications)
		e.contextUnsigned(3, data.Lifetime)
	}
	return e.Error()
}

// SubscribeCOV decodes a SubscribeCOV request body (APDU RawData).
func (d *Decoder) SubscribeCOV(data *btypes.SubscribeCOVData) error {
	if d.len() < 6 {
		return fmt.Errorf("SubscribeCOV: missing parameters")
	}

	tag, meta, value := d.tagNumberAndValue()
	if tag != 0 || !meta.isContextSpecific() {
		return &ErrorIncorrectTag{Expected: 0, Given: tag}
	}
	data.ProcessID = d.unsigned(int(value))

	tag, meta = d.tagNumber()
	if tag != 1 || !meta.isContextSpecific() {
		return &ErrorIncorrectTag{Expected: 1, Given: tag}
	}
	objType, instance := d.objectId()
	data.ObjectID = btypes.ObjectID{Type: objType, Instance: instance}

	if d.len() == 0 {
		data.Cancellation = true
		return d.Error()
	}

	tag, meta, value = d.tagNumberAndValue()
	if tag != 2 || !meta.isContextSpecific() {
		// Unexpected leftover data without tag 2 → treat as cancel if empty after object
		return fmt.Errorf("SubscribeCOV: expected tag 2 (issueConfirmedNotifications), got %d", tag)
	}
	data.IssueConfirmedNotifications = d.contextBooleanValue(meta, value)
	data.Cancellation = false

	if d.len() == 0 {
		return fmt.Errorf("SubscribeCOV: missing lifetime")
	}
	tag, meta, value = d.tagNumberAndValue()
	if tag != 3 || !meta.isContextSpecific() {
		return &ErrorIncorrectTag{Expected: 3, Given: tag}
	}
	data.Lifetime = d.unsigned(int(value))
	return d.Error()
}

// contextBoolean encodes a context-tagged boolean (length=1, value 0/1).
func (e *Encoder) contextBoolean(tagNumber uint8, value bool) {
	e.tag(tagInfo{ID: tagNumber, Context: true, Value: 1})
	if value {
		e.write(uint8(1))
	} else {
		e.write(uint8(0))
	}
}

func (d *Decoder) contextBooleanValue(meta tagMeta, length uint32) bool {
	// Some stacks put the boolean in the LVT field (length 0/1 with no data).
	if length <= 1 && d.len() == 0 {
		return length == 1
	}
	if length == 0 {
		return false
	}
	if length == 1 && !meta.isOpening() {
		var b uint8
		d.decode(&b)
		return b != 0
	}
	// Fallback: boolean encoded like application (value in LVT)
	return length != 0
}
