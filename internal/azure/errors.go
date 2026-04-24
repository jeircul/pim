package azure

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrUserCancelled = errors.New("user cancelled")
	ErrNoCredential  = errors.New("no supported Azure login found; sign in with 'az login' or 'Connect-AzAccount'")
)

// APIError is a structured HTTP error returned by the Azure REST API.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("HTTP %d: %s - %s", e.StatusCode, e.Code, e.Message)
}

func errorFromResponse(resp *http.Response) *APIError {
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return &APIError{StatusCode: resp.StatusCode, Message: "<read error>"}
	}
	if len(body) == 0 {
		return &APIError{StatusCode: resp.StatusCode, Message: "empty response body"}
	}
	var azErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &azErr) == nil && azErr.Error.Code != "" {
		return &APIError{StatusCode: resp.StatusCode, Code: azErr.Error.Code, Message: azErr.Error.Message}
	}
	return &APIError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(body))}
}
