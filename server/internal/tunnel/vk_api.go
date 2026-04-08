package tunnel

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/pion/webrtc/v4"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type CallInfo struct {
	CallID     string
	JoinLink   string
	ShortLink  string
	OKJoinLink string
	TurnServer TurnServer
	StunServer StunServer
	WSEndpoint string
}

type TurnServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

type StunServer struct {
	URLs []string `json:"urls"`
}

func HttpPost(endpoint string, form url.Values, extraHeaders map[string]string) ([]byte, error) {
	body := form.Encode()
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://vk.com")
	req.Header.Set("Referer", "https://vk.com/")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func HttpGet(endpoint string) ([]byte, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if strings.Contains(resp.Request.URL.String(), "challenge") {
		return nil, fmt.Errorf("VK captcha required - open %s in browser and solve it", resp.Request.URL.String())
	}
	return io.ReadAll(resp.Body)
}

func CreateAndJoinCall(cookieStr, peerId string, cfg VKConfig) (*CallInfo, error) {
	if cfg.AppID == "" || cfg.APIVersion == "" {
		return nil, fmt.Errorf("config incomplete: app_id=%q api=%q", cfg.AppID, cfg.APIVersion)
	}

	auth := func(bearer string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + bearer}
	}

	log.Println("[auth] Getting VK token...")
	r, err := HttpPost("https://login.vk.com/?act=web_token",
		url.Values{"version": {"1"}, "app_id": {cfg.AppID}},
		map[string]string{"Cookie": cookieStr})
	if err != nil {
		return nil, fmt.Errorf("web_token: %w", err)
	}
	var tok struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	json.Unmarshal(r, &tok)
	vkToken := tok.Data.AccessToken
	if vkToken == "" {
		return nil, fmt.Errorf("empty VK token, response: %s", string(r))
	}

	log.Println("[auth] Getting call settings...")
	r, err = HttpPost("https://api.vk.com/method/calls.getSettings",
		url.Values{"v": {cfg.APIVersion}}, auth(vkToken))
	if err != nil {
		return nil, fmt.Errorf("calls.getSettings: %w", err)
	}
	var settings struct {
		Response struct {
			Settings struct {
				PublicKey string `json:"public_key"`
				IsDev     bool   `json:"is_dev"`
			} `json:"settings"`
		} `json:"response"`
	}
	json.Unmarshal(r, &settings)
	appKey := settings.Response.Settings.PublicKey
	if appKey == "" {
		return nil, fmt.Errorf("empty public_key, response: %s", string(r))
	}
	env := "production"
	if settings.Response.Settings.IsDev {
		env = "development"
	}

	log.Printf("[auth] Creating call peer_id=%s...", peerId)
	r, err = HttpPost("https://api.vk.com/method/calls.start",
		url.Values{"v": {cfg.APIVersion}, "peer_id": {peerId}}, auth(vkToken))
	if err != nil {
		return nil, fmt.Errorf("calls.start: %w", err)
	}
	var call struct {
		Response struct {
			CallID           string `json:"call_id"`
			JoinLink         string `json:"join_link"`
			OKJoinLink       string `json:"ok_join_link"`
			ShortCredentials struct {
				LinkWithPassword string `json:"link_with_password"`
			} `json:"short_credentials"`
		} `json:"response"`
	}
	json.Unmarshal(r, &call)
	c := call.Response
	if c.CallID == "" {
		return nil, fmt.Errorf("empty call_id, response: %s", string(r))
	}
	if c.OKJoinLink == "" {
		return nil, fmt.Errorf("empty ok_join_link, response: %s", string(r))
	}
	log.Printf("[auth] call_id: %s", c.CallID)
	log.Printf("[auth] join_link: %s", c.JoinLink)

	log.Println("[auth] Getting call token...")
	r, err = HttpPost("https://api.vk.com/method/messages.getCallToken",
		url.Values{"v": {cfg.APIVersion}, "env": {env}}, auth(vkToken))
	if err != nil {
		return nil, fmt.Errorf("messages.getCallToken: %w", err)
	}
	var ct struct {
		Response struct {
			Token      string `json:"token"`
			APIBaseURL string `json:"api_base_url"`
		} `json:"response"`
	}
	json.Unmarshal(r, &ct)
	if ct.Response.Token == "" {
		return nil, fmt.Errorf("empty call token, response: %s", string(r))
	}
	if ct.Response.APIBaseURL == "" {
		return nil, fmt.Errorf("empty api_base_url, response: %s", string(r))
	}

	log.Println("[auth] OK.ru auth...")
	apiBaseURL := strings.TrimRight(ct.Response.APIBaseURL, "/")
	if !strings.HasSuffix(apiBaseURL, "/fb.do") {
		apiBaseURL += "/fb.do"
	}
	sd, _ := json.Marshal(map[string]interface{}{
		"device_id": "headless-go-1", "client_version": cfg.AppVersion,
		"client_type": "SDK_JS", "auth_token": ct.Response.Token, "version": 3,
	})
	r, err = HttpPost(apiBaseURL, url.Values{
		"method": {"auth.anonymLogin"}, "application_key": {appKey},
		"format": {"json"}, "session_data": {string(sd)},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("auth.anonymLogin: %w", err)
	}
	var okAuth struct {
		SessionKey string `json:"session_key"`
	}
	json.Unmarshal(r, &okAuth)
	if okAuth.SessionKey == "" {
		return nil, fmt.Errorf("empty session_key, response: %s", string(r))
	}

	log.Println("[auth] Joining conversation...")
	ms, _ := json.Marshal(map[string]bool{
		"isAudioEnabled": false, "isVideoEnabled": true, "isScreenSharingEnabled": false,
	})
	r, err = HttpPost(apiBaseURL, url.Values{
		"method": {"vchat.joinConversationByLink"}, "session_key": {okAuth.SessionKey},
		"application_key": {appKey}, "format": {"json"}, "joinLink": {c.OKJoinLink},
		"isVideo": {"true"}, "isAudio": {"false"}, "mediaSettings": {string(ms)},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("vchat.joinConversationByLink: %w", err)
	}
	var jr struct {
		Endpoint   string     `json:"endpoint"`
		TurnServer TurnServer `json:"turn_server"`
		StunServer StunServer `json:"stun_server"`
	}
	json.Unmarshal(r, &jr)
	if jr.Endpoint == "" {
		return nil, fmt.Errorf("empty WS endpoint, response: %s", string(r))
	}

	return &CallInfo{
		CallID: c.CallID, JoinLink: c.JoinLink, ShortLink: c.ShortCredentials.LinkWithPassword,
		OKJoinLink: c.OKJoinLink, TurnServer: jr.TurnServer, StunServer: jr.StunServer,
		WSEndpoint: jr.Endpoint,
	}, nil
}

func BuildICEServers(callInfo *CallInfo) []webrtc.ICEServer {
	var servers []webrtc.ICEServer
	if len(callInfo.StunServer.URLs) > 0 {
		servers = append(servers, webrtc.ICEServer{URLs: callInfo.StunServer.URLs})
	}
	if len(callInfo.TurnServer.URLs) > 0 {
		urls := append([]string{}, callInfo.TurnServer.URLs...)
		urls = append(urls, urls[len(urls)-1]+"?transport=tcp")
		servers = append(servers, webrtc.ICEServer{
			URLs: urls, Username: callInfo.TurnServer.Username, Credential: callInfo.TurnServer.Credential,
		})
	}
	return servers
}
