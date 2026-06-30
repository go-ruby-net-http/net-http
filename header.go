// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package nethttp is a pure-Go (no cgo) reimplementation of the deterministic,
// interpreter-independent core of Ruby's Net::HTTP: the HTTP/1.1 *message*
// codec. It builds request bytes exactly as MRI 4.0.5 writes them to the socket
// and parses a raw response byte stream into MRI's Net::HTTPResponse subclass
// model — without any Ruby runtime, and without performing any I/O itself.
//
// The TCP socket and TLS are a host-side seam: the host supplies the byte
// transport, this library supplies build-request-bytes + parse-response-bytes +
// the header / response object model.
package nethttp

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Field-length limits mirror Net::HTTPHeader::MAX_KEY_LENGTH and
// MAX_FIELD_LENGTH (header.rb).
const (
	maxKeyLength   = 1024
	maxFieldLength = 65536
)

// HeaderSyntaxError corresponds to Net::HTTPHeaderSyntaxError: a malformed
// Content-Length / Content-Range field value.
type HeaderSyntaxError struct{ Msg string }

func (e *HeaderSyntaxError) Error() string { return e.Msg }

// Header is the field store shared by requests and responses — the pure-Go port
// of the Net::HTTPHeader mixin. Like MRI it stores keys downcased, each mapping
// to an ordered list of raw string values, and preserves first-insertion order.
type Header struct {
	fields map[string][]string
	order  []string // downcased keys in first-insertion order
}

// NewHeader returns an empty Header.
func NewHeader() *Header {
	return &Header{fields: map[string][]string{}}
}

func (h *Header) ensure() {
	if h.fields == nil {
		h.fields = map[string][]string{}
	}
}

// validateValue mirrors set_field/append_field_value: a field value may not
// contain CR or LF.
func validateValue(val string) error {
	if strings.ContainsAny(val, "\r\n") {
		return errors.New("header field value cannot include CR/LF")
	}
	return nil
}

// rawSet stores key=>[vals] under the downcased key, recording insertion order.
func (h *Header) rawSet(key string, vals []string) {
	h.ensure()
	dk := strings.ToLower(key)
	if _, ok := h.fields[dk]; !ok {
		h.order = append(h.order, dk)
	}
	h.fields[dk] = vals
}

// Set replaces the field for key (Net::HTTPHeader#[]=). A nil-equivalent is
// expressed by Delete; Set always stores a value.
func (h *Header) Set(key, val string) error {
	if err := validateValue(val); err != nil {
		return err
	}
	h.rawSet(key, []string{val})
	return nil
}

// Get returns the field for key joined by ", " (Net::HTTPHeader#[]), and ""
// with ok=false when the field is absent.
func (h *Header) Get(key string) (string, bool) {
	h.ensure()
	a, ok := h.fields[strings.ToLower(key)]
	if !ok {
		return "", false
	}
	return strings.Join(a, ", "), true
}

// GetFields returns a copy of the raw value list for key, or nil if absent
// (Net::HTTPHeader#get_fields).
func (h *Header) GetFields(key string) []string {
	h.ensure()
	a, ok := h.fields[strings.ToLower(key)]
	if !ok {
		return nil
	}
	out := make([]string, len(a))
	copy(out, a)
	return out
}

// Key reports whether key is present (Net::HTTPHeader#key?).
func (h *Header) Key(key string) bool {
	h.ensure()
	_, ok := h.fields[strings.ToLower(key)]
	return ok
}

// Delete removes key and returns its prior value list (Net::HTTPHeader#delete).
func (h *Header) Delete(key string) []string {
	h.ensure()
	dk := strings.ToLower(key)
	prev := h.fields[dk]
	if _, ok := h.fields[dk]; ok {
		delete(h.fields, dk)
		for i, k := range h.order {
			if k == dk {
				h.order = append(h.order[:i], h.order[i+1:]...)
				break
			}
		}
	}
	return prev
}

// AddField appends val to key, preserving any existing values
// (Net::HTTPHeader#add_field).
func (h *Header) AddField(key, val string) error {
	if err := validateValue(val); err != nil {
		return err
	}
	h.ensure()
	dk := strings.ToLower(key)
	if cur, ok := h.fields[dk]; ok {
		h.fields[dk] = append(cur, val)
		return nil
	}
	h.rawSet(key, []string{val})
	return nil
}

// appendField appends val to key without CR/LF validation. It is the internal
// path used by the response parser, whose values are already CR/LF-free (each
// field is split on '\n' then whitespace-trimmed).
func (h *Header) appendField(key, val string) {
	h.ensure()
	dk := strings.ToLower(key)
	if cur, ok := h.fields[dk]; ok {
		h.fields[dk] = append(cur, val)
		return
	}
	h.rawSet(key, []string{val})
}

// EachHeader calls fn for each field in insertion order with the downcased key
// and the values joined by ", " (Net::HTTPHeader#each_header / #each).
func (h *Header) EachHeader(fn func(key, value string)) {
	h.ensure()
	for _, dk := range h.order {
		fn(dk, strings.Join(h.fields[dk], ", "))
	}
}

// capitalize mirrors Net::HTTPHeader#capitalize: split on '-' and Capitalize
// each token ("content-md5" => "Content-Md5").
func capitalize(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		head := strings.ToUpper(string(r[0]))
		tail := strings.ToLower(string(r[1:]))
		parts[i] = head + tail
	}
	return strings.Join(parts, "-")
}

// EachCapitalized calls fn for each field in insertion order with the
// canonicalised key and the joined value (Net::HTTPHeader#each_capitalized) —
// this is the order and casing written to the wire by write_header.
func (h *Header) EachCapitalized(fn func(key, value string)) {
	h.ensure()
	for _, dk := range h.order {
		fn(capitalize(dk), strings.Join(h.fields[dk], ", "))
	}
}

