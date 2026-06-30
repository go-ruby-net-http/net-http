// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import (
	"strconv"
	"strings"
)

// BadResponse corresponds to Net::HTTPBadResponse: a protocol-level error while
// parsing the status line, a header line, or a chunk-size line.
type BadResponse struct{ Msg string }

func (e *BadResponse) Error() string { return e.Msg }

// Response is the pure-Go port of Net::HTTPResponse: the parsed status line, the
// response header model (it embeds *Header, so all Net::HTTPHeader getters apply),
// the decoded body, and the selected subclass identity. The subclass is not a Go
// type but the Class / Category names MRI would have instantiated (e.g. "HTTPOK"
// in category "HTTPSuccess"), exposed via Class, Category and the kind helpers.
type Response struct {
	*Header

	httpVersion string // e.g. "1.1", or "" if the status line had no version
	code        string // e.g. "200"
	message     string // e.g. "OK", or "" if none was present

	class    string // Net::HTTPResponse subclass name, e.g. "HTTPOK"
	category string // category class name, e.g. "HTTPSuccess"
	hasBody  bool   // whether the status permits a body (HAS_BODY)

	body     []byte
	bodyRead bool
}

// HTTPVersion returns the server's HTTP version string (Net::HTTPResponse#http_version).
func (r *Response) HTTPVersion() string { return r.httpVersion }

// Code returns the 3-digit status code string (Net::HTTPResponse#code).
func (r *Response) Code() string { return r.code }

// Message returns the reason phrase (Net::HTTPResponse#message / #msg).
func (r *Response) Message() string { return r.message }

// Class returns the Net::HTTPResponse subclass name MRI selects for this code
// (e.g. "HTTPOK", "HTTPNotFound", "HTTPUnknownResponse").
func (r *Response) Class() string { return r.class }

// Category returns the response category class name (e.g. "HTTPSuccess",
// "HTTPClientError") — the per-first-digit parent class.
func (r *Response) Category() string { return r.category }

// BodyPermitted reports whether the status permits a body
// (Net::HTTPResponse.body_permitted? / HAS_BODY).
func (r *Response) BodyPermitted() bool { return r.hasBody }

// kind tests reproduce the `res.kind_of?(Net::HTTPSuccess)` family used to
// classify a response; they hold for the exact subclass and its category.

// IsInformation reports a 1xx response (kind_of? Net::HTTPInformation).
func (r *Response) IsInformation() bool { return r.category == "HTTPInformation" }

// IsSuccess reports a 2xx response (kind_of? Net::HTTPSuccess).
func (r *Response) IsSuccess() bool { return r.category == "HTTPSuccess" }

// IsRedirection reports a 3xx response (kind_of? Net::HTTPRedirection).
func (r *Response) IsRedirection() bool { return r.category == "HTTPRedirection" }

// IsClientError reports a 4xx response (kind_of? Net::HTTPClientError).
func (r *Response) IsClientError() bool { return r.category == "HTTPClientError" }

// IsServerError reports a 5xx response (kind_of? Net::HTTPServerError).
func (r *Response) IsServerError() bool { return r.category == "HTTPServerError" }

// Body returns the decoded response body (Net::HTTPResponse#body), or nil for a
// status that permits no body. It is populated by ReadBody during ParseResponse.
func (r *Response) Body() []byte { return r.body }

// ReadBody returns the cached decoded body (Net::HTTPResponse#read_body); the
// body is decoded once during parsing.
func (r *Response) ReadBody() []byte { return r.body }

// EachHeader is provided by the embedded *Header (Net::HTTPResponse#each_header).

