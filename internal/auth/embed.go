package auth

import _ "embed"

// SuccessHTML is the browser page shown after a successful auth callback.
//
//go:embed templates/success.html
var SuccessHTML []byte

// ErrorHTML is the browser page shown after a failed auth callback.
//
//go:embed templates/error.html
var ErrorHTML []byte
