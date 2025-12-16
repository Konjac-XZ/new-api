package common

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

// IsDownstreamContextDone reports whether the downstream request context is done
// (typically because the client disconnected or explicitly cancelled).
func IsDownstreamContextDone(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}

// IsClientGoneError reports whether err likely came from writing/reading after the client
// connection has been closed (broken pipe, reset by peer, etc).
//
// This intentionally does NOT treat context.Canceled as "client gone", because internal
// per-attempt cancellations may also surface as context cancellation.
func IsClientGoneError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, http.ErrAbortHandler) {
		return true
	}
	if isUnixClientGoneSyscallError(err) {
		return true
	}

	// Fallback heuristics for common transport errors that don't unwrap cleanly.
	// Keep these conservative: only match well-known client-disconnect phrases.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "connection reset by peer"):
		return true
	case strings.Contains(msg, "use of closed network connection"):
		return true
	}

	return false
}
