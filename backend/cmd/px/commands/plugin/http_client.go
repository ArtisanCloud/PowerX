package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type apiClient struct {
	baseURL string
	token   string
	httpCli *http.Client
}

func newAPIClient(baseURL, token string) *apiClient {
	return &apiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		httpCli: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *apiClient) post(path string, payload any, dest any) error {
	url := c.baseURL + path
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		message := readError(resp.Body)
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("%s", message)
	}

	if dest == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, dest)
}

func readError(r io.Reader) string {
	data, _ := io.ReadAll(r)
	if len(data) == 0 {
		return ""
	}
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err == nil {
		if msg, ok := envelope["error"].(string); ok {
			return msg
		}
	}
	return strings.TrimSpace(string(data))
}
