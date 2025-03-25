package crypto

import _ "embed"

//go:embed embed/localhost.crt
var TLSSelfSignedCert []byte

//go:embed embed/localhost.key
var TLSSelfSignedKey []byte

//go:embed embed/localhost-CA.crt
var TLSSelfSignedCA []byte
