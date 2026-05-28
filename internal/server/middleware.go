package server

import (
	"context"
	"crypto/subtle"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// requireAPIKey returns a Hertz middleware that compares the X-API-Key header
// (constant-time) against the configured key. If expected is empty the
// middleware is a no-op, preserving the current public/local-dev behaviour.
func requireAPIKey(expected string) app.HandlerFunc {
	if expected == "" {
		return func(c context.Context, ctx *app.RequestContext) { ctx.Next(c) }
	}
	expectedBytes := []byte(expected)
	return func(c context.Context, ctx *app.RequestContext) {
		got := ctx.Request.Header.Get("X-API-Key")
		if got == "" {
			got = string(ctx.Request.Header.Get("Authorization"))
		}
		if subtle.ConstantTimeCompare([]byte(got), expectedBytes) != 1 {
			ctx.AbortWithStatusJSON(consts.StatusUnauthorized, utils.H{
				"error": "missing or invalid X-API-Key",
			})
			return
		}
		ctx.Next(c)
	}
}
