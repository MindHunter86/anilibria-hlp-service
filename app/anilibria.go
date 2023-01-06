package app

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
	"golang.org/x/net/http2"
)

var (
	ccx *cli.Context
	log *zerolog.Logger
)

var (
	errApiAbnormalResponse    = errors.New("there is some problems with anilibria servers communication")
)

type ApiClient struct {
	http *http.Client

	apiBaseUrl *url.URL
}

const defaultApiMethodFilter = "id,code,names,updated,last_change,player"

type ApiRequestMethod string
const (
	apiMethodGetTitle ApiRequestMethod = "/getTitle"
)

type (
	apiError struct {
		Error *apiErrorDetails
	}
	apiErrorDetails struct {
		Code    int
		Message string
	}
	apiResponse struct {
		payload []byte
		err     error
	}
)

func (m *apiResponse) Err() error {
	return m.err
}

func (m *apiResponse) Error() string {
	return m.err.Error()
}

func NewApiClient(c *cli.Context, l *zerolog.Logger) (*ApiClient, error) {
	ccx, log = c, l

	defaultTransportDialContext := func(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
		return dialer.DialContext
	}

	http1Transport := &http.Transport{
		DialContext: defaultTransportDialContext(&net.Dialer{
			Timeout:   ccx.Duration("http-tcp-timeout"),
			KeepAlive: ccx.Duration("http-keepalive-timeout"),
		}),

		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: ccx.Bool("http-client-insecure"),
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS12,
		},
		TLSHandshakeTimeout: ccx.Duration("http-tls-handshake-timeout"),

		MaxIdleConns:    ccx.Int("http-max-idle-conns"),
		IdleConnTimeout: ccx.Duration("http-idle-timeout"),

		DisableCompression: false,
		DisableKeepAlives:  false,
		ForceAttemptHTTP2:  true,
	}

	var httpTransport http.RoundTripper = http1Transport
	http2Transport, err := http2.ConfigureTransports(http1Transport)
	if err != nil {
		httpTransport = http2Transport
		log.Warn().Err(err).Msg("could not upgrade http transport to v2 because of internal error")
	}

	var apiClient = &ApiClient{
		http: &http.Client{
			Timeout:   time.Duration(ccx.Int("http-client-timeout")) * time.Second,
			Transport: httpTransport,
		},
	}

	return apiClient, apiClient.getApiBaseUrl()
}

func (m *ApiClient) getApiBaseUrl() (e error) {
	m.apiBaseUrl, e = url.Parse(ccx.String("anilibria-api-baseurl"))
	return e
}
