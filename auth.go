package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type UAATransport struct {
	clientID     string
	clientSecret string
}

func (t *UAATransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", t.clientID, t.clientSecret)))))
	return http.DefaultTransport.RoundTrip(r)
}

type uaaTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type Auth struct {
	UAAAPI          string
	UAAClientID     string
	UAAClientSecret string

	tokens uaaTokenResponse
}

func (a *Auth) Authenticate() error {
	httpClient := &http.Client{
		Transport: &UAATransport{clientID: a.UAAClientID, clientSecret: a.UAAClientSecret},
	}

	requestBody := fmt.Sprintf("grant_type=client_credentials&scopes=cloud_controller.admin_read_only")
	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", a.UAAAPI, "/oauth/token"), strings.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("unable to prepare authentication request: %s", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to perform authentication request: %s", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("unable to parse authentication response: %s", err)
	}

	err = json.Unmarshal(body, &a.tokens)
	if err != nil {
		return fmt.Errorf("unable to unmarshal authentication response: %s", err)
	}

	return nil
}

func (a *Auth) AccessToken() string {
	return a.tokens.AccessToken
}
