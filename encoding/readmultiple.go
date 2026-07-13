package encoding

import (
	"github.com/anviod/bacnet/btypes"
)

func (e *Encoder) ReadMultipleProperty(invokeID uint8, data btypes.MultiplePropertyData) error {
	a := btypes.APDU{
		DataType:         btypes.ConfirmedServiceRequest,
		Service:          btypes.ServiceConfirmedReadPropMultiple,
		MaxSegs:          0,
		MaxApdu:          MaxAPDU,
		InvokeId:         invokeID,
		SegmentedMessage: false,
	}
	e.APDU(a)
	err := e.objects(data.Objects, false)
	if err != nil {
		return err
	}

	return e.Error()
}

// ReadMultipleProperty decodes a ReadPropertyMultiple request payload.
func (d *Decoder) ReadMultipleProperty(data *btypes.MultiplePropertyData) error {
	err := d.decodeObjects(&data.Objects, false)
	if err != nil {
		d.err = err
	}
	return d.Error()
}

// WriteMultipleProperty decodes a WritePropertyMultiple request payload.
func (d *Decoder) WriteMultipleProperty(data *btypes.MultiplePropertyData) error {
	err := d.decodeObjects(&data.Objects, true)
	if err != nil {
		d.err = err
	}
	return d.Error()
}

func (d *Decoder) decodeObjects(objects *[]btypes.Object, write bool) error {
	for d.Error() == nil && d.len() > 0 {
		obj := btypes.Object{Properties: []btypes.Property{}}

		tag, meta, _ := d.tagNumberAndValue()
		if tag != 0 {
			return &ErrorIncorrectTag{Expected: 0, Given: tag}
		}
		if !meta.isContextSpecific() {
			return &ErrorWrongTagType{ContextTag}
		}
		obj.ID.Type, obj.ID.Instance = d.objectId()

		tag, meta = d.tagNumber()
		if tag != 1 {
			return &ErrorIncorrectTag{Expected: 1, Given: tag}
		}
		if !meta.isOpening() {
			return &ErrorWrongTagType{OpeningTag}
		}

		for d.len() > 0 {
			tag, meta, length := d.tagNumberAndValue()
			if meta.isClosing() && tag == 1 {
				break
			}
			if tag != 0 || !meta.isContextSpecific() || meta.isOpening() || meta.isClosing() {
				return &ErrorIncorrectTag{Expected: 0, Given: tag}
			}

			prop := btypes.Property{
				Type:       btypes.PropertyType(d.enumerated(int(length))),
				ArrayIndex: ArrayAll,
			}

			if d.len() == 0 {
				obj.Properties = append(obj.Properties, prop)
				break
			}

			tag, meta = d.tagNumber()
			if meta.isClosing() && tag == 1 {
				obj.Properties = append(obj.Properties, prop)
				break
			}

			// Optional array index (context tag 1)
			if tag == 1 && meta.isContextSpecific() && !meta.isOpening() && !meta.isClosing() {
				lenValue := d.value(meta)
				prop.ArrayIndex = d.unsigned(int(lenValue))
				if d.len() == 0 {
					obj.Properties = append(obj.Properties, prop)
					break
				}
				tag, meta = d.tagNumber()
				if meta.isClosing() && tag == 1 {
					obj.Properties = append(obj.Properties, prop)
					break
				}
			}

			if write && tag == 2 && meta.isOpening() {
				var values []interface{}
				for d.len() > 0 {
					peekTag, peekMeta := d.tagNumber()
					if peekMeta.isClosing() && peekTag == 2 {
						break
					}
					_ = d.UnreadByte()
					val, err := d.AppData()
					if err != nil {
						return err
					}
					values = append(values, val)
				}
				if len(values) == 1 {
					prop.Data = values[0]
				} else {
					prop.Data = values
				}

				// Optional priority (tag 3 per standard; some encoders reuse tag 2)
				if d.len() > 0 {
					prioTag, prioMeta := d.tagNumber()
					if (prioTag == 3 || prioTag == 2) && prioMeta.isContextSpecific() &&
						!prioMeta.isOpening() && !prioMeta.isClosing() {
						lenValue := d.value(prioMeta)
						prop.Priority = btypes.NPDUPriority(d.unsigned(int(lenValue)))
					} else {
						_ = d.UnreadByte()
					}
				}
			} else if tag == 0 {
				// Beginning of next property identifier.
				_ = d.UnreadByte()
			} else if !(meta.isClosing() && tag == 1) {
				_ = d.UnreadByte()
			}

			obj.Properties = append(obj.Properties, prop)
		}

		*objects = append(*objects, obj)
	}
	return d.Error()
}