// ParseResponse parses a complete raw HTTP/1.1 response byte stream into a
// Response: the status line, the header block (with obs-fold continuation and
// multi-value fields), and the body decoded per Content-Length or chunked
// Transfer-Encoding. It is the inverse of Request.Bytes and the payload side of
// the host's read seam: hand it everything read from the socket for one response.
//
// It mirrors Net::HTTPResponse.read_new (status line + headers) followed by
// read_body (body framing). A response whose status permits no body (HAS_BODY
// false: 1xx, 204, 205, 304) yields a nil body and ignores any trailing bytes,
// exactly as MRI does.
func ParseResponse(raw []byte) (*Response, error) {
	p := &parser{data: raw}

	statusLine, ok := p.readLine()
	if !ok {
		return nil, &BadResponse{Msg: "wrong status line: " + strconv.Quote(string(raw))}
	}
	httpv, code, msg, err := parseStatusLine(statusLine)
	if err != nil {
		return nil, err
	}

	class, category, hasBody := classForCode(code)
	res := &Response{
		Header:      NewHeader(),
		httpVersion: httpv,
		code:        code,
		message:     msg,
		class:       class,
		category:    category,
		hasBody:     hasBody,
	}

	if err := parseHeaders(p, res.Header); err != nil {
		return nil, err
	}

	if !hasBody {
		res.bodyRead = true
		return res, nil
	}

	body, err := readBody(p, res.Header)
	if err != nil {
		return nil, err
	}
	res.body = body
	res.bodyRead = true
	return res, nil
}

// parseStatusLine ports read_status_line's regex:
//
//	/\AHTTP(?:\/(\d+\.\d+))?\s+(\d\d\d)(?:\s+(.*))?\z/in
func parseStatusLine(line string) (httpv, code, msg string, err error) {
	fail := func() (string, string, string, error) {
		return "", "", "", &BadResponse{Msg: "wrong status line: " + strconv.Quote(line)}
	}
	rest := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(strings.ToUpper(rest), "HTTP") {
		return fail()
	}
	rest = rest[len("HTTP"):]

	// optional /<digits>.<digits>
	if strings.HasPrefix(rest, "/") {
		ver := rest[1:]
		i := 0
		for i < len(ver) && isDigit(ver[i]) {
			i++
		}
		if i == 0 || i >= len(ver) || ver[i] != '.' {
			return fail()
		}
		dot := i
		i++
		start := i
		for i < len(ver) && isDigit(ver[i]) {
			i++
		}
		if i == start {
			return fail()
		}
		httpv = ver[:dot] + "." + ver[dot+1:i]
		rest = ver[i:]
	}

	// require \s+
	if rest == "" || !isSpace(rest[0]) {
		return fail()
	}
	rest = trimLeftSpace(rest)

	// exactly three digits
	if len(rest) < 3 || !isDigit(rest[0]) || !isDigit(rest[1]) || !isDigit(rest[2]) {
		return fail()
	}
	code = rest[:3]
	rest = rest[3:]

	// optional \s+(.*)
	if rest != "" {
		if !isSpace(rest[0]) {
			return fail()
		}
		msg = trimLeftSpace(rest)
	}
	return httpv, code, msg, nil
}

// parseHeaders ports each_response_header: read lines until a blank one,
// supporting obs-fold continuation lines (leading space/tab) and "key: value"
// splitting on /\s*:\s*/. Repeated keys accumulate as multiple values.
func parseHeaders(p *parser, h *Header) error {
	var key, value string
	have := false

	// flush appends the pending field. Parsed values are split on '\n' and then
	// whitespace-trimmed, so they can never contain CR/LF; appendField is the
	// CR/LF-free internal append (AddField's validation cannot trip here).
	flush := func() {
		if have {
			h.appendField(key, value)
		}
	}

	for {
		line, ok := p.readLine()
		if !ok {
			// EOF before the blank separator: MRI's readuntil would raise EOF;
			// treat a missing terminator as a malformed header block.
			return &BadResponse{Msg: "EOF while reading response header"}
		}
		line = trimRightWhitespace(line)
		if line == "" {
			break
		}
		if (line[0] == ' ' || line[0] == '\t') && have {
			if value != "" {
				value += " "
			}
			value += strings.TrimSpace(line)
			continue
		}
		flush()
		k, v, found := splitHeaderField(line)
		if !found {
			return &BadResponse{Msg: "wrong header line format"}
		}
		key, value, have = k, v, true
	}
	flush()
	return nil
}

// splitHeaderField splits "key: value" on the first /\s*:\s*/, returning ok=false
// when there is no colon (mirroring the value.nil? guard in MRI).
func splitHeaderField(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimRight(line[:idx], " \t")
	value = strings.TrimLeft(line[idx+1:], " \t")
	return key, value, true
}

