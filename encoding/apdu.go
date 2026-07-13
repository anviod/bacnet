package encoding

import (
	"fmt"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/bacerr"
)

func (e *Encoder) APDU(a btypes.APDU) error {
	meta := APDUMetadata(0)
	meta.setDataType(a.DataType)
	meta.setMoreFollows(a.MoreFollows)
	meta.setSegmentedMessage(a.SegmentedMessage)
	meta.setSegmentedAccepted(a.SegmentedResponseAccepted)
	e.write(meta)

	switch a.DataType {
	case btypes.ComplexAck:
		e.apduComplexAck(a)
	case btypes.SimpleAck:
		e.apduSimpleAck(a)
	case btypes.UnconfirmedServiceRequest:
		e.apduUnconfirmed(a)
	case btypes.ConfirmedServiceRequest:
		e.apduConfirmed(a)
	case btypes.Error:
		e.apduError(a)
	case btypes.SegmentAck:
		return fmt.Errorf("decoded Segmented")
	case btypes.Reject:
		return fmt.Errorf("decoded Rejected")
	case btypes.Abort:
		e.apduAbort(a)
	default:
		return fmt.Errorf("unknown PDU type: %d", meta.DataType())
	}
	return nil
}

func (e *Encoder) apduAbort(a btypes.APDU) {
	// PDU type byte already written by APDU(); write invoke ID + reason from RawData[0].
	e.write(a.InvokeId)
	var reason uint8
	if len(a.RawData) > 0 {
		reason = a.RawData[0]
	} else {
		reason = uint8(AbortReasonOther)
	}
	e.write(reason)
}

func (e *Encoder) apduConfirmed(a btypes.APDU) {
	e.maxSegsMaxApdu(a.MaxSegs, a.MaxApdu)
	e.write(a.InvokeId)
	if a.SegmentedMessage {
		e.write(a.Sequence)
		e.write(a.WindowNumber)
	}
	e.write(a.Service)
}

func (e *Encoder) apduUnconfirmed(a btypes.APDU) {
	e.write(a.UnconfirmedService)
}

func (e *Encoder) apduComplexAck(a btypes.APDU) {
	e.write(a.InvokeId)
	e.write(a.Service)
}

func (e *Encoder) apduSimpleAck(a btypes.APDU) {
	e.write(a.InvokeId)
	e.write(a.Service)
}

func (e *Encoder) apduError(a btypes.APDU) {
	e.write(a.InvokeId)
	e.write(a.Service)
	_ = e.AppData(btypes.Enumerated(a.Error.Class), false)
	_ = e.AppData(btypes.Enumerated(a.Error.Code), false)
}

func (d *Decoder) APDU(a *btypes.APDU) error {
	var meta APDUMetadata
	d.decode(&meta)
	a.SegmentedMessage = meta.isSegmentedMessage()
	a.SegmentedResponseAccepted = meta.segmentedResponseAccepted()
	a.MoreFollows = meta.moreFollows()
	a.DataType = meta.DataType()

	switch a.DataType {
	case btypes.ComplexAck:
		return d.apduComplexAck(a)
	case btypes.SimpleAck:
		return d.apduSimpleAck(a)
	case btypes.UnconfirmedServiceRequest:
		return d.apduUnconfirmed(a)
	case btypes.ConfirmedServiceRequest:
		return d.apduConfirmed(a)
	case btypes.SegmentAck:
		return fmt.Errorf("Segmented")
	case btypes.Error:
		return d.apduError(a)
	case btypes.Reject:
		return fmt.Errorf("Rejected")
	case btypes.Abort:
		return d.apduAbort(a)
	default:
		return fmt.Errorf("Unknown PDU type:%d", a.DataType)
	}
}

func (d *Decoder) apduAbort(a *btypes.APDU) error {
	d.decode(&a.InvokeId)
	var reason uint8
	d.decode(&reason)
	a.RawData = []byte{reason}
	return d.Error()
}

//func (d *Decoder) apduError(a *btypes.APDU) error {
//	d.decode(&a.InvokeId)
//	d.decode(&a.Service)
//
//	_, meta := d.tagNumber()
//	if meta.isOpening() {
//		_, _, value := d.tagNumberAndValue()
//		a.Error.Class = d.unsigned(int(value))
//		_, _, value = d.tagNumberAndValue()
//		a.Error.Code = d.unsigned(int(value))
//		_, meta = d.tagNumber()
//		if !meta.isClosing() {
//			return &ErrorWrongTagType{ClosingTag}
//		}
//	} else {
//		_, _, value := d.tagNumberAndValue()
//		a.Error.Class = d.unsigned(int(value))
//		_, _, value = d.tagNumberAndValue()
//		a.Error.Code = d.unsigned(int(value))
//	}
//	return nil
//}

