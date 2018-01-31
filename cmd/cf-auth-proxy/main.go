package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"crypto/tls"
	"crypto/x509"

	"code.cloudfoundry.org/go-envstruct"
	"code.cloudfoundry.org/log-cache"
	"code.cloudfoundry.org/log-cache/internal/auth"
	logtls "code.cloudfoundry.org/log-cache/internal/tls"
)

func main() {
	log.Print("Starting Log Cache CF Auth Reverse Proxy...")
	defer log.Print("Closing Log Cache CF Auth Reverse Proxy.")

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %s", err)
	}
	envstruct.WriteReport(cfg)

	uaaClient := auth.NewUAAClient(
		cfg.UAA.Addr,
		cfg.UAA.ClientID,
		cfg.UAA.ClientSecret,
		buildUAAClient(cfg),
	)

	capiClient := auth.NewCAPIClient(cfg.CAPI.Addr, buildCAPIClient(cfg))

	middlewareProvider := auth.NewCFAuthMiddlewareProvider(
		uaaClient,
		capiClient,
	)

	proxy := logcache.NewCFAuthProxy(
		cfg.LogCacheGatewayAddr,
		cfg.Addr,
		logcache.WithAuthMiddleware(middlewareProvider.Middleware),
		logcache.WithCFAuthProxyBlock(),
	)
	proxy.Start()
}

func buildUAAClient(cfg *Config) *http.Client {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.SkipCertVerify,
		MinVersion:         tls.VersionTLS12,
	}

	tlsConfig.RootCAs = loadUaaCA(cfg.UAA.CAPath)

	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfig,
		DisableKeepAlives:   true,
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}
}

func buildCAPIClient(cfg *Config) *http.Client {
	tlsConfig, err := logtls.NewTLSConfig(
		cfg.CAPI.CAPath,
		cfg.CAPI.CertPath,
		cfg.CAPI.KeyPath,
		cfg.CAPI.CommonName,
	)
	if err != nil {
		log.Fatalf("unable to create CC HTTP Client: %s", err)
	}

	tlsConfig.InsecureSkipVerify = cfg.SkipCertVerify
	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfig,
		DisableKeepAlives:   true,
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}
}

func loadUaaCA(uaaCertPath string) *x509.CertPool {
	caCert, err := ioutil.ReadFile(uaaCertPath)
	if err != nil {
		log.Fatalf("failed to read UAA CA certificate: %s", err)
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(caCert)
	if !ok {
		log.Fatal("failed to parse UAA CA certificate.")
	}

	return certPool
}
