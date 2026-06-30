// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// rubyBin locates a usable `ruby` once and gates the oracle to the targeted MRI
// major version (4.x). The differential tests reproduce MRI 4.0.5 byte-for-byte;
// older lines (e.g. 3.4) diverge in Net::HTTP details such as the empty-body and
// set_form_data Content-Type defaulting, so the oracle self-skips off-version.
// It also skips when `ruby` is absent (the qemu cross-arch and Windows lanes),
// so the deterministic suite alone drives the 100% gate there.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping MRI oracle")
	}
	out, err := exec.Command(path, "-e", "print RUBY_VERSION").Output()
	if err != nil {
		t.Skipf("cannot determine ruby version: %v", err)
	}
	major, _, _ := strings.Cut(string(out), ".")
	if major != "4" {
		t.Skipf("MRI oracle targets ruby 4.x; found %s", out)
	}
	return path
}

// rubyEval runs a Ruby script and returns its stdout. Every script $stdout.binmode
// itself (the go-ruby-erb lesson) so Windows text-mode never pollutes the bytes;
// the shared preamble does so and requires net/http + stringio.
func rubyEval(t *testing.T, bin, script string) string {
	t.Helper()
	preamble := "$stdout.binmode\nrequire 'net/http'\nrequire 'stringio'\n"
	cmd := exec.Command(bin, "-e", preamble+script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nscript:\n%s\noutput:\n%s", err, script, out)
	}
	return string(out)
}

// recSock is the Ruby helper that records the exact bytes a request writes to a
// socket via Net::HTTPGenericRequest#exec, including the body. It supplies just
// the socket surface exec touches.
const recSock = `
class Rec
  attr_reader :buf
  def initialize; @buf = +""; end
  def write(*a); a.each { |s| @buf << s }; a.map(&:bytesize).sum; end
  def continue_timeout; nil; end
end
def emit(req, path, ver = '1.1')
  # Mirror Net::HTTP#request: set_body_internal runs before exec, so a
  # body-permitted method with no body set defaults its body to "".
  req.set_body_internal(nil)
  s = Rec.new
  req.exec(s, ver, path)
  s.buf
end
`

// TestOracleRequestBytes builds the same requests here and in MRI and asserts the
// request byte streams are identical — the core "match MRI byte-for-byte on the
// request bytes" requirement.
func TestOracleRequestBytes(t *testing.T) {
	bin := rubyBin(t)

	type want struct {
		name  string
		build func() *Request // Go-side request
		ruby  string          // Ruby that prints the wire bytes via emit(...)
	}
	cases := []want{
		{
			name: "get",
			build: func() *Request {
				r, _ := NewRequest("GET", "/path?q=1", "example.com", nil)
				return r
			},
			ruby: `print emit(Net::HTTP::Get.new(URI('http://example.com/path?q=1')), '/path?q=1')`,
		},
		{
			name: "get-no-host",
			build: func() *Request {
				r, _ := NewRequest("GET", "/", "", nil)
				return r
			},
			ruby: `print emit(Net::HTTP::Get.new('/'), '/')`,
		},
		{
			name: "post-form",
			build: func() *Request {
				r, _ := NewRequest("POST", "/submit", "example.com", nil)
				r.SetFormData([][2]string{{"name", "a b"}, {"x", "1&2"}})
				return r
			},
			ruby: `r = Net::HTTP::Post.new(URI('http://example.com/submit'))
r.set_form_data({'name'=>'a b','x'=>'1&2'})
print emit(r, '/submit')`,
		},
		{
			name: "post-explicit-body",
			build: func() *Request {
				r, _ := NewRequest("POST", "/api", "h", nil)
				r.SetBody([]byte("hello world"))
				_ = r.Set("Content-Type", "text/plain")
				return r
			},
			// Host from the URI authority (emitted last), matching the Go host arg.
			ruby: `r = Net::HTTP::Post.new(URI('http://h/api'))
r.body = "hello world"
r['Content-Type'] = 'text/plain'
print emit(r, '/api')`,
		},
		{
			name: "put-default-empty-body",
			build: func() *Request {
				r, _ := NewRequest("PUT", "/r", "h", nil)
				return r
			},
			ruby: `print emit(Net::HTTP::Put.new(URI('http://h/r')), '/r')`,
		},
		{
			name: "head",
			build: func() *Request {
				r, _ := NewRequest("HEAD", "/r", "h", nil)
				return r
			},
			ruby: `print emit(Net::HTTP::Head.new(URI('http://h/r')), '/r')`,
		},
		{
			name: "delete",
			build: func() *Request {
				r, _ := NewRequest("DELETE", "/z", "h", nil)
				return r
			},
			ruby: `print emit(Net::HTTP::Delete.new(URI('http://h/z')), '/z')`,
		},
		{
			name: "patch",
			build: func() *Request {
				r, _ := NewRequest("PATCH", "/p", "h", nil)
				r.SetBody([]byte("delta"))
				return r
			},
			ruby: `r = Net::HTTP::Patch.new(URI('http://h/p'))
r.body = "delta"
print emit(r, '/p')`,
		},
		{
			name: "options",
			build: func() *Request {
				r, _ := NewRequest("OPTIONS", "/z", "h", nil)
				return r
			},
			ruby: `print emit(Net::HTTP::Options.new(URI('http://h/z')), '/z')`,
		},
		{
			name: "basic-auth",
			build: func() *Request {
				r, _ := NewRequest("GET", "/", "h", nil)
				r.BasicAuth("user", "pass")
				return r
			},
			ruby: `r = Net::HTTP::Get.new(URI('http://h/'))
r.basic_auth('user','pass')
print emit(r, '/')`,
		},
		{
			name: "user-accept-encoding-suppresses-seed",
			build: func() *Request {
				r, _ := NewRequest("GET", "/", "", [][2]string{{"Accept-Encoding", "identity"}, {"X-Foo", "bar"}})
				return r
			},
			// Ruby hash preserves insertion order, matching our ordered pairs.
			ruby: `print emit(Net::HTTP::Get.new('/', {'Accept-Encoding'=>'identity','X-Foo'=>'bar'}), '/')`,
		},
		{
			name: "custom-header-order",
			build: func() *Request {
				r, _ := NewRequest("GET", "/", "", [][2]string{{"Zeta", "1"}, {"Alpha", "2"}, {"Host", "h"}})
				return r
			},
			ruby: `print emit(Net::HTTP::Get.new('/', {'Zeta'=>'1','Alpha'=>'2','Host'=>'h'}), '/')`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			goBytes, err := c.build().Bytes("1.1")
			if err != nil {
				t.Fatalf("Go Bytes: %v", err)
			}
			mri := rubyEval(t, bin, recSock+c.ruby)
			if string(goBytes) != mri {
				t.Errorf("request bytes differ:\n go:  %q\n mri: %q", goBytes, mri)
			}
		})
	}
}

