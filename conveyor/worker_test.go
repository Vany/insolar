/*
 *    Copyright 2019 Insolar Technologies
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package conveyor

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/insolar/insolar/conveyor/adapter"
	"github.com/insolar/insolar/conveyor/interfaces/constant"
	slot2 "github.com/insolar/insolar/conveyor/interfaces/slot"
	"github.com/insolar/insolar/conveyor/interfaces/statemachine"
	"github.com/insolar/insolar/conveyor/queue"
	"github.com/insolar/insolar/core"
	"github.com/insolar/insolar/log"
	"github.com/stretchr/testify/require"
)

func addElements(queue queue.IQueue, num int) {
	for i := 0; i < num; i++ {
		queue.SinkPush("Test" + strconv.Itoa(i))
	}

}

func run(pulseState constant.PulseState, t *testing.T) {
	slot := NewSlot(pulseState, 22)
	worker := newWorkerStateMachineImpl(slot)

	sm := statemachine.NewStateMachineTypeMock(t)
	sm.GetMigrationHandlerFunc = func(p constant.PulseState, p1 uint32) (r statemachine.MigrationHandler) {
		return func(element slot2.SlotElementHelper) (interface{}, uint32, error) {
			return element.GetElementID(), 0, nil
		}
	}

	sm.GetTransitionHandlerFunc = func(p constant.PulseState, p1 uint32) (r statemachine.TransitHandler) {
		return func(element slot2.SlotElementHelper) (interface{}, uint32, error) {
			return element.GetElementID(), 0, nil
		}
	}

	el, _ := slot.createElement(sm, 22, queue.OutputElement{})
	go worker.run()

	for i := 0; i < 5; i++ {
		resp := adapter.AdapterResponse{}
		resp.SetElementID(el.GetElementID())

		slot.responseQueue.SinkPush(&resp)

		log.Info(">>>>>> ", i, 1)
		time.Sleep(time.Millisecond * 400)
		addElements(slot.inputQueue, 10)
		log.Info(">>>>>> ", i, 2)
		time.Sleep(time.Millisecond * 400)
		slot.inputQueue.PushSignal(PendingPulseSignal, mockCallback())
		addElements(slot.inputQueue, 10)
		log.Info(">>>>>> ", i, 3)
		time.Sleep(time.Millisecond * 300)
		addElements(slot.inputQueue, 10)
		slot.inputQueue.PushSignal(ActivatePulseSignal, mockCallback())
		log.Info(">>>>>> ", i, 4)
		addElements(slot.inputQueue, 10)

		time.Sleep(time.Millisecond * 400)
		log.Info(">>>>>> ", i, 5)

		slot.inputQueue.PushSignal(PendingPulseSignal, mockCallback())
		time.Sleep(time.Millisecond * 400)

		log.Info(">>>>>> ", i, 6)

		slot.inputQueue.PushSignal(ActivatePulseSignal, mockCallback())
		log.Info(">>>>>> ", i, 7)
	}

	time.Sleep(time.Millisecond * 400)
	slot.inputQueue.PushSignal(PendingPulseSignal, mockCallback())
	slot.inputQueue.PushSignal(ActivatePulseSignal, mockCallback())

	time.Sleep(time.Millisecond * 900)

}

func _TestSlot_Worker(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		run(constant.Present, t)
		wg.Done()
	}(&wg)

	// go func(wg *sync.WaitGroup) {
	// 	run(constant.Past, t)
	// 	wg.Done()
	// }(&wg)
	// go func(wg *sync.WaitGroup) {
	// 	run(constant.Future, t)
	// 	wg.Done()
	// }(&wg)

	wg.Wait()

	//run(constant.Present, t)

}

func makeSlotAndWorker(pulseState constant.PulseState, pulseNumber core.PulseNumber) (*Slot, workerStateMachineImpl) {
	slot := NewSlot(pulseState, pulseNumber)
	worker := newWorkerStateMachineImpl(slot)
	return slot, worker
}

func Test_changePulseState(t *testing.T) {
	slot, worker := makeSlotAndWorker(constant.Future, 22)

	worker.changePulseState()
	require.Equal(t, constant.Present, slot.pulseState)

	worker.changePulseState()
	require.Equal(t, constant.Past, slot.pulseState)

	worker.changePulseState()
	require.Equal(t, constant.Past, slot.pulseState)

	slot.pulseState = 99999
	require.PanicsWithValue(t, "[ changePulseState ] Unknown state: PulseState(99999)", worker.changePulseState)
}

func areSlotStatesEqual(s1 *Slot, s2 *Slot, t *testing.T) {
	require.Equal(t, s1.pulseState, s2.pulseState)
	require.Equal(t, s1.stateMachine, s2.stateMachine)
	require.Equal(t, s1.pulse, s2.pulse)
	require.Equal(t, s1.slotState, s2.slotState)
	// TODO: add check of lengthes
}

// ---- processSignalsWorking

func Test_processSignalsWorking_EmptyInput(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)

			oldSlot := *slot
			require.Equal(t, 0, worker.processSignalsWorking([]queue.OutputElement{}))
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsWorking_NonSignals(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			nonSignals := []queue.OutputElement{
				*queue.NewOutputElement(1, 0),
				*queue.NewOutputElement(2, 0),
				*queue.NewOutputElement(3, 0),
			}
			require.Equal(t, 0, worker.processSignalsWorking(nonSignals))

			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsWorking_BadSignal(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			badSignal := []queue.OutputElement{*queue.NewOutputElement(1, 9999999)}
			require.PanicsWithValue(t, "[ processSignalsWorking ] Unknown signal: 9999999", func() {
				worker.processSignalsWorking(badSignal)
			})
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsWorking_PendingPulseSignal(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			pendingSignal := []queue.OutputElement{*queue.NewOutputElement(1, PendingPulseSignal)}
			require.Equal(t, 1, worker.processSignalsWorking(pendingSignal))
			require.Equal(t, Suspending, slot.slotState)
		})
	}
}

func Test_processSignalsWorking_ActivatePulseSignal(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot
			activateSignal := []queue.OutputElement{*queue.NewOutputElement(1, ActivatePulseSignal)}
			require.Equal(t, 1, worker.processSignalsWorking(activateSignal))

			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsWorking_ActivateAndPendingPulseSignals(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			signals := []queue.OutputElement{
				*queue.NewOutputElement(1, PendingPulseSignal),
				*queue.NewOutputElement(1, ActivatePulseSignal),
			}

			require.Equal(t, 2, worker.processSignalsWorking(signals))
			require.Equal(t, Suspending, slot.slotState)
			inputElements := slot.inputQueue.RemoveAll()
			require.Len(t, inputElements, 1)
			require.Equal(t, ActivatePulseSignal, int(inputElements[0].GetItemType()))
		})
	}
}

// ---- readInputQueueWorking

func Test_readInputQueueWorking_EmptyInputQueue(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot
			require.NoError(t, worker.readInputQueueWorking())

			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_readInputQueueWorking_SignalOnly(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}
	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {

			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			require.NoError(t, slot.inputQueue.PushSignal(ActivatePulseSignal, mockCallback()))
			require.NoError(t, worker.readInputQueueWorking())
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_readInputQueueWorking_EventOnly(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot
			var payLoad interface{}
			payLoad = 99
			require.NoError(t, slot.inputQueue.SinkPush(payLoad))
			require.NoError(t, worker.readInputQueueWorking())

			areSlotStatesEqual(&oldSlot, slot, t)
			el := slot.popElement(ActiveElement)
			require.Equal(t, payLoad, el.payload)
		})
	}
}

func Test_readInputQueueWorking_SignalsAndEvents(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			slot.inputQueue.PushSignal(ActivatePulseSignal, mockCallback())

			numElements := 20
			for i := 0; i < numElements; i++ {
				require.NoError(t, slot.inputQueue.SinkPush(i))
			}

			require.NoError(t, worker.readInputQueueWorking())
			areSlotStatesEqual(&oldSlot, slot, t)

			for i := 0; i < numElements; i++ {
				el := slot.popElement(ActiveElement)
				require.Equal(t, i, el.payload)
			}
		})
	}
}

// ---- processSignalsSuspending

func Test_processSignalsSuspending_EmptyInput(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)

			oldSlot := *slot
			require.Equal(t, 0, worker.processSignalsSuspending([]queue.OutputElement{}))
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsSuspending_NonSignals(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			nonSignals := []queue.OutputElement{
				*queue.NewOutputElement(1, 0),
				*queue.NewOutputElement(2, 0),
				*queue.NewOutputElement(3, 0),
			}
			require.Equal(t, 0, worker.processSignalsSuspending(nonSignals))

			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsSuspending_BadSignal(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			badSignal := []queue.OutputElement{*queue.NewOutputElement(1, 9999999)}
			require.PanicsWithValue(t, "[ processSignalsSuspending ] Unknown signal: 9999999", func() {
				worker.processSignalsSuspending(badSignal)
			})
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsSuspending_PendingPulseSignal(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot
			pendingSignal := []queue.OutputElement{*queue.NewOutputElement(1, PendingPulseSignal)}
			require.Equal(t, 1, worker.processSignalsSuspending(pendingSignal))
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_processSignalsSuspending_ActivatePulseSignal(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			activateSignal := []queue.OutputElement{*queue.NewOutputElement(1, ActivatePulseSignal)}
			require.Equal(t, 1, worker.processSignalsSuspending(activateSignal))

			require.Equal(t, Initializing, slot.slotState)
		})
	}
}

// ---- readInputQueueWorking

func Test_readInputQueueSuspending_EmptyInputQueue(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot
			require.NoError(t, worker.readInputQueueSuspending())

			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_readInputQueueSuspending_SignalOnly(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present, constant.Past}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			require.NoError(t, slot.inputQueue.PushSignal(PendingPulseSignal, mockCallback()))
			require.NoError(t, worker.readInputQueueSuspending())
			areSlotStatesEqual(&oldSlot, slot, t)
		})
	}
}

func Test_readInputQueueSuspending_EventOnly(t *testing.T) {

	tests := []constant.PulseState{constant.Future, constant.Present}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot
			var payLoad interface{}
			payLoad = 99
			require.NoError(t, slot.inputQueue.SinkPush(payLoad))
			require.NoError(t, worker.readInputQueueSuspending())

			areSlotStatesEqual(&oldSlot, slot, t)
			el := slot.popElement(ActiveElement)
			require.Equal(t, payLoad, el.payload)
		})
	}
}

func Test_readInputQueueSuspending_EventOnly_Past(t *testing.T) {
	slot, worker := makeSlotAndWorker(constant.Past, 22)
	var payLoad interface{}
	payLoad = 99
	require.NoError(t, slot.inputQueue.SinkPush(payLoad))
	require.NoError(t, worker.readInputQueueSuspending())

	require.Equal(t, Working, slot.slotState)

	el := slot.popElement(ActiveElement)
	require.Equal(t, payLoad, el.payload)
}

func Test_readInputQueueSuspending_SignalsAndEvents(t *testing.T) {
	tests := []constant.PulseState{constant.Future, constant.Present}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			slot, worker := makeSlotAndWorker(tt, 22)
			oldSlot := *slot

			slot.inputQueue.PushSignal(PendingPulseSignal, mockCallback())

			numElements := 20
			for i := 0; i < numElements; i++ {
				require.NoError(t, slot.inputQueue.SinkPush(i))
			}

			require.NoError(t, worker.readInputQueueSuspending())
			areSlotStatesEqual(&oldSlot, slot, t)

			for i := 0; i < numElements; i++ {
				el := slot.popElement(ActiveElement)
				require.Equal(t, i, el.payload)
			}
		})
	}
}

func Test_readInputQueueSuspending_SignalsAndEvents_Past(t *testing.T) {
	slot, worker := makeSlotAndWorker(constant.Past, 22)
	slot.inputQueue.PushSignal(ActivatePulseSignal, mockCallback())

	numElements := 20
	for i := 0; i < numElements; i++ {
		require.NoError(t, slot.inputQueue.SinkPush(i))
	}

	require.NoError(t, worker.readInputQueueSuspending())

	for i := 0; i < numElements; i++ {
		el := slot.popElement(ActiveElement)
		require.Equal(t, i, el.payload)
	}

	require.Equal(t, Working, slot.slotState)

}
