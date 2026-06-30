// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import (
	"strings"
	"testing"
)

func TestNewRequestUnknownMethod(t *testing.T) {
	if _, err := NewRequest("BREW", "/", "h", nil); err == nil {
		t.Error("expected unknown-method error")
	}
}

func TestNewRequestEmptyPath(t *testing.T) {
	if _, err := NewRequest("GET", "", "h", nil); err == nil {
		t.Error("expected empty-path error")
	}
}

func TestNewRequestInitHeaderError(t *testing.T) {
	if _, err := NewRequest("GET", "/", "h", [][2]string{{"X", "a\r\nb"}}); err == nil {
		t.Error("expected initHeader CR/LF error")
	}
}

func TestGetRequestBytes(t *testing.T) {
	r, err := NewRequest("GET", "/p?q=1", "example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method() != "GET" || r.Path() != "/p?q=1" {
		t.Errorf("method/path = %q/%q", r.Method(), r.Path())
	}
	if !r.ResponseBodyPermitted() || r.RequestBodyPermitted() {
		t.Error("GET body permissions wrong")
	}
	b, err := r.Bytes("1.1")
	if err != nil {
		t.Fatal(err)
	}
	want := "GET /p?q=1 HTTP/1.1\r\n" +
		"Accept-Encoding: gzip;q=1.0,deflate;q=0.6,identity;q=0.3\r\n" +
		"Accept: */*\r\nUser-Agent: Ruby\r\nHost: example.com\r\n\r\n"
	if string(b) != want {
		t.Errorf("GET bytes:\n got %q\nwant %q", b, want)
	}
}

func TestGetRequestNoHost(t *testing.T) {
	r, _ := NewRequest("GET", "/", "", nil)
	if r.Key("Host") {
		t.Error("Host should be absent without authority")
	}
}

func TestInitHeaderOrderPreserved(t *testing.T) {
	r, _ := NewRequest("GET", "/", "", [][2]string{{"Zeta", "1"}, {"Alpha", "2"}, {"Host", "h"}})
	b, _ := r.Bytes("1.1")
	want := "GET / HTTP/1.1\r\nZeta: 1\r\nAlpha: 2\r\nHost: h\r\n" +
		"Accept-Encoding: gzip;q=1.0,deflate;q=0.6,identity;q=0.3\r\n" +
		"Accept: */*\r\nUser-Agent: Ruby\r\n\r\n"
	if string(b) != want {
		t.Errorf("order bytes:\n got %q\nwant %q", b, want)
	}
}

func TestUserAcceptEncodingSuppressesSeed(t *testing.T) {
	r, _ := NewRequest("GET", "/", "", [][2]string{{"Accept-Encoding", "identity"}, {"X-Foo", "bar"}})
	b, _ := r.Bytes("1.1")
	if strings.Contains(string(b), "gzip;q=1.0") {
		t.Error("seed not suppressed when user set Accept-Encoding")
	}
	if !strings.Contains(string(b), "Accept-Encoding: identity") {
		t.Error("user Accept-Encoding missing")
	}
}

func TestRangeSuppressesSeed(t *testing.T) {
	r, _ := NewRequest("GET", "/", "", [][2]string{{"Range", "bytes=0-1"}})
	b, _ := r.Bytes("1.1")
	if strings.Contains(string(b), "gzip;q=1.0") {
		t.Error("seed not suppressed when user set Range")
	}
}

func TestPostFormData(t *testing.T) {
	r, _ := NewRequest("POST", "/submit", "example.com", nil)
	r.SetFormData([][2]string{{"name", "a b"}, {"x", "1&2"}})
	if !r.RequestBodyPermitted() {
		t.Error("POST should permit body")
	}
	if string(r.Body()) != "name=a+b&x=1%262" {
		t.Errorf("form body = %q", r.Body())
	}
	b, _ := r.Bytes("1.1")
	want := "POST /submit HTTP/1.1\r\n" +
		"Accept-Encoding: gzip;q=1.0,deflate;q=0.6,identity;q=0.3\r\n" +
		"Accept: */*\r\nUser-Agent: Ruby\r\nHost: example.com\r\n" +
		"Content-Type: application/x-www-form-urlencoded\r\n" +
		"Content-Length: 16\r\n\r\nname=a+b&x=1%262"
	if string(b) != want {
		t.Errorf("POST form bytes:\n got %q\nwant %q", b, want)
	}
}

