package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/callmemhz/milo/pkg/api"
)

// Client wraps an HTTP client with a fixed endpoint and bearer token.
type Client struct {
	Endpoint string
	Token    string
	HTTP     *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) do(method, path string, body any, out any) error {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.Endpoint+path, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var apiErr api.Error
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Code != "" {
			return &apiErr
		}
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Get performs a GET request and decodes the JSON response into out.
func (c *Client) Get(path string, out any) error { return c.do("GET", path, nil, out) }

// Post performs a POST request with a JSON body and decodes the response.
func (c *Client) Post(path string, body, out any) error { return c.do("POST", path, body, out) }

// Put performs a PUT request with a JSON body and decodes the response.
func (c *Client) Put(path string, body, out any) error { return c.do("PUT", path, body, out) }

// Patch performs a PATCH request with a JSON body and decodes the response.
func (c *Client) Patch(path string, body, out any) error { return c.do("PATCH", path, body, out) }

// Delete performs a DELETE request (no response body expected).
func (c *Client) Delete(path string) error { return c.do("DELETE", path, nil, nil) }

// Stream issues a GET and returns the response body for streaming (e.g. logs).
// The caller must Close the returned ReadCloser.
func (c *Client) Stream(path string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", c.Endpoint+path, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return resp.Body, nil
}
