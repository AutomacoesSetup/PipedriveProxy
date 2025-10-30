package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

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

	req, err := http.NewRequestWithContext(ctx, string(method), u.String(), bodyReader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("new request: %w", err)
	}

	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set(utils.HeaderContentType, utils.ContentTypeJSON)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("http do: %w", err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	rate := utils.ExtractRateLimitFromHeaders(resp.Header)

	if resp.StatusCode == http.StatusTooManyRequests {
		return resp, body, rate, fmt.Errorf("rate limit exceeded (429)")
	}

	return resp, body, rate, nil
}
