package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"pipedrive_api_service/internal/upstream"
	"pipedrive_api_service/internal/utils"
)

type PipedriveClient struct {
	baseURL   string
	token     string
	http      *http.Client
	retryQ    *RetryQueue
	userAgent string
}

func NewPipedriveClient() *PipedriveClient {
	base := os.Getenv("PIPEDRIVE_BASE_URL")
	tok := os.Getenv("PIPEDRIVE_API_TOKEN")
	return &PipedriveClient{
		baseURL: base,
		token:   tok,
		http: &http.Client{
			Timeout: 40 * time.Second,
		},
		retryQ:    NewRetryQueue(64, &http.Client{Timeout: 20 * time.Second}),
		userAgent: "pipedrive_api_service/1.0",
	}
}

func (c *PipedriveClient) BaseURL() string {
	return c.baseURL
}

func (c *PipedriveClient) Do(ctx context.Context, method utils.HTTPMethod, path string, q url.Values) (*http.Response, []byte, *utils.RateLimitInfo, error) {
	return c.DoWithBody(ctx, method, path, q, nil)
}

// DoWithBody delegates to global broker when available.
func (c *PipedriveClient) DoWithBody(ctx context.Context, method utils.HTTPMethod, path string, q url.Values, bodyReader io.Reader) (*http.Response, []byte, *utils.RateLimitInfo, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("api_token", c.token)

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid base url: %w", err)
	}
	u.Path = u.Path + path
	u.RawQuery = q.Encode()

	var bodyBytes []byte
	if bodyReader != nil {
		bodyBytes, _ = io.ReadAll(bodyReader)
	}

	headers := map[string]string{
		"User-Agent":            c.userAgent,
		utils.HeaderContentType: utils.ContentTypeJSON,
	}

	broker := upstream.GlobalBroker()
	if broker == nil {
		// fallback to direct HTTP
		var reader io.Reader
		if len(bodyBytes) > 0 {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, string(method), u.String(), reader)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("new request: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("http do: %w", err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		rate := utils.ExtractRateLimitFromHeaders(resp.Header)
		respCopy := &http.Response{
			Status:        resp.Status,
			StatusCode:    resp.StatusCode,
			Header:        resp.Header.Clone(),
			Body:          io.NopCloser(bytes.NewReader(b)),
			ContentLength: int64(len(b)),
			Request:       resp.Request,
		}
		return respCopy, b, rate, nil
	}

	resp, b, rate, err := broker.Execute(ctx, string(method), u.String(), headers, bodyBytes, 3)
	return resp, b, rate, err
}
