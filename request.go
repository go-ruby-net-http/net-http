// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

import (
	"encoding/base64"
	"errors"
	"strings"
)

// HAVE_ZLIB is always true in a stock MRI build, so the Accept-Encoding default
// below is always seeded; the constant is named to track the upstream guard.
const haveZlib = true

// defaultAcceptEncoding is the exact value MRI seeds for Accept-Encoding when
// the user did not set Accept-Encoding or Range (generic_request.rb).
const defaultAcceptEncoding = "gzip;q=1.0,deflate;q=0.6,identity;q=0.3"

// Request is the pure-Go port of Net::HTTPGenericRequest (the parent of every
// Net::HTTP::Get/Post/... request). It carries the method, the request path,
// the header model, and an optional body, and serialises itself to the exact
// HTTP/1.1 request bytes MRI writes to the socket.
type Request struct {
	*Header

	method      string
	path        string
	host        string // URI authority used for the default Host header, if any
	hasReqBody  bool
	hasRespBody bool

	body []byte
	// bodySet records that the body was explicitly set (vs. defaulted to ""),
	// matching MRI's distinction between @body == nil and @body == "".
	bodySet bool
}

// requestSpec captures the (request-body, response-body) permission flags MRI
// bakes into each request subclass.
type requestSpec struct {
	method      string
	hasReqBody  bool
	hasRespBody bool
}

var requestSpecs = map[string]requestSpec{
	"GET":     {"GET", false, true},
	"HEAD":    {"HEAD", false, false},
	"POST":    {"POST", true, true},
	"PUT":     {"PUT", true, true},
	"DELETE":  {"DELETE", false, true},
	"PATCH":   {"PATCH", true, true},
	"OPTIONS": {"OPTIONS", false, true},
}

// NewRequest builds a request for the given uppercase method ("GET", "POST", …)
// targeting path. path is the request-target as written on the request line
// (e.g. URI#request_uri: "/p?q=1"); host, when non-empty, supplies the default
// Host header (URI#authority). initHeader is the ordered list of caller-supplied
// initial fields; MRI preserves the initheader hash's insertion order on the
// wire, so the order of these pairs is the order they are emitted.
//
// It mirrors Net::HTTPGenericRequest#initialize: seeding Accept-Encoding (unless
// the caller's initheader already set accept-encoding or range) appended after
// the caller's fields, then the Accept, User-Agent and Host defaults.
func NewRequest(method, path, host string, initHeader [][2]string) (*Request, error) {
	spec, ok := requestSpecs[strings.ToUpper(method)]
	if !ok {
		return nil, errors.New("unknown HTTP method: " + method)
	}
	if path == "" {
		return nil, errors.New("HTTP request path is empty")
	}
	r := &Request{
		Header:      NewHeader(),
		method:      spec.method,
		path:        path,
		host:        host,
		hasReqBody:  spec.hasReqBody,
		hasRespBody: spec.hasRespBody,
	}

	// MRI seeds Accept-Encoding unless the user-supplied initheader already set
	// accept-encoding or range (case-insensitive). The seeded field is appended
	// to the dup'd initheader after the user's keys, so it sorts last among them.
	init := initHeader
	if haveZlib && !userHas(initHeader, "accept-encoding") && !userHas(initHeader, "range") {
		init = make([][2]string, 0, len(initHeader)+1)
		init = append(init, initHeader...)
		init = append(init, [2]string{"accept-encoding", defaultAcceptEncoding})
	}
	if err := r.initFrom(init); err != nil {
		return nil, err
	}
	if !r.Key("Accept") {
		r.rawSet("accept", []string{"*/*"})
	}
	if !r.Key("User-Agent") {
		r.rawSet("user-agent", []string{"Ruby"})
	}
	if host != "" && !r.Key("Host") {
		r.rawSet("host", []string{host})
	}
	return r, nil
}

func userHas(init [][2]string, name string) bool {
	for _, kv := range init {
		if strings.EqualFold(kv[0], name) {
			return true
		}
	}
	return false
}

// Method returns the request method ("GET", "POST", …).
func (r *Request) Method() string { return r.method }

// Path returns the request-target path written on the request line.
func (r *Request) Path() string { return r.path }

// RequestBodyPermitted reports whether the request may carry a body
// (Net::HTTPGenericRequest#request_body_permitted?).
func (r *Request) RequestBodyPermitted() bool { return r.hasReqBody }

// ResponseBodyPermitted reports whether a response to this request may carry a
// body (Net::HTTPGenericRequest#response_body_permitted?).
func (r *Request) ResponseBodyPermitted() bool { return r.hasRespBody }

// SetBody sets the request body (Net::HTTPGenericRequest#body=).
func (r *Request) SetBody(body []byte) {
	r.body = body
	r.bodySet = true
}

// Body returns the request body, or nil if none was set.
func (r *Request) Body() []byte { return r.body }

// SetFormData sets an application/x-www-form-urlencoded body from params,
// mirroring Net::HTTPHeader#set_form_data: it URL-encodes the pairs (default
// separator "&"), sets the body, and sets Content-Type.
func (r *Request) SetFormData(params [][2]string, sep ...string) {
	query := EncodeWWWForm(params)
	if len(sep) > 0 && sep[0] != "&" {
		query = strings.ReplaceAll(query, "&", sep[0])
	}
	r.SetBody([]byte(query))
	r.SetContentType("application/x-www-form-urlencoded")
}

// BasicAuth sets the Authorization header to a Basic credential for
// account/password (Net::HTTPHeader#basic_auth / #basic_encode).
func (r *Request) BasicAuth(account, password string) {
	r.rawSet("authorization", []string{basicEncode(account, password)})
}

// ProxyBasicAuth sets the Proxy-Authorization header (Net::HTTPHeader#proxy_basic_auth).
func (r *Request) ProxyBasicAuth(account, password string) {
	r.rawSet("proxy-authorization", []string{basicEncode(account, password)})
}

func basicEncode(account, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(account+":"+password))
}

// Bytes returns the exact HTTP/1.1 request byte stream MRI writes to the socket
// for this request at the given version (e.g. "1.1"). This is the host-side
// seam's payload: write these bytes to the connected socket.
//
// It mirrors Net::HTTPGenericRequest#exec: when a body is present (explicitly,
// or defaulted to "" for a body-permitted method), it sets Content-Length,
// deletes Transfer-Encoding, writes the header block, then the body — exactly
// send_request_with_body. Otherwise it writes the header block alone.
func (r *Request) Bytes(version string) ([]byte, error) {
	if r.method == "" {
		return nil, errors.New("no request method")
	}
	body := r.body
	// set_body_internal: a body-permitted method with no body set defaults to "".
	if !r.bodySet && r.hasReqBody {
		body = []byte{}
		r.bodySet = true
		r.body = body
	}
	hasBody := r.bodySet

	if hasBody {
		r.SetContentLength(len(body))
		r.Delete("transfer-encoding")
	}

	reqline := r.method + " " + r.path + " HTTP/" + version
	if strings.ContainsAny(reqline, "\r\n") {
		return nil, errors.New("A Request-Line must not contain CR or LF")
	}

	var b strings.Builder
	b.WriteString(reqline)
	b.WriteString("\r\n")
	r.EachCapitalized(func(k, v string) {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	})
	b.WriteString("\r\n")

	out := []byte(b.String())
	if hasBody {
		out = append(out, body...)
	}
	return out, nil
}
