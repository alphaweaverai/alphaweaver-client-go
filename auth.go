package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AuthManager struct {
	config       *Config
	httpClient   *http.Client
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func NewAuthManager(cfg *Config) *AuthManager {
	return &AuthManager{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (am *AuthManager) Authenticate(email, password string) error {
	url := fmt.Sprintf("%s/auth/v1/token?grant_type=password", am.config.Supabase.URL)
	body, _ := json.Marshal(authRequest{Email: email, Password: password})
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", am.config.Supabase.AnonKey)
	// Some Supabase setups require Authorization header alongside apikey
	req.Header.Set("Authorization", "Bearer "+am.config.Supabase.AnonKey)

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("auth failed: http %d", resp.StatusCode)
	}

	var ar authResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return fmt.Errorf("parse auth response: %w", err)
	}
	am.accessToken = ar.AccessToken
	am.refreshToken = ar.RefreshToken
	am.tokenExpiry = time.Now().Add(time.Duration(ar.ExpiresIn) * time.Second)
	return nil
}

func (am *AuthManager) RefreshToken() error {
	if am.refreshToken == "" {
		return fmt.Errorf("no refresh token")
	}
	url := fmt.Sprintf("%s/auth/v1/token?grant_type=refresh_token", am.config.Supabase.URL)
	body, _ := json.Marshal(map[string]string{"refresh_token": am.refreshToken})
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", am.config.Supabase.AnonKey)
	// Mirror headers used by successful clients
	req.Header.Set("Authorization", "Bearer "+am.config.Supabase.AnonKey)

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("refresh failed: http %d", resp.StatusCode)
	}

	var ar authResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}
	am.accessToken = ar.AccessToken
	am.refreshToken = ar.RefreshToken
	am.tokenExpiry = time.Now().Add(time.Duration(ar.ExpiresIn) * time.Second)
	return nil
}

func (am *AuthManager) IsTokenValid() bool {
	if am.accessToken == "" {
		return false
	}
	return am.tokenExpiry.After(time.Now().Add(5 * time.Minute))
}

func (am *AuthManager) EnsureValidToken() error {
	if am.IsTokenValid() {
		return nil
	}
	if am.refreshToken != "" {
		return am.RefreshToken()
	}
	return fmt.Errorf("no valid token available")
}

func (am *AuthManager) GetAuthHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + am.accessToken,
		"apikey":        am.config.Supabase.AnonKey,
		"Content-Type":  "application/json",
	}
}

func (am *AuthManager) Logout() {
	am.accessToken = ""
	am.refreshToken = ""
	am.tokenExpiry = time.Time{}
}
