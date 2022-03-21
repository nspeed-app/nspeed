package nspeed

import _ "embed"

//go:embed embed/localhost.pem
var TLSSelfSignedCert []byte

//go:embed embed/localhost-key.pem
var TLSSelfSignedKey []byte

//go:embed embed/localhost-CA.pem
var TLSSelfSignedCA []byte
