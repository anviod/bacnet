package encoding

import (
	"fmt"

	"github.com/anviod/bacnet/btypes"
)

// AbortReason identifies why an Abort PDU was sent (ASHRAE 135).
type AbortReason uint8

const (
	AbortReasonOther                     AbortReason = 0
	AbortReasonBufferOverflow            AbortReason = 1
	AbortReasonInvalidAPDUInThisState    AbortReason = 2
	AbortReasonPreemptedByHigherPriority AbortReason = 3
	AbortReasonSegmentationNotSupported  AbortReason = 4
)

// AbortReasonFromAPDU extracts abort reason stored in APDU.RawData by the decoder.
func AbortReasonFromAPDU(a *btypes.APDU) (AbortReason, error) {
	if len(a.RawData) < 1 {
		return 0, fmt.Errorf("no abort reason")
	}
	return AbortReason(a.RawData[0]), nil
}