// readBody ports read_body_0's framing decision: chunked Transfer-Encoding wins,
// else Content-Length, else read to EOF. (Content-encoding inflation is the
// host's job, like the socket itself.)
func readBody(p *parser, h *Header) ([]byte, error) {
	if h.Chunked() {
		return readChunked(p)
	}
	clen, ok, err := h.ContentLength()
	if err != nil {
		return nil, err
	}
	if ok {
		return p.readN(clen)
	}
	return p.readAll(), nil
}

// readChunked ports read_chunked: repeatedly read a chunk-size line (hex, with
// an optional ;extension), then that many bytes followed by CRLF, until a
// zero-size chunk; then consume the trailer up to the blank line.
func readChunked(p *parser) ([]byte, error) {
	var out []byte
	for {
		line, ok := p.readLine()
		if !ok {
			return nil, &BadResponse{Msg: "wrong chunk size line: EOF"}
		}
		hexlen := firstHexRun(line)
		if hexlen == "" {
			return nil, &BadResponse{Msg: "wrong chunk size line: " + line}
		}
		n, err := strconv.ParseInt(hexlen, 16, 64)
		if err != nil {
			return nil, &BadResponse{Msg: "wrong chunk size line: " + line}
		}
		if n == 0 {
			break
		}
		data, err := p.readN(int(n))
		if err != nil {
			return nil, err
		}
		out = append(out, data...)
		// consume the trailing CRLF after the chunk data (MRI: @socket.read 2).
		if _, err := p.readN(2); err != nil {
			return nil, err
		}
	}
	// MRI reads trailer lines until a blank line.
	for {
		line, ok := p.readLine()
		if !ok {
			break
		}
		if trimRightWhitespace(line) == "" {
			break
		}
	}
	return out, nil
}

// firstHexRun returns the first maximal run of hex digits in line ("" if none) —
// the String#slice(/[0-9a-fA-F]+/) MRI uses on a chunk-size line.
func firstHexRun(line string) string {
	start := -1
	for i := 0; i < len(line); i++ {
		if isHex(line[i]) {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			return line[start:i]
		}
	}
	if start >= 0 {
		return line[start:]
	}
	return ""
}

// parser is a forward byte cursor over the raw response, exposing the line- and
// length-oriented reads MRI's BufferedIO offers (readline, read N, read_all).
type parser struct {
	data []byte
	pos  int
}

// readLine returns the next line including any trailing CR/LF stripped by the
// caller; it returns ok=false at EOF with no remaining bytes. The returned line
// excludes the LF but retains a trailing CR, which callers trim as needed.
func (p *parser) readLine() (string, bool) {
	if p.pos >= len(p.data) {
		return "", false
	}
	idx := indexByte(p.data[p.pos:], '\n')
	if idx < 0 {
		line := string(p.data[p.pos:])
		p.pos = len(p.data)
		return line, true
	}
	line := string(p.data[p.pos : p.pos+idx])
	p.pos += idx + 1
	return line, true
}

// readN returns exactly n bytes, erroring if fewer remain (MRI raises EOFError;
// here a truncated body is a BadResponse).
func (p *parser) readN(n int) ([]byte, error) {
	if n < 0 {
		n = 0
	}
	if p.pos+n > len(p.data) {
		return nil, &BadResponse{Msg: "unexpected EOF while reading body"}
	}
	b := make([]byte, n)
	copy(b, p.data[p.pos:p.pos+n])
	p.pos += n
	return b, nil
}

// readAll returns all remaining bytes (read_all).
func (p *parser) readAll() []byte {
	b := make([]byte, len(p.data)-p.pos)
	copy(b, p.data[p.pos:])
	p.pos = len(p.data)
	return b
}

func indexByte(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }
func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func trimLeftSpace(s string) string {
	for len(s) > 0 && isSpace(s[0]) {
		s = s[1:]
	}
	return s
}

// trimRightWhitespace ports the .sub(/\s+\z/, ”) MRI applies to each header
// line: strip trailing whitespace (spaces, tabs, CR, LF).
func trimRightWhitespace(s string) string {
	end := len(s)
	for end > 0 && isSpace(s[end-1]) {
		end--
	}
	return s[:end]
}
