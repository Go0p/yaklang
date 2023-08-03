package yakdns

import (
	"context"
	"crypto/tls"
	"github.com/ReneKroon/ttlcache"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

var ipv6DNSCache = ttlcache.NewCache()
var ipv4DNSCache = ttlcache.NewCache()

func reliableLookupHost(host string, opt ...DNSOption) error {
	var config = NewDefaultReliableDNSConfig()
	for _, o := range opt {
		o(config)
	}

	if config.Hosts != nil && len(config.Hosts) > 0 {
		result, ok := config.Hosts[host]
		if ok && result != "" {
			config.call("", host, result, "hosts", "hosts")
			return nil
		}
	}

	// handle hosts
	result, ok := GetHost(host)
	if ok && result != "" {
		config.call("", host, result, "hosts", "hosts")
		return nil
	}

	if !config.NoCache {
		// ttlcache v4 > v6
		cachedResult, ok := ipv4DNSCache.Get(host)
		if ok {
			result, ok := cachedResult.(string)
			if ok {
				config.call("", host, result, "cache", "cache")
				return nil
			}
		}
		cachedResult, ok = ipv6DNSCache.Get(host)
		if ok {
			result, ok := cachedResult.(string)
			if ok {
				config.call("", host, result, "cache", "cache")
				return nil
			}
		}
	}

	// handle system resolver
	if !config.DisableSystemResolver {
		nativeLookupHost(host, config)
		if config.count > 0 {
			return nil
		}
	}

	startDoH := func() {
		if len(config.SpecificDoH) > 0 {
			swg := utils.NewSizedWaitGroup(5)
			dohCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			for _, doh := range config.SpecificDoH {
				err := swg.AddWithContext(dohCtx)
				if err != nil {
					break
				}
				dohUrl := doh
				go func() {
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("doh server %s panic: %v", dohUrl, err)
							utils.PrintCurrentGoroutineRuntimeStack()
						}
						swg.Done()
					}()
					err := dohRequest(host, dohUrl, config)
					if err != nil {
						log.Debugf("doh request failed: %s", err)
					}
				}()
			}
			swg.Wait()
		}
	}

	var dohExecuted bool
	if config.PreferDoH {
		log.Infof("start(prefer) to use doh to lookup %s", host)
		startDoH()
		dohExecuted = true
		if config.count > 0 {
			return nil
		}
	}

	// handle specific dns servers
	if len(config.SpecificDNSServers) > 0 {
		swg := utils.NewSizedWaitGroup(5)
		for _, customServer := range config.SpecificDNSServers {
			customServer := customServer
			swg.Add()
			go func() {
				defer swg.Done()
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("dns server %s panic: %v", customServer, err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				err := _exec(customServer, host, config)
				if err != nil {
					log.Debugf("dns server %s lookup failed: %v", customServer, err)
				}
			}()
		}
		swg.Wait()
	} else {
		log.Info("no user custom specific dns servers found")
	}

	if config.FallbackDoH && config.count <= 0 && !dohExecuted {
		log.Infof("start(fallback) to use doh to lookup %s", host)
		startDoH()
	}

	return nil
}

func LookupAll(host string, opt ...DNSOption) []string {
	var results []string
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		if ip == "" {
			return
		}
		results = append(results, ip)
	}))
	err := reliableLookupHost(host, opt...)
	if err != nil {
		log.Errorf("reliable lookup host %s failed: %v", host, err)
	}
	return results
}

func LookupCallback(host string, h func(dnsType, domain, ip, fromServer string, method string), opt ...DNSOption) error {
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		h(dnsType, domain, ip, fromServer, method)
	}))
	return reliableLookupHost(host, opt...)
}

func LookupFirst(host string, opt ...DNSOption) string {
	start := time.Now()
	defer func() {
		log.Debugf("lookup first %s cost %s", host, time.Since(start))
	}()

	var firstResult string
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	opt = append(opt, WithDNSCallback(func(dnsType, domain, ip, fromServer, method string) {
		if ip == "" {
			return
		}

		if firstResult == "" {
			firstResult = ip
			cancel()
		}
	}), WithDNSContext(ctx))
	go func() {
		defer cancel()
		err := reliableLookupHost(host, opt...)
		if err != nil {
			log.Errorf("reliable lookup host %s failed: %v", host, err)
		}
	}()
	select {
	case <-ctx.Done():
	}
	return firstResult
}

func NewDialContextFunc(timeout time.Duration, opts ...DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		host, port, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			return nil, utils.Errorf("cannot parse %v as host:port, reason: %v", addr, err)
		}

		if utils.IsIPv4(host) || utils.IsIPv6(host) {
			return net.DialTimeout(network, utils.HostPort(host, port), timeout)
		}

		newHost := LookupFirst(host, opts...)
		if newHost == "" {
			return nil, utils.Errorf("cannot resolve %v", addr)
		}
		return net.DialTimeout(network, utils.HostPort(newHost, port), timeout)
	}
}

func NewDialGMTLSContextFunc(preferGMTLS bool, onlyGMTLS bool, timeout time.Duration, opts ...DNSOption) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	origin := NewDialContextFunc(timeout, opts...)
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		plainConn, err := origin(ctx, network, addr)
		if err != nil {
			return nil, utils.Errorf("gmtls dialer with TCP dial: %v", err)
		}
		targetHost, _, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			targetHost = addr
		}

		handleTLS := func() (net.Conn, error) {
			conn := tls.Client(plainConn, &tls.Config{
				ServerName:         targetHost,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true,
			})
			handshakeCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			err = conn.HandshakeContext(handshakeCtx)
			if err != nil {
				return nil, utils.Errorf("tls handshake error: %v", err)
			}
			return conn, nil
		}

		handleGMTLS := func() (net.Conn, error) {
			conn := gmtls.Client(plainConn, &gmtls.Config{
				GMSupport: &gmtls.GMSupport{
					WorkMode: "GMSSLOnly",
				},
				ServerName:         targetHost,
				InsecureSkipVerify: true,
			})
			handshakeCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			err = conn.HandshakeContext(handshakeCtx)
			if err != nil {
				return nil, utils.Errorf("gmtls handshake error: %v", err)
			}
			return conn, nil
		}

		if onlyGMTLS {
			return handleGMTLS()
		}

		if preferGMTLS {
			conn, err := handleGMTLS()
			if err != nil {
				return handleTLS()
			}
			return conn, nil
		} else {
			conn, err := handleTLS()
			if err != nil {
				return handleGMTLS()
			}
			return conn, nil
		}
	}
}