// TestOracleFormEncoding cross-checks EncodeWWWFormComponent against MRI's
// URI.encode_www_form_component over a tricky corpus.
func TestOracleFormEncoding(t *testing.T) {
	bin := rubyBin(t)
	inputs := []string{"a b", "a.b-c_d~e", "1&2", "a=b", "*", "café", "x/y?z#w", "100%"}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			got := EncodeWWWFormComponent(in)
			script := "require 'uri'\nprint URI.encode_www_form_component(" +
				rubyStringLiteral(in) + ")"
			want := rubyEval(t, bin, script)
			if got != want {
				t.Errorf("encode(%q) = %q, MRI = %q", in, got, want)
			}
		})
	}
}

// TestOracleResponseSubclass parses a corpus of raw responses both here and in
// MRI (Net::HTTPResponse.read_new) and asserts the selected subclass name and
// the parsed status match for every code in MRI's table.
func TestOracleResponseSubclass(t *testing.T) {
	bin := rubyBin(t)
	// Walk MRI's own CODE_TO_OBJ so this stays in lockstep with the table.
	listScript := `Net::HTTPResponse::CODE_TO_OBJ.sort_by{|k,_| k}.each do |code, klass|
  puts "#{code}\t#{klass.name.sub('Net::','')}"
end`
	table := rubyEval(t, bin, listScript)
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		code, mriClass := parts[0], parts[1]
		raw := "HTTP/1.1 " + code + " Reason Phrase\r\nContent-Length: 0\r\n\r\n"
		res, err := ParseResponse([]byte(raw))
		if err != nil {
			t.Fatalf("ParseResponse(%s): %v", code, err)
		}
		if res.Class() != mriClass {
			t.Errorf("code %s: Go class %q, MRI %q", code, res.Class(), mriClass)
		}
	}
}

