package auth

import _ "embed"

//go:embed templates/success.html
var SuccessHTML []byte

//go:embed templates/error.html
var ErrorHTML []byte
