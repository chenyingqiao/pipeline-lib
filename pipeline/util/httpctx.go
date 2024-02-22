package util

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/hetiansu5/urlquery"
	"golang.org/x/net/context/ctxhttp"
)

var tracingClient = http.DefaultClient

func Get(ctx context.Context, url string, header map[string]string) (*http.Response, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range header {
		request.Header.Set(key, value)
	}
	return ctxhttp.Do(ctx, tracingClient, request)
}

func GetWithQuery(ctx context.Context, url string, header map[string]string, query interface{}) (*http.Response, error) {
	bytes, err := urlquery.Marshal(query)
	if err != nil {
		return nil, err
	}

	fullPath := url + "?" + string(bytes)
	return Get(ctx, fullPath, header)
}

func PostJson(ctx context.Context, url string, body interface{}, header map[string]string) (*http.Response, error) {
	return DoJson(ctx, "POST", url, body, header)
}

func PutJson(ctx context.Context, url string, body interface{}, header map[string]string) (*http.Response, error) {
	return DoJson(ctx, "PUT", url, body, header)
}

func DeleteJson(ctx context.Context, url string, body interface{}, header map[string]string) (*http.Response, error) {
	return DoJson(ctx, "DELETE", url, body, header)
}

func DoJson(ctx context.Context, method string, url string, body interface{}, header map[string]string) (*http.Response, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(method, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, err
	}
	for key, value := range header {
		request.Header.Set(key, value)
		// 用于Host被框架重写的情况
		if key == "Host" && value != "" {
			request.Host = value
		}
	}
	request.Header.Set("Content-Type", "application/json")
	return ctxhttp.Do(ctx, tracingClient, request)
}

func PostForm(ctx context.Context, url string, data url.Values, header map[string]string) (*http.Response, error) {
	request, err := http.NewRequest("POST", url, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	for key, value := range header {
		request.Header.Set(key, value)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return ctxhttp.Do(ctx, tracingClient, request)
}

func Delete(ctx context.Context, url string, header map[string]string) (*http.Response, error) {
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range header {
		request.Header.Set(key, value)
	}
	return ctxhttp.Do(ctx, tracingClient, request)
}
