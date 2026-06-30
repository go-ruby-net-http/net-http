// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import "strings"

const upperHex = "0123456789ABCDEF"

// formUnreserved reports whether b is passed through literally by MRI's
// URI.encode_www_form_component: ALPHA / DIGIT / '*' / '-' / '.' / '_'. Note
// that '~' is *not* in this set (MRI escapes it), and space maps to '+'.
func formUnreserved(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9':
		return true
	case b == '*' || b == '-' || b == '.' || b == '_':
		return true
	}
	return false
}

// EncodeWWWFormComponent percent-encodes s as
// URI.encode_www_form_component does: unreserved bytes pass through, space
// becomes '+', and every other byte becomes %XX with uppercase hex.
func EncodeWWWFormComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case formUnreserved(c):
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&0x0f])
		}
	}
	return b.String()
}

// EncodeWWWForm joins key/value pairs into an application/x-www-form-urlencoded
// query string, mirroring URI.encode_www_form. Each key and value is encoded
// with EncodeWWWFormComponent and joined with '='; pairs are joined with '&'.
// A pair whose value is the empty string is emitted as bare "key" only when it
// was given as a value-less entry; here every pair has a value, so an empty
// value yields "key=" — matching MRI for the [][2]string form.
func EncodeWWWForm(pairs [][2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, kv := range pairs {
		parts = append(parts,
			EncodeWWWFormComponent(kv[0])+"="+EncodeWWWFormComponent(kv[1]))
	}
	return strings.Join(parts, "&")
}
