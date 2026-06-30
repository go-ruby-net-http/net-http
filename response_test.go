// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import (
	"reflect"
	"testing"
)

func TestParseSimpleResponse(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 5\r\n" +
		"\r\nhello"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.HTTPVersion() != "1.1" || res.Code() != "200" || res.Message() != "OK" {
		t.Errorf("status = %q/%q/%q", res.HTTPVersion(), res.Code(), res.Message())
	}
	if res.Class() != "HTTPOK" || res.Category() != "HTTPSuccess" {
		t.Errorf("class/category = %q/%q", res.Class(), res.Category())
	}
	if !res.IsSuccess() || res.IsClientError() {
		t.Error("kind predicates wrong")
	}
	if !res.BodyPermitted() {
		t.Error("200 should permit body")
	}
	if string(res.Body()) != "hello" || string(res.ReadBody()) != "hello" {
		t.Errorf("body = %q", res.Body())
	}
	if ct := res.ContentType(); ct != "text/plain" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestParseChunked(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Body()) != "Wikipedia" {
		t.Errorf("chunked body = %q", res.Body())
	}
}

func TestParseChunkedWithExtensionAndTrailer(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"4;ext=1\r\nWiki\r\n0\r\n" +
		"X-Trailer: v\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Body()) != "Wiki" {
		t.Errorf("chunked+ext body = %q", res.Body())
	}
}

func TestParseChunkedTrailerEOF(t *testing.T) {
	// Zero chunk with no trailing blank line (stream ends) — trailer loop hits EOF.
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Body()) != 0 {
		t.Errorf("expected empty body, got %q", res.Body())
	}
}

func TestParseChunkedBadSizeLine(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nzzz\r\n"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected bad chunk-size error")
	}
}

func TestParseChunkedSizeEOF(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected EOF chunk-size error")
	}
}

func TestParseChunkedTruncatedData(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n9\r\nWiki"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected truncated chunk error")
	}
}

func TestParseChunkedMissingCRLFAfterData(t *testing.T) {
	// chunk size 4, data "Wiki", but stream ends before the trailing CRLF.
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWiki"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected missing-CRLF error")
	}
}

func TestParseChunkedOverflowHex(t *testing.T) {
	// A hex run too large for int64 must surface as a bad chunk-size line.
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nffffffffffffffffff\r\n"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected overflow hex error")
	}
}

func TestParseContentLengthBody(t *testing.T) {
	raw := "HTTP/1.1 404 Not Found\r\nContent-Length: 3\r\n\r\nxyz"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.Class() != "HTTPNotFound" || !res.IsClientError() {
		t.Errorf("404 class = %q", res.Class())
	}
	if string(res.Body()) != "xyz" {
		t.Errorf("cl body = %q", res.Body())
	}
}

func TestParseContentLengthTruncated(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nshort"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected truncated body error")
	}
}

func TestParseBadContentLength(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Length: none\r\n\r\nbody"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected bad Content-Length error")
	}
}

func TestParseNoLengthReadsToEOF(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nall the rest"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Body()) != "all the rest" {
		t.Errorf("eof body = %q", res.Body())
	}
}

func TestParseNoBodyStatuses(t *testing.T) {
	for _, tc := range []struct {
		code, class, cat string
	}{
		{"204", "HTTPNoContent", "HTTPSuccess"},
		{"304", "HTTPNotModified", "HTTPRedirection"},
		{"100", "HTTPContinue", "HTTPInformation"},
	} {
		// trailing bytes after the headers must be ignored for a no-body status.
		raw := "HTTP/1.1 " + tc.code + " X\r\nContent-Length: 5\r\n\r\nIGNORED-EXTRA"
		res, err := ParseResponse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		if res.Class() != tc.class || res.Category() != tc.cat {
			t.Errorf("%s class/cat = %q/%q", tc.code, res.Class(), res.Category())
		}
		if res.BodyPermitted() {
			t.Errorf("%s should not permit body", tc.code)
		}
		if res.Body() != nil {
			t.Errorf("%s body = %q, want nil", tc.code, res.Body())
		}
	}
}

func TestParseHeaderFolding(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"X-Long: part1\r\n" +
		"  part2\r\n" +
		"\tpart3\r\n" +
		"Content-Length: 0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := res.Get("X-Long"); v != "part1 part2 part3" {
		t.Errorf("folded value = %q", v)
	}
}

func TestParseMultiValueHeader(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Set-Cookie: a=1\r\nSet-Cookie: b=2\r\n" +
		"Content-Length: 0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.GetFields("Set-Cookie"); !reflect.DeepEqual(got, []string{"a=1", "b=2"}) {
		t.Errorf("multi-value = %#v", got)
	}
}

