package crep

import (
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
	"net/http/httptrace"
)

type httpTraceTransport struct {
	*http.Transport
	cache *ttlcache.Cache
}

func (t *httpTraceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			addr := info.Conn.RemoteAddr()
			host, port, _ := utils.ParseStringToHostPort(fmt.Sprintf("%v://%v", req.URL.Scheme, req.Host))
			key := utils.HostPort(host, port)
			if key == "" {
				host = req.Host
			}
			//log.Infof("remote addr: %v(%v)", addr, key)
			if t.cache != nil {
				t.cache.Set(key, addr)
			}
		},
	}))
	*req = *req.WithContext(context.WithValue(req.Context(), "request-id", fmt.Sprintf("%p", req)))

	if connected := httpctx.GetContextStringInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_ConnectedTo); connected != "" {
		req.Host = connected
		if req.URL.Host != "" {
			req.URL.Host = connected
		}
	}
	rsp, err := t.Transport.RoundTrip(req)
	return rsp, err
}
