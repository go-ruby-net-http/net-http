// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import (
	"reflect"
	"strings"
	"testing"
)

func TestHeaderSetGet(t *testing.T) {
	h := NewHeader()
	if err := h.Set("Content-Type", "text/html"); err != nil {
		t.Fatal(err)
	}
	if v, ok := h.Get("content-type"); !ok || v != "text/html" {
		t.Errorf("Get = %q,%v", v, ok)
	}
	if _, ok := h.Get("missing"); ok {
		t.Error("missing key reported present")
	}
	if !h.Key("CONTENT-TYPE") {
		t.Error("Key case-insensitive failed")
	}
	if h.Key("nope") {
		t.Error("Key reported absent key present")
	}
}

func TestHeaderSetCRLFRejected(t *testing.T) {
	h := NewHeader()
	if err := h.Set("X", "a\r\nb"); err == nil {
		t.Error("expected CR/LF rejection")
	}
}

func TestHeaderEnsureNilMap(t *testing.T) {
	// A zero-value Header (nil map) must lazily initialise on every accessor.
	var h Header
	if _, ok := h.Get("x"); ok {
		t.Error("nil-map Get reported present")
	}
	if h.Key("x") {
		t.Error("nil-map Key true")
	}
	if h.GetFields("x") != nil {
		t.Error("nil-map GetFields non-nil")
	}
	if h.Delete("x") != nil {
		t.Error("nil-map Delete non-nil")
	}
	got := 0
	h.EachHeader(func(string, string) { got++ })
	h.EachCapitalized(func(string, string) { got++ })
	if got != 0 {
		t.Errorf("nil-map iterate yielded %d", got)
	}
	if err := h.AddField("x", "1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := h.Get("x"); v != "1" {
		t.Errorf("AddField on nil map = %q", v)
	}
}

func TestHeaderAddFieldAndGetFields(t *testing.T) {
	h := NewHeader()
	if err := h.AddField("Set-Cookie", "a=1"); err != nil {
		t.Fatal(err)
	}
	if err := h.AddField("set-cookie", "b=2"); err != nil {
		t.Fatal(err)
	}
	got := h.GetFields("Set-Cookie")
	if !reflect.DeepEqual(got, []string{"a=1", "b=2"}) {
		t.Errorf("GetFields = %#v", got)
	}
	// joined form
	if v, _ := h.Get("set-cookie"); v != "a=1, b=2" {
		t.Errorf("joined = %q", v)
	}
	// mutating the returned slice must not affect internal state
	got[0] = "x"
	if again := h.GetFields("set-cookie"); again[0] != "a=1" {
		t.Error("GetFields did not copy")
	}
	if h.GetFields("absent") != nil {
		t.Error("GetFields(absent) != nil")
	}
}

func TestHeaderAddFieldCRLF(t *testing.T) {
	h := NewHeader()
	if err := h.AddField("X", "a\nb"); err == nil {
		t.Error("expected CR/LF rejection in AddField")
	}
}

func TestHeaderDelete(t *testing.T) {
	h := NewHeader()
	_ = h.Set("A", "1")
	_ = h.Set("B", "2")
	_ = h.Set("C", "3")
	prev := h.Delete("b")
	if !reflect.DeepEqual(prev, []string{"2"}) {
		t.Errorf("Delete returned %#v", prev)
	}
	if h.Key("B") {
		t.Error("B still present")
	}
	// order should now be A, C
	var keys []string
	h.EachHeader(func(k, _ string) { keys = append(keys, k) })
	if !reflect.DeepEqual(keys, []string{"a", "c"}) {
		t.Errorf("order after delete = %#v", keys)
	}
	// deleting absent key returns nil and leaves order intact
	if h.Delete("zzz") != nil {
		t.Error("delete absent returned non-nil")
	}
}

func TestHeaderEachCapitalized(t *testing.T) {
	h := NewHeader()
	_ = h.Set("content-md5", "x")
	_ = h.Set("x-custom-foo", "y")
	_ = h.Set("", "empty") // empty key token path
	var pairs [][2]string
	h.EachCapitalized(func(k, v string) { pairs = append(pairs, [2]string{k, v}) })
	want := [][2]string{{"Content-Md5", "x"}, {"X-Custom-Foo", "y"}, {"", "empty"}}
	if !reflect.DeepEqual(pairs, want) {
		t.Errorf("EachCapitalized = %#v", pairs)
	}
}