func TestParseHeaderUsesContentLengthGetters(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nTransfer-Encoding: identity\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.Chunked() {
		t.Error("identity must not be chunked")
	}
}

func TestParseEmptyInput(t *testing.T) {
	if _, err := ParseResponse(nil); err == nil {
		t.Error("expected status-line error on empty input")
	}
	if _, ok := errBadResponse(t, nil); !ok {
		t.Error("nil input did not yield *BadResponse")
	}
}

func TestParseBadStatusLines(t *testing.T) {
	bad := []string{
		"NOTHTTP 200 OK\r\n\r\n",
		"HTTP/1 200 OK\r\n\r\n",   // missing minor
		"HTTP/x.y 200 OK\r\n\r\n", // non-digit version
		"HTTP/1. 200 OK\r\n\r\n",  // missing minor digits
		"HTTP/1.1200 OK\r\n\r\n",  // no space after version
		"HTTP/1.1 20 OK\r\n\r\n",  // two-digit code
		"HTTP/1.1 200OK\r\n\r\n",  // no space before message char run... actually 200 then 'OK' no space
		"HTTP/1.1\r\n\r\n",        // no code at all
		"HTTP/1.1 abc\r\n\r\n",    // non-digit code
		"HTTP/.1 200 OK\r\n\r\n",  // missing major digits
	}
	for _, raw := range bad {
		if _, err := ParseResponse([]byte(raw)); err == nil {
			t.Errorf("expected error for status line %q", raw)
		}
	}
}

func TestParseStatusLineNoVersion(t *testing.T) {
	// HTTP without a /version is valid (the (?:\/...)? group is optional).
	raw := "HTTP 200 OK\r\nContent-Length: 0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.HTTPVersion() != "" || res.Code() != "200" {
		t.Errorf("no-version status = %q/%q", res.HTTPVersion(), res.Code())
	}
}

func TestParseStatusLineNoMessage(t *testing.T) {
	raw := "HTTP/1.1 200\r\nContent-Length: 0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.Message() != "" {
		t.Errorf("no-message status message = %q", res.Message())
	}
}

func TestParseHeaderNoColon(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nBadHeaderNoColon\r\n\r\n"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected wrong-header-line error")
	}
}

func TestParseHeaderEOFBeforeBlank(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected EOF-in-headers error")
	}
}

func TestParseLeadingSpaceHeaderWithoutKey(t *testing.T) {
	// A continuation line before any key falls through and fails to split (MRI).
	raw := "HTTP/1.1 200 OK\r\n folded-first\r\nContent-Length: 0\r\n\r\n"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected wrong-header-line error for orphan continuation")
	}
}

func TestParseUnknownCodeAndCategory(t *testing.T) {
	// Exact code unknown but first digit maps to a category.
	raw := "HTTP/1.1 299 Weird\r\nContent-Length: 1\r\n\r\nq"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.Class() != "HTTPSuccess" || res.Category() != "HTTPSuccess" {
		t.Errorf("299 fallback class = %q/%q", res.Class(), res.Category())
	}
	if !res.IsSuccess() {
		t.Error("299 should be success category")
	}
}

func TestParseHeaderValueLowercaseHTTP(t *testing.T) {
	// The status-line "HTTP" token match is case-insensitive (/.../in).
	raw := "http/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.Code() != "200" {
		t.Errorf("lowercase http code = %q", res.Code())
	}
}

func TestEachHeaderIteration(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nA: 1\r\nB: 2\r\nContent-Length: 0\r\n\r\n"
	res, _ := ParseResponse([]byte(raw))
	var keys []string
	res.EachHeader(func(k, _ string) { keys = append(keys, k) })
	if !reflect.DeepEqual(keys, []string{"a", "b", "content-length"}) {
		t.Errorf("each_header keys = %#v", keys)
	}
}

func TestParserReadNNegative(t *testing.T) {
	// readN clamps a negative request to zero (defensive); reachable via a
	// direct parser to keep the clamp covered.
	p := &parser{data: []byte("abc")}
	b, err := p.readN(-5)
	if err != nil || len(b) != 0 {
		t.Errorf("readN(-5) = %q,%v", b, err)
	}
}

func TestStatusLineNoLF(t *testing.T) {
	// A status line not terminated by LF is still the whole buffer (readLine
	// returns it). With no header terminator it then errors in the header block.
	raw := "HTTP/1.1 200 OK"
	if _, err := ParseResponse([]byte(raw)); err == nil {
		t.Error("expected error: no header block")
	}
}

// errBadResponse asserts ParseResponse(raw) returned a *BadResponse, returning
// the message and whether the type matched.
func errBadResponse(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	_, err := ParseResponse(raw)
	br, ok := err.(*BadResponse)
	if !ok {
		return "", false
	}
	return br.Error(), true
}
