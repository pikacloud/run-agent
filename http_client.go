package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{
	Timeout: time.Second * 10,
}

func (client *Client) makeRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := client.BaseURL + fmt.Sprintf("%s/%s", apiVersion, path)
	req, err := http.NewRequest(method, url, body)
	req.Header.Add("Authorization", fmt.Sprintf("Token: %s", client.APIToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	if err != nil {
		return nil, err
	}
	return req, nil
}

// Client manages communication with Pikacloud API.
type Client struct {
	// API Token for authenticating
	APIToken string
	// HTTP client used to communicate with the Pikacloud API.
	HTTPClient *http.Client
	// Base URL for API requests.
	BaseURL string
}

// NewClient agent
func NewClient(apiToken string) *Client {
	return &Client{APIToken: apiToken, HTTPClient: httpClient, BaseURL: defaultBaseURL}
}

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
	if code := resp.StatusCode; 200 == code && code <= 299 {
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

func (client *Client) sendRequest(method, path string, body io.Reader) (string, int, error) {
	req, err := client.makeRequest(method, path, body)
	if err != nil {
		return "", 0, err
	}

	resp, err := client.HTTPClient.Do(req)
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
func (client *Client) Get(path string, val interface{}) error {
	body, _, err := client.sendRequest("GET", path, nil)
	if err != nil {
		return err
	}

	if err = json.Unmarshal([]byte(body), &val); err != nil {
		return err
	}

	return nil
}

// PostOrPutOrDelete method
func (client *Client) PostOrPutOrDelete(method, path string, payload, val interface{}) (int, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	body, status, err := client.sendRequest(method, path, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return 0, err
	}

	if err = json.Unmarshal([]byte(body), &val); err != nil {
		return 0, err
	}

	return status, nil
}

// Put meta method
func (client *Client) Put(path string, payload, val interface{}) (int, error) {
	return client.PostOrPutOrDelete("PUT", path, payload, val)
}

// Post meta method
func (client *Client) Post(path string, payload, val interface{}) (int, error) {
	return client.PostOrPutOrDelete("POST", path, payload, val)
}

// Delete meta method
func (client *Client) Delete(path string, payload, val interface{}) (int, error) {
	return client.PostOrPutOrDelete("DELETE", path, payload, val)
}