// initFrom seeds the header from ordered initial fields, mirroring
// Net::HTTPHeader#initialize_http_header: each value is stripped, length- and
// CR/LF-validated, and stored in the order given (MRI preserves the initheader
// hash's insertion order, which becomes the on-the-wire field order).
func (h *Header) initFrom(init [][2]string) error {
	h.ensure()
	for _, kv := range init {
		k := kv[0]
		v := strings.TrimSpace(kv[1])
		if len(k) > maxKeyLength {
			return fmt.Errorf("too long (%d bytes) header", len(k))
		}
		if len(v) > maxFieldLength {
			return fmt.Errorf("header %s has too long field value: %d", k, len(v))
		}
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("header %s has field value %q, this cannot include CR/LF", k, v)
		}
		h.rawSet(k, []string{v})
	}
	return nil
}

// ContentLength returns the parsed Content-Length, ok=false when the field is
// absent (Net::HTTPHeader#content_length). A present-but-malformed value yields
// a *HeaderSyntaxError.
func (h *Header) ContentLength() (int, bool, error) {
	v, ok := h.Get("Content-Length")
	if !ok {
		return 0, false, nil
	}
	digits := firstDigits(v)
	if digits == "" {
		return 0, false, &HeaderSyntaxError{Msg: "wrong Content-Length format"}
	}
	n, _ := strconv.Atoi(digits)
	return n, true, nil
}

// SetContentLength sets Content-Length to len (Net::HTTPHeader#content_length=).
func (h *Header) SetContentLength(n int) {
	h.rawSet("content-length", []string{strconv.Itoa(n)})
}

// firstDigits returns the first maximal run of ASCII digits in s ("" if none) —
// the String#slice(/\d+/) MRI uses on Content-Length.
func firstDigits(s string) string {
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			return s[start:i]
		}
	}
	if start >= 0 {
		return s[start:]
	}
	return ""
}

// SetContentType sets Content-Type to type plus "; k=v" params in the given
// order (Net::HTTPHeader#set_content_type / #content_type=).
func (h *Header) SetContentType(typ string, params ...[2]string) {
	var b strings.Builder
	b.WriteString(typ)
	for _, kv := range params {
		b.WriteString("; ")
		b.WriteString(kv[0])
		b.WriteString("=")
		b.WriteString(kv[1])
	}
	h.rawSet("content-type", []string{b.String()})
}

// MainType returns the part of Content-Type before '/' (Net::HTTPHeader#main_type),
// or "" if there is no Content-Type.
func (h *Header) MainType() string {
	v, ok := h.Get("Content-Type")
	if !ok {
		return ""
	}
	first := strings.SplitN(v, ";", 2)[0]
	return strings.TrimSpace(strings.SplitN(first, "/", 2)[0])
}

// SubType returns the part of Content-Type after '/' (Net::HTTPHeader#sub_type),
// or "" if there is none.
func (h *Header) SubType() string {
	v, ok := h.Get("Content-Type")
	if !ok {
		return ""
	}
	first := strings.SplitN(v, ";", 2)[0]
	parts := strings.SplitN(first, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// ContentType returns "main/sub" (or just "main", or "") mirroring
// Net::HTTPHeader#content_type.
func (h *Header) ContentType() string {
	main := h.MainType()
	if main == "" {
		return ""
	}
	if sub := h.SubType(); sub != "" {
		return main + "/" + sub
	}
	return main
}

// Chunked reports whether Transfer-Encoding requests chunked encoding
// (Net::HTTPHeader#chunked?). MRI uses /(?:\A|[^\-\w])chunked(?![\-\w])/i.
func (h *Header) Chunked() bool {
	field, ok := h.Get("Transfer-Encoding")
	if !ok {
		return false
	}
	return matchChunked(field)
}

// matchChunked is the Go port of MRI's chunked? regex applied to field.
func matchChunked(field string) bool {
	lower := strings.ToLower(field)
	for i := 0; ; {
		idx := strings.Index(lower[i:], "chunked")
		if idx < 0 {
			return false
		}
		pos := i + idx
		// preceding boundary: start-of-string or a non [-\w] byte
		okLeft := pos == 0 || !isWordOrDash(lower[pos-1])
		end := pos + len("chunked")
		// following: not followed by [-\w]
		okRight := end >= len(lower) || !isWordOrDash(lower[end])
		if okLeft && okRight {
			return true
		}
		i = pos + 1
	}
}

func isWordOrDash(b byte) bool {
	return b == '-' || b == '_' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

// ConnectionClose reports whether Connection (or Proxy-Connection) requests the
// connection be closed (Net::HTTPHeader#connection_close?).
func (h *Header) ConnectionClose() bool {
	return h.connectionTokenAny("close")
}

// ConnectionKeepAlive reports whether Connection (or Proxy-Connection) requests
// keep-alive (Net::HTTPHeader#connection_keep_alive?).
func (h *Header) ConnectionKeepAlive() bool {
	return h.connectionTokenAny("keep-alive")
}

func (h *Header) connectionTokenAny(token string) bool {
	for _, key := range []string{"connection", "proxy-connection"} {
		for _, raw := range h.GetFields(key) {
			if hasConnectionToken(raw, token) {
				return true
			}
		}
	}
	return false
}

// hasConnectionToken ports the MRI token match /(?:\A|,)\s*<token>\s*(?:\z|,)/i:
// the field is a comma list and one element (trimmed) equals token.
func hasConnectionToken(field, token string) bool {
	for _, part := range strings.Split(field, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
