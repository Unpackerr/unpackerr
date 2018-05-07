package starr

import (
	"crypto/tls"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Config is the data needed to poll Radarr or Sonarr.
type Config struct {
	APIKey   string `json:"api_key" toml:"api_key" xml:"api_key" yaml:"api_key"`
	URL      string `json:"url" toml:"url" xml:"url" yaml:"url"`
	Tag      string `json:"tag" toml:"tag" xml:"tag" yaml:"tag"` // not used
	HTTPPass string `json:"http_pass" toml:"http_pass" xml:"http_pass" yaml:"http_pass"`
	HTTPUser string `json:"http_user" toml:"http_user" xml:"http_user" yaml:"http_user"`
}

// Req makes a http request, with some additions.
// path = "/query", params = "sort_by=timeleft&order=asc" (as url.Values)
func (c *Config) Req(path string, params url.Values) ([]byte, error) {
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	path = c.fixPath(path)

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, errors.Wrap(err, "http.NewRequest(path)")
	}

	params.Add("apikey", c.APIKey)
	req.URL.RawQuery = params.Encode()
	// This app allows http auth, in addition to api key (nginx proxy).
	if c.HTTPUser += ":" + c.HTTPPass; c.HTTPUser != ":" {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(c.HTTPUser))
		req.Header.Add("Authorization", auth)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "d.Do(req)")
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			// boo.
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed: %v (status: %v/%v)",
			path, resp.StatusCode, resp.Status)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "ioutil.ReadAll")
	}
	//log.Println(string(body))
	return body, nil
}

func (c *Config) fixPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasSuffix(c.URL, "/") {
		path = c.URL + "api" + path
	} else {
		path = c.URL + "/api" + path
	}
	return path
}
