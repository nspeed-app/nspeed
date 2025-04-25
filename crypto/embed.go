// Copyright (c) Jean-Francois Giorgi & AUTHORS
// part of nspeed.app
// SPDX-License-Identifier: BSD-3-Clause

package crypto

import _ "embed"

//go:embed embed/localhost.crt
var TLSSelfSignedCert []byte

//go:embed embed/localhost.key
var TLSSelfSignedKey []byte

//go:embed embed/localhost-CA.crt
var TLSSelfSignedCA []byte
