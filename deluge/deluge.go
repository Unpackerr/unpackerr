package deluge

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Deluge is what you get for providing a password.
type Deluge struct {
	*http.Client
	url  string
	auth string
	id   int
}

// New creates a http.Client with authenticated cookies.
// Used to make additional, authenticated requests to the APIs.
func New(config Config) (*Deluge, error) {
	// The cookie jar is used to auth Deluge.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errors.Wrap(err, "cookiejar.New(nil)")
	}
	if !strings.HasSuffix(config.URL, "/") {
		config.URL += "/"
	}
	config.URL += "json"

	// This app allows http auth, in addition to deluge web password.
	if config.HTTPUser += ":" + config.HTTPPass; config.HTTPUser != ":" {
		config.HTTPUser = "Basic " + base64.StdEncoding.EncodeToString([]byte(config.HTTPUser))
	} else {
		config.HTTPUser = ""
	}

	deluge := &Deluge{&http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		Jar:       jar,
		Timeout:   10 * time.Second,
	}, config.URL, config.HTTPUser, 0}

	// This []string{config.Password} line is how you send auth creds. It's weird.
	if req, err := deluge.DelReq(AuthLogin, []string{config.Password}); err != nil {
		return nil, errors.Wrap(err, "DelReq(LoginPath, json)")
	} else if resp, err := deluge.Do(req); err != nil {
		return nil, errors.Wrap(err, "d.Do(req)")
	} else if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("authentication failed: %v[%v] (status: %v/%v)",
			config.URL, AuthLogin, resp.StatusCode, resp.Status)
	}
	return deluge, nil
}

// DelReq is a small helper function that adds headers and marshals the json.
func (d Deluge) DelReq(method string, params interface{}) (req *http.Request, err error) {
	d.id++
	paramMap := map[string]interface{}{"method": method, "id": d.id, "params": params}
	if data, errr := json.Marshal(paramMap); errr != nil {
		return req, errors.Wrap(errr, "json.Marshal(params)")
	} else if req, err = http.NewRequest("POST", d.url, bytes.NewBuffer(data)); err == nil {
		if d.auth != "" {
			// In case Deluge is also behind HTTP auth.
			req.Header.Add("Authorization", d.auth)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Add("Accept", "application/json")
	}
	return
}

// GetXfers gets all the Transfers from Deluge.
func (d Deluge) GetXfers() (map[string]*XferStatus, error) {
	xfers := make(map[string]*XferStatus)
	if response, err := d.Get(GetAllTorrents, []string{"", ""}); err != nil {
		return xfers, errors.Wrap(err, "get(GetAllTorrents)")
	} else if err := json.Unmarshal(response.Result, &xfers); err != nil {
		return xfers, errors.Wrap(err, "json.Unmarshal(xfers)")
	}
	return xfers, nil
}

// Get a response from Deluge
func (d Deluge) Get(method string, params interface{}) (*Response, error) {
	response := new(Response)
	req, err := d.DelReq(method, params)
	if err != nil {
		return response, errors.Wrap(err, "d.DelReq")
	}
	resp, err := d.Do(req)
	if err != nil {
		return response, errors.Wrap(err, "d.Do")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// boo.
		}
	}()
	if body, err := ioutil.ReadAll(resp.Body); err != nil {
		return response, errors.Wrap(err, "ioutil.ReadAll")
	} else if err = json.Unmarshal(body, &response); err != nil {
		return response, errors.Wrap(err, "json.Unmarshal(response)")
	} else if response.Error.Code != 0 {
		return response, errors.New("deluge error: " + response.Error.Message)
	}
	return response, nil
}
