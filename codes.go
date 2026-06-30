// Copyright (c) the go-ruby-net-http/net-http authors
//
// SPDX-License-Identifier: BSD-3-Clause

package nethttp

// This file is the Go port of MRI 4.0.5's net/http/responses.rb: the mapping
// from a 3-digit status code to its Net::HTTPResponse subclass name, the
// subclass's category (the per-first-digit Net::HTTPInformation / HTTPSuccess /
// HTTPRedirection / HTTPClientError / HTTPServerError parent), and whether that
// status carries a body (HAS_BODY). It is generated from MRI's own
// CODE_TO_OBJ table so the subclass selected for a parsed response is identical.

// codeEntry describes one row of CODE_TO_OBJ.
type codeEntry struct {
	Code     string
	Class    string // e.g. "HTTPOK"
	Category string // e.g. "HTTPSuccess"
	HasBody  bool
}

// codeTable is the exact, code-sorted CODE_TO_OBJ map MRI ships.
var codeTable = []codeEntry{
	{Code: "100", Class: "HTTPContinue", Category: "HTTPInformation", HasBody: false},
	{Code: "101", Class: "HTTPSwitchProtocol", Category: "HTTPInformation", HasBody: false},
	{Code: "102", Class: "HTTPProcessing", Category: "HTTPInformation", HasBody: false},
	{Code: "103", Class: "HTTPEarlyHints", Category: "HTTPInformation", HasBody: false},
	{Code: "200", Class: "HTTPOK", Category: "HTTPSuccess", HasBody: true},
	{Code: "201", Class: "HTTPCreated", Category: "HTTPSuccess", HasBody: true},
	{Code: "202", Class: "HTTPAccepted", Category: "HTTPSuccess", HasBody: true},
	{Code: "203", Class: "HTTPNonAuthoritativeInformation", Category: "HTTPSuccess", HasBody: true},
	{Code: "204", Class: "HTTPNoContent", Category: "HTTPSuccess", HasBody: false},
	{Code: "205", Class: "HTTPResetContent", Category: "HTTPSuccess", HasBody: false},
	{Code: "206", Class: "HTTPPartialContent", Category: "HTTPSuccess", HasBody: true},
	{Code: "207", Class: "HTTPMultiStatus", Category: "HTTPSuccess", HasBody: true},
	{Code: "208", Class: "HTTPAlreadyReported", Category: "HTTPSuccess", HasBody: true},
	{Code: "226", Class: "HTTPIMUsed", Category: "HTTPSuccess", HasBody: true},
	{Code: "300", Class: "HTTPMultipleChoices", Category: "HTTPRedirection", HasBody: true},
	{Code: "301", Class: "HTTPMovedPermanently", Category: "HTTPRedirection", HasBody: true},
	{Code: "302", Class: "HTTPFound", Category: "HTTPRedirection", HasBody: true},
	{Code: "303", Class: "HTTPSeeOther", Category: "HTTPRedirection", HasBody: true},
	{Code: "304", Class: "HTTPNotModified", Category: "HTTPRedirection", HasBody: false},
	{Code: "305", Class: "HTTPUseProxy", Category: "HTTPRedirection", HasBody: false},
	{Code: "307", Class: "HTTPTemporaryRedirect", Category: "HTTPRedirection", HasBody: true},
	{Code: "308", Class: "HTTPPermanentRedirect", Category: "HTTPRedirection", HasBody: true},
	{Code: "400", Class: "HTTPBadRequest", Category: "HTTPClientError", HasBody: true},
	{Code: "401", Class: "HTTPUnauthorized", Category: "HTTPClientError", HasBody: true},
	{Code: "402", Class: "HTTPPaymentRequired", Category: "HTTPClientError", HasBody: true},
	{Code: "403", Class: "HTTPForbidden", Category: "HTTPClientError", HasBody: true},
	{Code: "404", Class: "HTTPNotFound", Category: "HTTPClientError", HasBody: true},
	{Code: "405", Class: "HTTPMethodNotAllowed", Category: "HTTPClientError", HasBody: true},
	{Code: "406", Class: "HTTPNotAcceptable", Category: "HTTPClientError", HasBody: true},
	{Code: "407", Class: "HTTPProxyAuthenticationRequired", Category: "HTTPClientError", HasBody: true},
	{Code: "408", Class: "HTTPRequestTimeout", Category: "HTTPClientError", HasBody: true},
	{Code: "409", Class: "HTTPConflict", Category: "HTTPClientError", HasBody: true},
	{Code: "410", Class: "HTTPGone", Category: "HTTPClientError", HasBody: true},
	{Code: "411", Class: "HTTPLengthRequired", Category: "HTTPClientError", HasBody: true},
	{Code: "412", Class: "HTTPPreconditionFailed", Category: "HTTPClientError", HasBody: true},
	{Code: "413", Class: "HTTPPayloadTooLarge", Category: "HTTPClientError", HasBody: true},
	{Code: "414", Class: "HTTPURITooLong", Category: "HTTPClientError", HasBody: true},
	{Code: "415", Class: "HTTPUnsupportedMediaType", Category: "HTTPClientError", HasBody: true},
	{Code: "416", Class: "HTTPRangeNotSatisfiable", Category: "HTTPClientError", HasBody: true},
	{Code: "417", Class: "HTTPExpectationFailed", Category: "HTTPClientError", HasBody: true},
	{Code: "421", Class: "HTTPMisdirectedRequest", Category: "HTTPClientError", HasBody: true},
	{Code: "422", Class: "HTTPUnprocessableEntity", Category: "HTTPClientError", HasBody: true},
	{Code: "423", Class: "HTTPLocked", Category: "HTTPClientError", HasBody: true},
	{Code: "424", Class: "HTTPFailedDependency", Category: "HTTPClientError", HasBody: true},
	{Code: "426", Class: "HTTPUpgradeRequired", Category: "HTTPClientError", HasBody: true},
	{Code: "428", Class: "HTTPPreconditionRequired", Category: "HTTPClientError", HasBody: true},
	{Code: "429", Class: "HTTPTooManyRequests", Category: "HTTPClientError", HasBody: true},
	{Code: "431", Class: "HTTPRequestHeaderFieldsTooLarge", Category: "HTTPClientError", HasBody: true},
	{Code: "451", Class: "HTTPUnavailableForLegalReasons", Category: "HTTPClientError", HasBody: true},
	{Code: "500", Class: "HTTPInternalServerError", Category: "HTTPServerError", HasBody: true},
	{Code: "501", Class: "HTTPNotImplemented", Category: "HTTPServerError", HasBody: true},
	{Code: "502", Class: "HTTPBadGateway", Category: "HTTPServerError", HasBody: true},
	{Code: "503", Class: "HTTPServiceUnavailable", Category: "HTTPServerError", HasBody: true},
	{Code: "504", Class: "HTTPGatewayTimeout", Category: "HTTPServerError", HasBody: true},
	{Code: "505", Class: "HTTPVersionNotSupported", Category: "HTTPServerError", HasBody: true},
	{Code: "506", Class: "HTTPVariantAlsoNegotiates", Category: "HTTPServerError", HasBody: true},
	{Code: "507", Class: "HTTPInsufficientStorage", Category: "HTTPServerError", HasBody: true},
	{Code: "508", Class: "HTTPLoopDetected", Category: "HTTPServerError", HasBody: true},
	{Code: "510", Class: "HTTPNotExtended", Category: "HTTPServerError", HasBody: true},
	{Code: "511", Class: "HTTPNetworkAuthenticationRequired", Category: "HTTPServerError", HasBody: true},
}

