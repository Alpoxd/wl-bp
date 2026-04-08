package accounts

import (
	"encoding/json"
	"fmt"
	"net/url"

	"server/internal/db"
	"server/internal/tunnel" // For http client utils
)

// ValidateVKAccount checks if the account's cookies are still valid
// by attempting to get a web_token.
func ValidateVKAccount(acc *db.HostAccount) error {
	if acc.Platform != "vk" {
		return fmt.Errorf("account %d is not a vk account", acc.ID)
	}

	appID := "51868352" // Using known web caller app id to ping tokens

	r, err := tunnel.HttpPost("https://login.vk.com/?act=web_token",
		url.Values{"version": {"1"}, "app_id": {appID}},
		map[string]string{"Cookie": acc.Cookies})

	if err != nil {
		return fmt.Errorf("web_token request failed: %w", err)
	}

	var tok struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
		Error string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}

	if err := json.Unmarshal(r, &tok); err != nil {
		return fmt.Errorf("failed to parse web_token response: %w", err)
	}

	if tok.Error != "" {
		return fmt.Errorf("vk API error: %s - %s", tok.Error, tok.ErrorDescription)
	}

	if tok.Data.AccessToken == "" {
		return fmt.Errorf("empty access token returned, cookies likely expired")
	}

	return nil
}

// ValidateVKCookies checks raw cookie string
func ValidateVKCookies(cookies string) error {
	appID := "51868352" 

	r, err := tunnel.HttpPost("https://login.vk.com/?act=web_token",
		url.Values{"version": {"1"}, "app_id": {appID}},
		map[string]string{"Cookie": cookies})

	if err != nil {
		return fmt.Errorf("web_token request failed: %w", err)
	}

	var tok struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	json.Unmarshal(r, &tok)

	if tok.Data.AccessToken == "" {
		return fmt.Errorf("invalid or expired cookies")
	}

	return nil
}
