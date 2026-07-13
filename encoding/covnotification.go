package encoding

import (
	"fmt"

	"github.com/anviod/bacnet/btypes"
)

// UnconfirmedCOVNotification encodes an UnconfirmedCOVNotification request.
func (e *Encoder) UnconfirmedCOVNotification(data btypes.COVNotification) error {
	a := btypes.APDU{
		DataType:           btypes.UnconfirmedServiceRequest,
		UnconfirmedService: btypes.ServiceUnconfirmedCOVNotification,
	}
	e.APDU(a)
	e.encodeCOVNotificationBody(data)
	return e.Error()
}

// ConfirmedCOVNotification encodes a ConfirmedCOVNotification request.
func (e *Encoder) ConfirmedCOVNotification(invokeID uint8, data btypes.COVNotification) error {
	a := btypes.APDU{
		DataType: btypes.ConfirmedServiceRequest,
		Service:  btypes.ServiceConfirmedCOVNotification,
		MaxSegs:  0,
		MaxApdu:  MaxAPDU,
		InvokeId: invokeID,
	}
	e.APDU(a)
	e.encodeCOVNotificationBody(data)
	return e.Error()
}

func (e *Encoder) encodeCOVNotificationBody(data btypes.COVNotification) {
	// [0] subscriberProcessIdentifier
	e.contextUnsigned(0, data.ProcessID)
	// [1] initiatingDeviceIdentifier
	e.contextObjectID(1, data.InitiatingDeviceID.Type, data.InitiatingDeviceID.Instance)
	// [2] monitoredObjectIdentifier
	e.contextObjectID(2, data.MonitoredObjectID.Type, data.MonitoredObjectID.Instance)
	// [3] timeRemaining
	e.contextUnsigned(3, data.TimeRemaining)
	// [4] listOfValues — SEQUENCE OF BACnetPropertyValue
	e.openingTag(4)
	for _, prop := range data.ListOfValues {
		e.contextEnumerated(0, uint32(prop.Type))
		if prop.ArrayIndex != ArrayAll {
			e.contextUnsigned(1, prop.ArrayIndex)
		}
		e.openingTag(2)
		e.AppData(prop.Data, false)
		e.closingTag(2)
	}
	e.closingTag(4)
}

// COVNotification decodes a COV notification body (APDU RawData).
func (d *Decoder) COVNotification(data *btypes.COVNotification) error {
	if d.len() < 10 {
		return fmt.Errorf("COVNotification: missing parameters")
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
	t, inst := d.objectId()
	data.InitiatingDeviceID = btypes.ObjectID{Type: t, Instance: inst}

	tag, meta = d.tagNumber()
	if tag != 2 || !meta.isContextSpecific() {
		return &ErrorIncorrectTag{Expected: 2, Given: tag}
	}
	t, inst = d.objectId()
	data.MonitoredObjectID = btypes.ObjectID{Type: t, Instance: inst}

	tag, meta, value = d.tagNumberAndValue()
	if tag != 3 || !meta.isContextSpecific() {
		return &ErrorIncorrectTag{Expected: 3, Given: tag}
	}
	data.TimeRemaining = d.unsigned(int(value))

	tag, meta = d.tagNumber()
	if tag != 4 || !meta.isOpening() {
		return fmt.Errorf("COVNotification: expected opening tag 4")
	}

	data.ListOfValues = nil
	for d.len() > 0 {
		// Peek: closing tag 4 ends the list
		peek, err := d.buff.ReadByte()
		if err != nil {
			break
		}
		_ = d.buff.UnreadByte()
		if isClosingTag(peek) {
			closeTag, closeMeta := d.tagNumber()
			if closeTag != 4 || !closeMeta.isClosing() {
				return fmt.Errorf("COVNotification: expected closing tag 4")
			}
			break
		}

		tag, meta, value = d.tagNumberAndValue()
		if tag != 0 || !meta.isContextSpecific() {
			return fmt.Errorf("COVNotification: expected property identifier tag 0")
		}
		prop := btypes.Property{
			Type:       btypes.PropertyType(d.enumerated(int(value))),
			ArrayIndex: ArrayAll,
		}

		tag, meta = d.tagNumber()
		if tag == 1 && meta.isContextSpecific() && !meta.isOpening() && !meta.isClosing() {
			lenVal := d.value(meta)
			prop.ArrayIndex = d.unsigned(int(lenVal))
			tag, meta = d.tagNumber()
		}

		if tag != 2 || !meta.isOpening() {
			return fmt.Errorf("COVNotification: expected opening tag 2 for value")
		}

		var values []interface{}
		for d.len() > 1 {
			peek, err := d.buff.ReadByte()
			if err != nil {
				break
			}
			_ = d.buff.UnreadByte()
			if isClosingTag(peek) {
				break
			}
			v, err := d.AppData()
			if err != nil {
				return err
			}
			values = append(values, v)
		}
		closeTag, closeMeta := d.tagNumber()
		if closeTag != 2 || !closeMeta.isClosing() {
			return fmt.Errorf("COVNotification: expected closing tag 2")
		}
		if len(values) == 1 {
			prop.Data = values[0]
		} else {
			prop.Data = values
		}
		data.ListOfValues = append(data.ListOfValues, prop)
	}

	return d.Error()
}
