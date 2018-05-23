package proxy

import (
	"net/http"
	"strings"
	"time"

	"github.com/hellofresh/janus/pkg/proxy/balancer"

	"github.com/hellofresh/janus/pkg/router"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	methodAll = "ALL"
)

// Register handles the register of proxies into the chosen router.
// It also handles the conversion from a proxy to an http.HandlerFunc
type Register struct {
	router                 router.Router
	idleConnectionsPerHost int
	closeIdleConnsPeriod   time.Duration
	flushInterval          time.Duration
}

// NewRegister creates a new instance of Register
func NewRegister(opts ...RegisterOption) *Register {
	r := Register{}

	for _, opt := range opts {
		opt(&r)
	}

	return &r
}

// UpdateRouter updates the reference to the router. This is useful to reload the mux
func (p *Register) UpdateRouter(router router.Router) {
	p.router = router
}

// Add register a new route
func (p *Register) Add(definition *Definition) error {
	log.WithField("balancing_alg", definition.Upstreams.Balancing).Debug("Using a load balancing algorithm")
	balancer, err := balancer.New(definition.Upstreams.Balancing)
	if err != nil {
		msg := "Could not create a balancer"
		log.WithError(err).Error(msg)
		return errors.Wrap(err, msg)
	}

	handler := NewBalancedReverseProxy(definition, balancer)
	handler.Transport = NewTransportWithParams(Params{
		InsecureSkipVerify:     definition.InsecureSkipVerify,
		FlushInterval:          p.flushInterval,
		CloseIdleConnsPeriod:   p.closeIdleConnsPeriod,
		IdleConnectionsPerHost: p.idleConnectionsPerHost,
	})

	matcher := router.NewListenPathMatcher()
	if matcher.Match(definition.ListenPath) {
		p.doRegister(matcher.Extract(definition.ListenPath), handler.ServeHTTP, definition.Methods, definition.middleware)
	}

	p.doRegister(definition.ListenPath, handler.ServeHTTP, definition.Methods, definition.middleware)
	return nil
}

func (p *Register) doRegister(listenPath string, handler http.HandlerFunc, methods []string, handlers []router.Constructor) {
	log.WithFields(log.Fields{
		"listen_path": listenPath,
	}).Debug("Registering a route")

	if strings.Index(listenPath, "/") != 0 {
		log.WithField("listen_path", listenPath).
			Error("Route listen path must begin with '/'. Skipping invalid route.")
	} else {
		for _, method := range methods {
			if strings.ToUpper(method) == methodAll {
				p.router.Any(listenPath, handler, handlers...)
			} else {
				p.router.Handle(strings.ToUpper(method), listenPath, handler, handlers...)
			}
		}
	}
}