func (d *Decoder) apduError(a *btypes.APDU) error {
	d.decode(&a.InvokeId)
	d.decode(&a.Service)

	classVal, err := d.AppData()
	if err != nil {
		return err
	}
	codeVal, err := d.AppData()
	if err != nil {
		return err
	}

	switch v := classVal.(type) {
	case uint32:
		a.Error.Class = bacerr.ErrorClass(v)
	case btypes.Enumerated:
		a.Error.Class = bacerr.ErrorClass(v)
	default:
		return fmt.Errorf("unexpected error class type %T", classVal)
	}
	switch v := codeVal.(type) {
	case uint32:
		a.Error.Code = bacerr.ErrorCode(v)
	case btypes.Enumerated:
		a.Error.Code = bacerr.ErrorCode(v)
	default:
		return fmt.Errorf("unexpected error code type %T", codeVal)
	}
	return nil
}

func (d *Decoder) apduComplexAck(a *btypes.APDU) error {
	d.decode(&a.InvokeId)
	d.decode(&a.Service)
	// Snapshot remaining service data without consuming it, so callers that
	// continue decoding from this Decoder (e.g. client ReadProperty) still work.
	if d.len() > 0 {
		a.RawData = append([]byte(nil), d.Bytes()...)
	}
	return d.Error()
}

func (d *Decoder) apduSimpleAck(a *btypes.APDU) error {
	d.decode(&a.InvokeId)
	d.decode(&a.Service)
	return d.Error()
}

func (d *Decoder) apduUnconfirmed(a *btypes.APDU) error {
	d.decode(&a.UnconfirmedService)
	a.RawData = make([]byte, d.len())
	d.decode(a.RawData)
	return d.Error()
}
func (d *Decoder) apduConfirmed(a *btypes.APDU) error {
	a.MaxSegs, a.MaxApdu = d.maxSegsMaxApdu()

	d.decode(&a.InvokeId)
	if a.SegmentedMessage {
		d.decode(&a.Sequence)
		d.decode(&a.WindowNumber)
	}

	d.decode(&a.Service)
	if d.len() > 0 {
		a.RawData = make([]byte, d.len())
		d.decode(&a.RawData)
	}

	return d.Error()
}

type APDUMetadata byte

const (
	apduMaskSegmented         = 1 << 3
	apduMaskMoreFollows       = 1 << 2
	apduMaskSegmentedAccepted = 1 << 1
	// Bit 0 is reserved
)

func (meta *APDUMetadata) setInfoMask(b bool, mask byte) {
	*meta = APDUMetadata(setInfoMask(byte(*meta), b, mask))
}

// CheckMask uses mask to check bit position
func (meta *APDUMetadata) checkMask(mask byte) bool {
	return (*meta & APDUMetadata(mask)) > 0
}

func (meta *APDUMetadata) isSegmentedMessage() bool {
	return meta.checkMask(apduMaskSegmented)
}

func (meta *APDUMetadata) moreFollows() bool {
	return meta.checkMask(apduMaskMoreFollows)
}

func (meta *APDUMetadata) segmentedResponseAccepted() bool {
	return meta.checkMask(apduMaskSegmentedAccepted)
}

func (meta *APDUMetadata) setSegmentedMessage(b bool) {
	meta.setInfoMask(b, apduMaskSegmented)
}

func (meta *APDUMetadata) setMoreFollows(b bool) {
	meta.setInfoMask(b, apduMaskMoreFollows)
}

func (meta *APDUMetadata) setSegmentedAccepted(b bool) {
	meta.setInfoMask(b, apduMaskSegmentedAccepted)
}

func (meta *APDUMetadata) setDataType(t btypes.PDUType) {
	// clean the first 4 bits
	*meta = (*meta & APDUMetadata(0xF0)) | APDUMetadata(t)
}
func (meta *APDUMetadata) DataType() btypes.PDUType {
	// clean the first 4 bits
	return btypes.PDUType(0xF0) & btypes.PDUType(*meta)
}
