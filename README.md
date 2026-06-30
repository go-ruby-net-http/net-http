<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-net-http/brand/main/social/go-ruby-net-http-net-http.png" alt="go-ruby-net-http/net-http" width="720"></p>

# net-http — go-ruby-net-http

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-net-http.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's
[Net::HTTP](https://docs.ruby-lang.org/en/master/Net/HTTP.html) HTTP/1.1
*message* codec** — the deterministic, interpreter-independent core of MRI
4.0.5's `Net::HTTP`: it builds request bytes exactly as MRI writes them to the
socket, parses a raw HTTP/1.1 response byte stream into MRI's
`Net::HTTPResponse` subclass model, and ports the `Net::HTTPHeader` mixin —
**without any Ruby runtime, and without performing any I/O itself**.

It is the HTTP-message backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a
sibling of [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo
engine), [go-ruby-erb](https://github.com/go-ruby-erb/erb) (the ERB compiler) and
[go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (the Psych codec).

> **The socket / TLS is a host-side seam.** Building HTTP/1.1 request bytes
> (request line, default headers, `Content-Length` / chunked framing, form and
> basic-auth encoding) and parsing a response byte stream (status line, folded
> multi-value headers, `Content-Length` *and* chunked decoding, the response
> subclass selected by status code) is fully deterministic and needs **no
> interpreter**, so it lives here as pure Go. Opening the `TCPSocket`, doing the
> TLS handshake, and reading/writing the bytes is the host's job (rbgo supplies
> the byte transport): hand `Request.Bytes` to your socket's write, and feed
> everything read back from the socket to `ParseResponse`.

## Features

Faithful port of `Net::HTTP`'s request build + response parse, validated against
the `ruby` binary on every supported platform:

- **Request building** for `Get` / `Post` / `Put` / `Delete` / `Head` / `Patch` /
  `Options` — the request line, MRI's default headers in MRI's exact order and
  casing (`Accept-Encoding`, `Accept`, `User-Agent`, `Host`), `Content-Length`
  vs. `Transfer-Encoding` handling, the empty-body default for body-permitting
  methods, `set_form_data` (`application/x-www-form-urlencoded`), and HTTP Basic
  / Proxy-Basic auth — **byte-for-byte identical to MRI's socket writes**.
- **Response parsing** of a raw HTTP/1.1 byte stream — the status line
  (`/\AHTTP(?:\/\d+\.\d+)?\s+\d\d\d(?:\s+.*)?\z/in`), header lines with obs-fold
  continuation and repeated multi-value fields, and the body decoded by
  **`Content-Length`** *or* **chunked `Transfer-Encoding`** (chunk extensions and
  trailers included).
- **The `Net::HTTPResponse` subclass hierarchy** — every status code mapped to
  its subclass (`HTTPOK`, `HTTPNotFound`, `HTTPMovedPermanently`, …) and category
  (`HTTPSuccess` / `HTTPRedirection` / `HTTPClientError` / `HTTPServerError` /
  `HTTPInformation`), generated from MRI's own `CODE_TO_OBJ` table, with the
  `HAS_BODY` rule (1xx, 204, 205, 304 carry no body) and the `CODE_CLASS_TO_OBJ`
  / `HTTPUnknownResponse` fallbacks.
- **The `Net::HTTPHeader` mixin** — `[]` / `[]=` / `key?` / `delete` /
  `add_field` / `get_fields` / `each_header` / `each_capitalized`, plus
  `content_type` / `content_length` / `chunked?` / `connection_close?` and the
  `URI.encode_www_form` component encoder.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x) and three operating systems (Linux, macOS, Windows).

## Install

```sh
go get github.com/go-ruby-net-http/net-http
```

## Usage

```go
package main

import (
	"fmt"

	nethttp "github.com/go-ruby-net-http/net-http"
)

func main() {
	// Build the request bytes (Net::HTTP::Post.new + set_form_data).
	req, _ := nethttp.NewRequest("POST", "/submit", "example.com", nil)
	req.SetFormData([][2]string{{"name", "a b"}, {"x", "1&2"}})
	wire, _ := req.Bytes("1.1")
	// POST /submit HTTP/1.1
	// Accept-Encoding: gzip;q=1.0,deflate;q=0.6,identity;q=0.3
	// Accept: */*
	// User-Agent: Ruby
	// Host: example.com
	// Content-Type: application/x-www-form-urlencoded
	// Content-Length: 16
	//
	// name=a+b&x=1%262
	//
	// ... the host writes `wire` to its connected (TLS) socket, then reads the
	// whole response back and hands the bytes to ParseResponse:

	raw := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"
	res, _ := nethttp.ParseResponse([]byte(raw))
	fmt.Println(res.Class(), res.Code(), res.Message()) // HTTPOK 200 OK
	fmt.Println(res.IsSuccess())                        // true
	fmt.Printf("%q\n", res.Body())                      // "Wikipedia" (chunked-decoded)
	_ = wire
}
```

## The socket / TLS seam

This library is the message codec only; the transport is the host's:

| Stage                       | Owner          | This library                                |
| --------------------------- | -------------- | ------------------------------------------- |
| DNS, `TCPSocket`, TLS       | host (rbgo)    | —                                           |
| serialise the request       | this library   | `Request.Bytes(version) []byte`             |
| write bytes to the socket   | host (rbgo)    | —                                           |
| read bytes from the socket  | host (rbgo)    | —                                           |
| parse the response          | this library   | `ParseResponse([]byte) (*Response, error)`  |

`Request.Bytes` is the inverse of `ParseResponse`. Neither touches the network,
a file, or a clock, so the codec is fully deterministic and testable in
isolation — exactly how the host can drive it from any byte transport.

## API

```go
// Request building (Net::HTTPGenericRequest + the Get/Post/... subclasses).
func NewRequest(method, path, host string, initHeader [][2]string) (*Request, error)
func (r *Request) Bytes(version string) ([]byte, error) // exact MRI socket bytes
func (r *Request) SetBody(body []byte)
func (r *Request) SetFormData(params [][2]string, sep ...string) // set_form_data
func (r *Request) BasicAuth(account, password string)           // basic_auth
func (r *Request) ProxyBasicAuth(account, password string)
func (r *Request) Method() string
func (r *Request) Path() string
func (r *Request) RequestBodyPermitted() bool
func (r *Request) ResponseBodyPermitted() bool

// Response parsing (Net::HTTPResponse.read_new + read_body).
func ParseResponse(raw []byte) (*Response, error)
func (r *Response) Code() string        // "200"
func (r *Response) Message() string     // "OK"
func (r *Response) HTTPVersion() string // "1.1"
func (r *Response) Class() string       // "HTTPOK"
func (r *Response) Category() string    // "HTTPSuccess"
func (r *Response) Body() []byte        // decoded (Content-Length or chunked)
func (r *Response) IsSuccess() bool     // kind_of? Net::HTTPSuccess (+ Is{Information,Redirection,ClientError,ServerError})

// The Net::HTTPHeader mixin, embedded in both Request and Response.
func (h *Header) Get(key string) (string, bool)        // []
func (h *Header) Set(key, val string) error            // []=
func (h *Header) AddField(key, val string) error       // add_field
func (h *Header) GetFields(key string) []string        // get_fields
func (h *Header) EachHeader(fn func(key, value string)) // each_header
func (h *Header) ContentType() string                  // content_type
func (h *Header) ContentLength() (int, bool, error)    // content_length
func (h *Header) Chunked() bool                        // chunked?
func (h *Header) ConnectionClose() bool                // connection_close?

// URI.encode_www_form helpers.
func EncodeWWWFormComponent(s string) string
func EncodeWWWForm(pairs [][2]string) string
```

## Response subclass model

Like MRI, a parsed response carries the *subclass identity* its status selects —
exposed as `Class()` / `Category()` and the `Is*` kind predicates rather than a
distinct Go type per code:

| Code(s)            | `Class()`              | `Category()`        | Body? |
| ------------------ | ---------------------- | ------------------- | ----- |
| `200`              | `HTTPOK`               | `HTTPSuccess`       | yes   |
| `204` / `304`      | `HTTPNoContent` / `HTTPNotModified` | `HTTPSuccess` / `HTTPRedirection` | no |
| `301`              | `HTTPMovedPermanently` | `HTTPRedirection`   | yes   |
| `404`              | `HTTPNotFound`         | `HTTPClientError`   | yes   |
| `500`              | `HTTPInternalServerError` | `HTTPServerError` | yes   |
| unknown 2xx (`299`)| `HTTPSuccess`          | `HTTPSuccess`       | yes   |
| unknown (`999`)    | `HTTPUnknownResponse`  | `HTTPUnknownResponse` | yes |

## Tests & coverage

The suite pairs deterministic, ruby-free tests (which alone hold coverage at
100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential MRI oracle**: the same requests are serialised here and by the
system `ruby` (`Net::HTTPGenericRequest#exec` writing to a recording socket) and
compared **byte-for-byte**; responses are parsed both here and by
`Net::HTTPResponse.read_new` (status, multi-value headers, chunked-decoded body,
selected subclass over MRI's whole `CODE_TO_OBJ` table) and compared. The oracle
scripts `$stdout.binmode` so Windows text-mode never pollutes the bytes, and skip
themselves where `ruby` is absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-net-http/net-http authors.
