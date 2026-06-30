// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import "testing"

func TestClassForCodeExact(t *testing.T) {
	cases := []struct {
		code, class, cat string
		hasBody          bool
	}{
		{"200", "HTTPOK", "HTTPSuccess", true},
		{"404", "HTTPNotFound", "HTTPClientError", true},
		{"301", "HTTPMovedPermanently", "HTTPRedirection", true},
		{"500", "HTTPInternalServerError", "HTTPServerError", true},
		{"100", "HTTPContinue", "HTTPInformation", false},
		{"204", "HTTPNoContent", "HTTPSuccess", false},
		{"304", "HTTPNotModified", "HTTPRedirection", false},
	}
	for _, c := range cases {
		class, cat, hb := classForCode(c.code)
		if class != c.class || cat != c.cat || hb != c.hasBody {
			t.Errorf("classForCode(%s) = %q/%q/%v, want %q/%q/%v",
				c.code, class, cat, hb, c.class, c.cat, c.hasBody)
		}
	}
}

func TestClassForCodeCategoryFallback(t *testing.T) {
	cases := []struct {
		code, class string
		hasBody     bool
	}{
		{"199", "HTTPInformation", false},
		{"250", "HTTPSuccess", true},
		{"399", "HTTPRedirection", true},
		{"450", "HTTPClientError", true},
		{"599", "HTTPServerError", true},
	}
	for _, c := range cases {
		class, cat, hb := classForCode(c.code)
		if class != c.class || cat != c.class || hb != c.hasBody {
			t.Errorf("classForCode(%s) fallback = %q/%q/%v, want %q/%v",
				c.code, class, cat, hb, c.class, c.hasBody)
		}
	}
}

func TestClassForCodeUnknown(t *testing.T) {
	// First digit outside 1..5 → HTTPUnknownResponse (HAS_BODY true).
	class, cat, hb := classForCode("999")
	if class != "HTTPUnknownResponse" || cat != "HTTPUnknownResponse" || !hb {
		t.Errorf("999 = %q/%q/%v", class, cat, hb)
	}
	// Empty code → unknown.
	if class, _, _ := classForCode(""); class != "HTTPUnknownResponse" {
		t.Errorf("empty code = %q", class)
	}
}

func TestKindPredicatesAllCategories(t *testing.T) {
	for _, tc := range []struct {
		code string
		want func(*Response) bool
	}{
		{"100", (*Response).IsInformation},
		{"307", (*Response).IsRedirection},
		{"503", (*Response).IsServerError},
	} {
		raw := "HTTP/1.1 " + tc.code + " X\r\nContent-Length: 0\r\n\r\n"
		res, err := ParseResponse([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		if !tc.want(res) {
			t.Errorf("%s kind predicate false", tc.code)
		}
	}
}

func TestHeaderSyntaxErrorMessage(t *testing.T) {
	e := &HeaderSyntaxError{Msg: "boom"}
	if e.Error() != "boom" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestFirstHexRunTrailing(t *testing.T) {
	// A hex run reaching end-of-string exercises the trailing-run return.
	if got := firstHexRun("ff"); got != "ff" {
		t.Errorf("firstHexRun(ff) = %q", got)
	}
	if got := firstHexRun(""); got != "" {
		t.Errorf("firstHexRun(empty) = %q", got)
	}
	if got := firstHexRun("zz"); got != "" {
		t.Errorf("firstHexRun(zz) = %q", got)
	}
}

func TestFoldingOntoEmptyValue(t *testing.T) {
	// A header whose first line has an empty value, then a continuation line:
	// the value != "" guard's false branch is taken (no leading space inserted).
	raw := "HTTP/1.1 200 OK\r\nX-Empty:\r\n cont\r\nContent-Length: 0\r\n\r\n"
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := res.Get("X-Empty"); v != "cont" {
		t.Errorf("folded-onto-empty = %q", v)
	}
}

func TestCodeTableComplete(t *testing.T) {
	if len(codeTable) != len(codeToEntry) {
		t.Errorf("codeTable has %d rows but index has %d", len(codeTable), len(codeToEntry))
	}
	// Spot-check the index is consistent with the table.
	for _, e := range codeTable {
		got, ok := codeToEntry[e.Code]
		if !ok || got != e {
			t.Errorf("index mismatch for %s", e.Code)
		}
	}
}
