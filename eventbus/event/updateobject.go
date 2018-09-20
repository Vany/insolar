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

// UpdateObjectEvent is a event for calling constructor and obtain its reaction
type UpdateObjectEvent struct {
	baseEvent
	Object core.RecordRef
	Body   []byte
}

// React handles event and returns associated reaction.
func (e *UpdateObjectEvent) React(c core.Components) (core.Reaction, error) {
	return logicRunnerHandle(e, c)
}

// GetOperatingRole returns operating jet role for given event type.
func (e *UpdateObjectEvent) GetOperatingRole() core.JetRole {
	return core.RoleLightExecutor
}

// Get reference returns referenced object.
func (e *UpdateObjectEvent) GetReference() core.RecordRef {
	return e.Object
}

// Serialize serializes event.
func (e *UpdateObjectEvent) Serialize() (io.Reader, error) {
	return serialize(e, UpdateObjectEventType)
}
