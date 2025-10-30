package utils

type HTTPMethod string

const (
	HTTPGet     HTTPMethod = "GET"
	HTTPPost    HTTPMethod = "POST"
	HTTPPut     HTTPMethod = "PUT"
	HTTPPatch   HTTPMethod = "PATCH"
	HTTPDelete  HTTPMethod = "DELETE"
	HTTPOptions HTTPMethod = "OPTIONS"
	HTTPHead    HTTPMethod = "HEAD"
)

const (
	HeaderContentType         = "Content-Type"
	HeaderAuthorization       = "Authorization"
	HeaderRetryAfter          = "Retry-After"
	HeaderXRequestID          = "X-Request-ID"
	HeaderXRateLimitRemaining = "X-RateLimit-Remaining"
	HeaderXRateLimitLimit     = "X-RateLimit-Limit"
	HeaderXRateLimitReset     = "X-RateLimit-Reset"
	ContentTypeJSON           = "application/json"
	ContentTypeFormURLEncoded = "application/x-www-form-urlencoded"
	ContentTypeOctetStream    = "application/octet-stream"
)
