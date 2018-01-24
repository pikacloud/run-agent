package weave

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// A Response represents an API response.
type Response struct {
	// HTTP response
	HTTPResponse *http.Response
}

// An ErrorResponse represents an API response that generated an error.
type ErrorResponse struct {
	Response
	// human-readable message
	Detail string `json:"detail"`
}

// Error implements the error interface.
func (r *ErrorResponse) Error() string {
	return fmt.Sprintf("%v %v: %v %v",
		r.HTTPResponse.Request.Method, r.HTTPResponse.Request.URL,
		r.HTTPResponse.StatusCode, r.Detail)
}

// CheckResponse checks the API response for errors, and returns them if present.
func CheckResponse(resp *http.Response) error {
	if code := resp.StatusCode; 200 <= code && code <= 299 {
		return nil
	}
	errorResponse := &ErrorResponse{}
	errorResponse.HTTPResponse = resp
	err := json.NewDecoder(resp.Body).Decode(errorResponse)
	if err != nil {
		return err
	}

	return errorResponse
}

func (w *Weave) makeRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := w.BaseURL + fmt.Sprintf("%s", path)
	req, err := http.NewRequest(method, url, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (w *Weave) sendRequest(method, path string, body io.Reader) (string, int, error) {
	req, err := w.makeRequest(method, path, body)
	if err != nil {
		return "", 0, err
	}

	resp, err := w.HTTPClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	err = CheckResponse(resp)
	if err != nil {
		return "", resp.StatusCode, err
	}

	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	return string(responseBytes), resp.StatusCode, nil
}

// Get method
func (w *Weave) Get(path string, val interface{}) error {
	body, _, err := w.sendRequest("GET", path, nil)
	if err != nil {
		return err
	}

	if err = json.Unmarshal([]byte(body), &val); err != nil {
		return err
	}
	return nil
}
