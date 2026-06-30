// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import "testing"

func TestEncodeWWWFormComponent(t *testing.T) {
	cases := map[string]string{
		"a b":        "a+b",
		"a.b-c_d~e":  "a.b-c_d%7Ee", // ~ is escaped, . - _ are not
		"*":          "*",
		"1&2":        "1%262",
		"a=b":        "a%3Db",
		"\xc3\xa9":   "%C3%A9", // é UTF-8 bytes
		"":           "",
		"AZaz09":     "AZaz09",
		"slash/here": "slash%2Fhere",
	}
	for in, want := range cases {
		if got := EncodeWWWFormComponent(in); got != want {
			t.Errorf("EncodeWWWFormComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEncodeWWWForm(t *testing.T) {
	got := EncodeWWWForm([][2]string{{"a", "1"}, {"b", "x y"}, {"c", ""}})
	if got != "a=1&b=x+y&c=" {
		t.Errorf("EncodeWWWForm = %q", got)
	}
	if EncodeWWWForm(nil) != "" {
		t.Error("empty form not empty")
	}
}
