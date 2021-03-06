//
// Copyright 2019 Insolar Technologies GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package genesis

import (
	"fmt"
	"testing"

	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
)

func TestKeys_getKeysFromFile(t *testing.T) {
	priv, pub, err := getKeysFromFile("testdata/keypair.json")
	require.NoError(t, err, "read keys from json")
	assert.Equal(t, fmt.Sprintf("%T", priv), "*ecdsa.PrivateKey", "private key has proper type")
	assert.Equal(t, fmt.Sprintf("%T", pub), "string", "public key has proper type")
}

func TestKeys_publicKeyNormalize(t *testing.T) {
	var (
		begin   = "-----BEGIN PUBLIC KEY-----\n"
		end     = "-----END PUBLIC KEY-----\n"
		pubKey1 = begin + "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEf+vsMVU75xH8uj5WRcOqYdHXtaHH\nN0na2RVQ1xbhsVybYPae3ujNHeQCPj+RaJyMVhb6Aj/AOsTTOPFswwIDAQ==\n" + end
		pubKey2 = begin + "\n" + "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEf+vsMVU75xH8uj5WRcOqYdHXtaHH\nN0na2RVQ1xbhsVybYPae3ujNHeQCPj+RaJyMVhb6Aj/AOsTTOPFswwIDAQ==\n" + end
	)

	s1 := mustNormalizePublicKey(pubKey1)
	s2 := mustNormalizePublicKey(pubKey2)
	require.Equal(t, s1, s2, "the same result for the same public key")
}