func TestCapitalize(t *testing.T) {
	cases := map[string]string{
		"content-type": "Content-Type",
		"host":         "Host",
		"a":            "A",
		"":             "",
		"x--y":         "X--Y",
	}
	for in, want := range cases {
		if got := capitalize(in); got != want {
			t.Errorf("capitalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInitFromValidation(t *testing.T) {
	h := NewHeader()
	if err := h.initFrom([][2]string{{strings.Repeat("k", maxKeyLength+1), "v"}}); err == nil {
		t.Error("expected too-long-key error")
	}
	h2 := NewHeader()
	if err := h2.initFrom([][2]string{{"k", strings.Repeat("v", maxFieldLength+1)}}); err == nil {
		t.Error("expected too-long-value error")
	}
	h3 := NewHeader()
	if err := h3.initFrom([][2]string{{"k", "a\r\nb"}}); err == nil {
		t.Error("expected CR/LF error")
	}
	h4 := NewHeader()
	if err := h4.initFrom([][2]string{{"k", "  trimmed  "}}); err != nil {
		t.Fatal(err)
	}
	if v, _ := h4.Get("k"); v != "trimmed" {
		t.Errorf("value not stripped: %q", v)
	}
}

func TestContentLength(t *testing.T) {
	h := NewHeader()
	if _, ok, err := h.ContentLength(); ok || err != nil {
		t.Errorf("absent ContentLength: ok=%v err=%v", ok, err)
	}
	h.SetContentLength(42)
	if n, ok, err := h.ContentLength(); !ok || err != nil || n != 42 {
		t.Errorf("ContentLength = %d,%v,%v", n, ok, err)
	}
	// embedded digits in a noisy value
	_ = h.Set("Content-Length", "bytes 99")
	if n, _, _ := h.ContentLength(); n != 99 {
		t.Errorf("noisy ContentLength = %d", n)
	}
	// malformed: no digits at all
	_ = h.Set("Content-Length", "none")
	if _, _, err := h.ContentLength(); err == nil {
		t.Error("expected HeaderSyntaxError for non-numeric")
	} else if _, ok := err.(*HeaderSyntaxError); !ok {
		t.Errorf("wrong error type %T", err)
	}
}

func TestFirstDigits(t *testing.T) {
	cases := map[string]string{
		"abc123def": "123",
		"42":        "42",
		"x9":        "9",
		"none":      "",
		"":          "",
		"7end":      "7",
	}
	for in, want := range cases {
		if got := firstDigits(in); got != want {
			t.Errorf("firstDigits(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContentTypeAccessors(t *testing.T) {
	h := NewHeader()
	if h.ContentType() != "" || h.MainType() != "" || h.SubType() != "" {
		t.Error("empty content-type accessors not empty")
	}
	h.SetContentType("text/html", [2]string{"charset", "utf-8"})
	if v, _ := h.Get("content-type"); v != "text/html; charset=utf-8" {
		t.Errorf("set_content_type = %q", v)
	}
	if h.MainType() != "text" || h.SubType() != "html" || h.ContentType() != "text/html" {
		t.Errorf("type accessors = %q/%q/%q", h.MainType(), h.SubType(), h.ContentType())
	}
	// main-only (no slash)
	_ = h.Set("Content-Type", "application")
	if h.ContentType() != "application" || h.SubType() != "" {
		t.Errorf("main-only ContentType = %q sub %q", h.ContentType(), h.SubType())
	}
}

func TestChunked(t *testing.T) {
	mk := func(te string) *Header {
		h := NewHeader()
		_ = h.Set("Transfer-Encoding", te)
		return h
	}
	if NewHeader().Chunked() {
		t.Error("no TE reported chunked")
	}
	chunkedCases := []string{"chunked", "gzip, chunked", "CHUNKED", "gzip,chunked"}
	for _, c := range chunkedCases {
		if !mk(c).Chunked() {
			t.Errorf("%q not detected as chunked", c)
		}
	}
	nonChunked := []string{"gzip", "x-chunked", "chunkedy", "unchunked-foo"}
	for _, c := range nonChunked {
		if mk(c).Chunked() {
			t.Errorf("%q wrongly detected as chunked", c)
		}
	}
}

func TestConnectionTokens(t *testing.T) {
	mk := func(key, val string) *Header {
		h := NewHeader()
		_ = h.Set(key, val)
		return h
	}
	if !mk("Connection", "close").ConnectionClose() {
		t.Error("close not detected")
	}
	if !mk("Connection", "keep-alive, foo").ConnectionKeepAlive() {
		t.Error("keep-alive not detected")
	}
	if !mk("Proxy-Connection", "close").ConnectionClose() {
		t.Error("proxy close not detected")
	}
	if mk("Connection", "keep-alive").ConnectionClose() {
		t.Error("keep-alive wrongly close")
	}
	if NewHeader().ConnectionClose() || NewHeader().ConnectionKeepAlive() {
		t.Error("absent connection reported a token")
	}
	// multi-value via add_field
	h := NewHeader()
	_ = h.AddField("Connection", "te")
	_ = h.AddField("Connection", "close")
	if !h.ConnectionClose() {
		t.Error("multi-value close not detected")
	}
}