// codeToEntry indexes codeTable by exact 3-digit code.
var codeToEntry = func() map[string]codeEntry {
	m := make(map[string]codeEntry, len(codeTable))
	for _, e := range codeTable {
		m[e.Code] = e
	}
	return m
}()

// categoryByDigit maps a status' first digit to its category class name and
// that category's default HasBody — the CODE_CLASS_TO_OBJ fallback MRI uses
// when an exact code is unknown.
var categoryByDigit = map[byte]struct {
	Class   string
	HasBody bool
}{
	'1': {"HTTPInformation", false},
	'2': {"HTTPSuccess", true},
	'3': {"HTTPRedirection", true},
	'4': {"HTTPClientError", true},
	'5': {"HTTPServerError", true},
}

// classForCode returns the response subclass name, its category, and HasBody for
// a status code, mirroring Net::HTTPResponse.response_class:
//
//	CODE_TO_OBJ[code] || CODE_CLASS_TO_OBJ[code[0]] || Net::HTTPUnknownResponse
//
// HTTPUnknownResponse has HAS_BODY = true.
func classForCode(code string) (class, category string, hasBody bool) {
	if e, ok := codeToEntry[code]; ok {
		return e.Class, e.Category, e.HasBody
	}
	if len(code) > 0 {
		if c, ok := categoryByDigit[code[0]]; ok {
			return c.Class, c.Class, c.HasBody
		}
	}
	return "HTTPUnknownResponse", "HTTPUnknownResponse", true
}
