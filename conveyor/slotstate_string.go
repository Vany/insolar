// Code generated by "stringer -type=SlotState"; DO NOT EDIT.

package conveyor

import "strconv"

const _SlotState_name = "InitializingWorkingSuspending"

var _SlotState_index = [...]uint8{0, 12, 19, 29}

func (i SlotState) String() string {
	if i >= SlotState(len(_SlotState_index)-1) {
		return "SlotState(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _SlotState_name[_SlotState_index[i]:_SlotState_index[i+1]]
}