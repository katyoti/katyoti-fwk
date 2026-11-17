// SPDX-License-Identifier: Apache-2.0

// Package genspec is the one description of the generated CWM client, read by
// the generator (../../gen) and by the test that refuses a stale file. It sits
// apart from the client package so the generator still builds when the
// generated file no longer compiles against a changed interface.
package genspec

import (
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// FileName is the generated file, in the client package.
const FileName = "zz_client.go"

// Spec generates the CWMService client.
func Spec() busclient.Spec {
	return busclient.Spec{
		Package: "remote",
		Iface:   reflect.TypeOf((*cwmapi.CWMService)(nil)).Elem(),
		Imports: map[string]string{
			reflect.TypeOf(cwmapi.Object{}).PkgPath(): "cwmapi",
		},
	}
}
