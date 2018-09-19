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

package response

import (
	"io"

	"github.com/insolar/insolar/core"
)

// ObjectBodyResponse - the most common response
type ObjectBodyResponse struct {
	Body        []byte
	Code        core.RecordRef
	Class       core.RecordRef
	MachineType core.MachineType
}

// Serialize serializes response
func (r *ObjectBodyResponse) Serialize() (io.Reader, error) {
	return serialize(r, ObjectBodyResponseType)
}