// TestOracleResponseParse has MRI parse a response we then parse here and checks
// the status line, a multi-value header, and the chunked-decoded body agree.
func TestOracleResponseParse(t *testing.T) {
	bin := rubyBin(t)

	// MRI reads a chunked response and reports code/message/body; we parse the
	// same bytes and compare. The raw bytes are built in Ruby and printed so both
	// sides operate on the identical stream.
	script := recSock + `
raw = "HTTP/1.1 200 OK\r\n" \
      "Content-Type: text/plain\r\n" \
      "Transfer-Encoding: chunked\r\n" \
      "Set-Cookie: a=1\r\n" \
      "Set-Cookie: b=2\r\n" \
      "\r\n" \
      "4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"
io = Net::BufferedIO.new(StringIO.new(raw))
res = Net::HTTPResponse.read_new(io)
res.reading_body(io, true) {}
print "RAW\x00" + raw + "\x00"
print "CODE\x00#{res.code}\x00"
print "MSG\x00#{res.message}\x00"
print "CLASS\x00#{res.class.name.sub('Net::','')}\x00"
print "BODY\x00#{res.body}\x00"
print "COOKIES\x00#{res.get_fields('Set-Cookie').join('|')}\x00"
print "CTYPE\x00#{res.content_type}\x00"
print "CHUNKED\x00#{res.chunked?}\x00"
`
	out := rubyEval(t, bin, script)
	fields := parseNullFields(out)

	raw := fields["RAW"]
	res, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("ParseResponse: %v\nraw: %q", err, raw)
	}
	if res.Code() != fields["CODE"] {
		t.Errorf("code: Go %q, MRI %q", res.Code(), fields["CODE"])
	}
	if res.Message() != fields["MSG"] {
		t.Errorf("message: Go %q, MRI %q", res.Message(), fields["MSG"])
	}
	if res.Class() != fields["CLASS"] {
		t.Errorf("class: Go %q, MRI %q", res.Class(), fields["CLASS"])
	}
	if string(res.Body()) != fields["BODY"] {
		t.Errorf("body: Go %q, MRI %q", res.Body(), fields["BODY"])
	}
	if res.ContentType() != fields["CTYPE"] {
		t.Errorf("content_type: Go %q, MRI %q", res.ContentType(), fields["CTYPE"])
	}
	gotCookies := strings.Join(res.GetFields("Set-Cookie"), "|")
	if gotCookies != fields["COOKIES"] {
		t.Errorf("cookies: Go %q, MRI %q", gotCookies, fields["COOKIES"])
	}
	// Note: after reading_body MRI's chunked? still reflects the header.
	if (res.Chunked() && fields["CHUNKED"] != "true") || (!res.Chunked() && fields["CHUNKED"] == "true") {
		t.Errorf("chunked?: Go %v, MRI %q", res.Chunked(), fields["CHUNKED"])
	}
}

// TestOracleResponseContentLength has MRI emit and read a Content-Length-framed
// response; we parse the same bytes and compare the body.
func TestOracleResponseContentLength(t *testing.T) {
	bin := rubyBin(t)
	script := `
raw = "HTTP/1.1 201 Created\r\nContent-Length: 11\r\n\r\nhello world"
io = Net::BufferedIO.new(StringIO.new(raw))
res = Net::HTTPResponse.read_new(io)
res.reading_body(io, true) {}
print "RAW\x00" + raw + "\x00"
print "CLASS\x00#{res.class.name.sub('Net::','')}\x00"
print "BODY\x00#{res.body}\x00"
print "CLEN\x00#{res.content_length}\x00"
`
	fields := parseNullFields(rubyEval(t, bin, script))
	res, err := ParseResponse([]byte(fields["RAW"]))
	if err != nil {
		t.Fatal(err)
	}
	if res.Class() != fields["CLASS"] {
		t.Errorf("class: Go %q, MRI %q", res.Class(), fields["CLASS"])
	}
	if string(res.Body()) != fields["BODY"] {
		t.Errorf("body: Go %q, MRI %q", res.Body(), fields["BODY"])
	}
	clen, _, _ := res.ContentLength()
	if strconv.Itoa(clen) != fields["CLEN"] {
		t.Errorf("content_length: Go %d, MRI %q", clen, fields["CLEN"])
	}
}

// parseNullFields splits a "KEY\x00VALUE\x00KEY\x00VALUE\x00…" stream emitted by
// the oracle scripts into a map. NUL is used so header/body bytes (which contain
// CRLF and arbitrary text) survive intact.
func parseNullFields(s string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(s, "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		out[parts[i]] = parts[i+1]
	}
	return out
}

// rubyStringLiteral renders s as a double-quoted Ruby string literal with the
// bytes escaped so the oracle script reconstructs s exactly.
func rubyStringLiteral(s string) string {
	var b strings.Builder
	b.WriteString(`"`)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c >= 0x20 && c < 0x7f:
			b.WriteByte(c)
		default:
			b.WriteString("\\x")
			const hex = "0123456789ABCDEF"
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	b.WriteString(`".b.force_encoding("UTF-8")`)
	return b.String()
}
