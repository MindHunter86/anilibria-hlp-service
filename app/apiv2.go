package app

import (
	"encoding/json"
	"fmt"
	"io"
	dlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func (m *apiResponse) parseApiResponse(schema interface{}) (e error) {
	if m.Err() != nil {
		return m.Err()
	}

	return json.Unmarshal(m.payload, &schema)
}

func (*ApiClient) debugHttpHandshake(data interface{}, withBody ...bool) {
	if !ccx.Bool("http-debug") {
		return
	}

	var body bool
	if len(withBody) != 0 {
		body = withBody[0]
	}

	var dump []byte
	var err error

	switch v := data.(type) {
	case *http.Request:
		dump, err = httputil.DumpRequestOut(data.(*http.Request), body)
	case *http.Response:
		dump, _ = httputil.DumpResponse(data.(*http.Response), body)
	default:
		log.Error().Msgf("there is an internal application error; undefined type - %T", v)
	}

	if err != nil {
		log.Warn().Err(err).Msg("got an error in http debug proccess")
	}

	dlog.Println(string(dump))
}

func (*ApiClient) setApiRequestHeaders(req *http.Request) *http.Request {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:99.0) Gecko/20100101 Firefox/99.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en,ru;q=0.5")
	// req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	// req.Header.Set("Connection", "keep-alive") // !!
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Sec-GPC", "1")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")

	return req
}

func (m *ApiClient) getApiResponse(hmethod string, amethod ApiRequestMethod, params ...interface{}) (arsp *apiResponse) {
	arsp = &apiResponse{}

	rrl := *m.apiBaseUrl
	rrl.Path = rrl.Path + string(amethod)

	rgs := &url.Values{}
	rgs.Add("filter", defaultApiMethodFilter)

	// panic avoid
	var body io.Reader
	params = append(params, nil)

	for _, param := range params {
		switch param := param.(type) {
		case []string:
			rgs.Add(param[0], param[1])
		case io.Reader:
			body = param
		}
	}

	rrl.RawQuery = rgs.Encode()

	fmt.Println(rrl.RawQuery)

	var req *http.Request
	if req, arsp.err = http.NewRequest(hmethod, rrl.String(), body); arsp.Err() != nil {
		return
	}
	m.setApiRequestHeaders(req)

	var rsp *http.Response
	if rsp, arsp.err = m.http.Do(req); arsp.Err() != nil {
		return
	}
	defer func() {
		if e := rsp.Body.Close(); e != nil {
			log.Warn().Err(e)
		}
	}()

	m.debugHttpHandshake(req)
	m.debugHttpHandshake(rsp)

	if arsp.payload, arsp.err = io.ReadAll(rsp.Body); arsp.Err() != nil {
		return
	}

	switch rsp.StatusCode {
	case http.StatusOK:
		log.Debug().Str("api_method", string(amethod)).Msg("api reqiest has been completed with response 200 OK")
	default:
		log.Warn().Str("api_method", string(amethod)).Int("api_response_code", rsp.StatusCode).
			Msg("abnormal api response; trying to parse api error object...")

		var aerr *apiError
		if err := arsp.parseApiResponse(&aerr); err != nil {
			log.Error().Err(arsp.Err()).Msg("got an error in parsing reponse error object")
			arsp.err = errApiAbnormalResponse
			return
		}

		log.Warn().Int("error_code", aerr.Error.Code).Str("error_desc", aerr.Error.Message).
			Msg("api error object has been parse")
		arsp.err = errApiAbnormalResponse
		return
	}

	return
}
