// Package subsonic implements an API client library for Subsonic-compatible music streaming servers.
//
// This project handles communication with a remote *sonic server, but does not handle playback of media. The library user should be prepared to do something with the stream of audio in bytes, like decoding and playing that audio on a sound card.
// The list of API endpoints implemented is available on the project's github page.
package subsonic

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"path"
)

const (
	supportedApiVersion = "1.8.0"
	libraryVersion      = "0.0.4"
)

type Client struct {
	Client     *http.Client
	BaseUrl    string
	User       string
	ClientName string
	salt       string
	token      string
}

func generateSalt() string {
	var corpus = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	// length is minimum 6, but let's use ten to start
	b := make([]rune, 10)
	for i := range b {
		b[i] = corpus[rand.Intn(len(corpus))]
	}
	return string(b)
}

// Authenticate authenticates the current user with a provided password. The password is salted before transmission and requires Subsonic > 1.13.0.
func (s *Client) Authenticate(password string) error {
	salt := generateSalt()
	h := md5.New()
	_, err := io.WriteString(h, password)
	if err != nil {
		return err
	}
	_, err = io.WriteString(h, salt)
	if err != nil {
		return err
	}
	s.salt = salt
	s.token = fmt.Sprintf("%x", h.Sum(nil))
	// Test authentication
	if !s.Ping() {
		return errors.New("Authentication failed")
	}
	return nil
}

// Request performs a HTTP request against the Subsonic server as the current user.
func (s *Client) Request(method string, endpoint string, params map[string]string) (*http.Response, error) {
	baseUrl, err := url.Parse(s.BaseUrl)
	if err != nil {
		return nil, err
	}
	baseUrl.Path = path.Join(baseUrl.Path, "/rest/", endpoint)
	req, err := http.NewRequest(method, baseUrl.String(), nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("f", "json")
	q.Add("v", supportedApiVersion)
	q.Add("c", s.ClientName)
	q.Add("u", s.User)
	q.Add("t", s.token)
	q.Add("s", s.salt)
	for key, val := range params {
		q.Add(key, val)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Get is a convenience interface to issue a GET request and parse the response body (99% of Subsonic API calls)
func (s *Client) Get(endpoint string, params map[string]string) (*subsonicResponse, error) {
	response, err := s.Request("GET", endpoint, params)
	if err != nil {
		return nil, err
	}
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	//log.Printf("%s: %s\n", endpoint, contents)
	parsed := apiResponse{}
	err = json.Unmarshal(responseBody, &parsed)
	if err != nil {
		return nil, err
	}
	resp := parsed.Response
	if resp.Error != nil {
		return nil, fmt.Errorf("Error #%d: %s\n", resp.Error.Code, resp.Error.Message)
	}
	return resp, nil
}

// Ping is used to test connectivity with the server. It returns true if the server is up.
func (s *Client) Ping() bool {
	_, err := s.Request("GET", "ping", nil)
	if err != nil {
		log.Println(err)
		return false
	}
	return true
}

// GetLicense retrieves details about the software license. Subsonic requires a license after a 30-day trial, compatible applications have a perpetually valid license.
func (s *Client) GetLicense() (*License, error) {
	resp, err := s.Get("getLicense", nil)
	if err != nil {
		return nil, err
	}
	return resp.License, nil
}