func TestSetFormDataCustomSep(t *testing.T) {
	r, _ := NewRequest("POST", "/", "", nil)
	r.SetFormData([][2]string{{"a", "1"}, {"b", "2"}}, ";")
	if string(r.Body()) != "a=1;b=2" {
		t.Errorf("custom sep body = %q", r.Body())
	}
}

func TestExplicitBody(t *testing.T) {
	r, _ := NewRequest("POST", "/api", "h", nil)
	r.SetBody([]byte("hello world"))
	_ = r.Set("Content-Type", "text/plain")
	b, _ := r.Bytes("1.1")
	if !strings.HasSuffix(string(b), "\r\n\r\nhello world") {
		t.Errorf("explicit body tail wrong: %q", b)
	}
	if !strings.Contains(string(b), "Content-Length: 11\r\n") {
		t.Errorf("Content-Length wrong: %q", b)
	}
}

func TestBodyPermittedDefaultsEmpty(t *testing.T) {
	// PUT permits a body; with none set, MRI defaults it to "" and emits CL: 0.
	r, _ := NewRequest("PUT", "/r", "h", nil)
	b, _ := r.Bytes("1.1")
	if !strings.Contains(string(b), "Content-Length: 0\r\n") {
		t.Errorf("PUT default body Content-Length missing: %q", b)
	}
	if r.Body() == nil || len(r.Body()) != 0 {
		t.Errorf("default body = %#v", r.Body())
	}
}

func TestGetNoBodyNoContentLength(t *testing.T) {
	r, _ := NewRequest("GET", "/", "h", nil)
	b, _ := r.Bytes("1.1")
	if strings.Contains(string(b), "Content-Length") {
		t.Errorf("GET should have no Content-Length: %q", b)
	}
}

func TestHeadAndOptionsAndDelete(t *testing.T) {
	for _, m := range []string{"HEAD", "OPTIONS", "DELETE"} {
		r, _ := NewRequest(m, "/z", "h", nil)
		if r.RequestBodyPermitted() {
			t.Errorf("%s should not permit request body", m)
		}
		b, _ := r.Bytes("1.1")
		if strings.Contains(string(b), "Content-Length") {
			t.Errorf("%s emitted Content-Length: %q", m, b)
		}
		if !strings.HasPrefix(string(b), m+" /z HTTP/1.1\r\n") {
			t.Errorf("%s request line wrong: %q", m, b)
		}
	}
	if r, _ := NewRequest("HEAD", "/", "h", nil); r.ResponseBodyPermitted() {
		t.Error("HEAD response should not permit body")
	}
}

func TestPatchBody(t *testing.T) {
	r, _ := NewRequest("PATCH", "/r", "h", nil)
	r.SetBody([]byte("x"))
	b, _ := r.Bytes("1.1")
	if !strings.HasSuffix(string(b), "\r\n\r\nx") {
		t.Errorf("PATCH body tail wrong: %q", b)
	}
}

func TestBasicAuth(t *testing.T) {
	r, _ := NewRequest("GET", "/", "h", nil)
	r.BasicAuth("user", "pass")
	if v, _ := r.Get("Authorization"); v != "Basic dXNlcjpwYXNz" {
		t.Errorf("basic auth = %q", v)
	}
	r.ProxyBasicAuth("u", "p")
	if v, _ := r.Get("Proxy-Authorization"); v != "Basic dTpw" {
		t.Errorf("proxy basic auth = %q", v)
	}
}

func TestBytesRejectsCRLFInRequestLine(t *testing.T) {
	r, _ := NewRequest("GET", "/a\r\nb", "h", nil)
	if _, err := r.Bytes("1.1"); err == nil {
		t.Error("expected CR/LF request-line error")
	}
}

func TestBytesNoMethod(t *testing.T) {
	// A directly-zeroed Request (no method) must error from Bytes.
	r := &Request{Header: NewHeader()}
	if _, err := r.Bytes("1.1"); err == nil {
		t.Error("expected no-method error")
	}
}

func TestExplicitBodyDeletesTransferEncoding(t *testing.T) {
	r, _ := NewRequest("POST", "/", "h", nil)
	_ = r.Set("Transfer-Encoding", "chunked")
	r.SetBody([]byte("data"))
	b, _ := r.Bytes("1.1")
	if strings.Contains(string(b), "Transfer-Encoding") {
		t.Errorf("Transfer-Encoding should be deleted with a body: %q", b)
	}
}
