package server

import (
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
)

// hertzHeaderToHTTP copies inbound Hertz request headers into a net/http
// Header so the standard OTel propagation.HeaderCarrier can extract from it.
// Only used in the read direction at request entry; cheap one-shot allocation.
func hertzHeaderToHTTP(ctx *app.RequestContext) http.Header {
	out := http.Header{}
	ctx.Request.Header.VisitAll(func(k, v []byte) {
		out.Add(string(k), string(v))
	})
	return out
}
