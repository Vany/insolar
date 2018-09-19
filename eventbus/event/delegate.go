/*
 *    Copyright 2018 Insolar
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

package event

import (
	"io"

	"github.com/insolar/insolar/core"
)

// DelegateEvent is a event for saving contract's body as a delegate
type DelegateEvent struct {
	baseEvent
	Into  core.RecordRef
	Class core.RecordRef
	Body  []byte
}

// GetOperatingRole returns operating jet role for given event type.
func (e *DelegateEvent) GetOperatingRole() core.JetRole {
	return core.RoleLightExecutor
}

// GetReference returns referenced object.
func (e *DelegateEvent) GetReference() core.RecordRef {
	return e.Into
}

// Serialize serializes event.
func (e *DelegateEvent) Serialize() (io.Reader, error) {
	return serialize(e, DelegateEventType)
}