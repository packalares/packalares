package wizard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

// BindUserZoneLocal binds the user zone without DID/JWS/vault.
// Used in local cert mode to skip LarePass requirements.
func BindUserZoneLocal(bflUrl, accessToken string) error {
	log.Printf("Binding user zone (local mode, no DID/JWS)...")

	payload := map[string]string{
		// No JWSSignature or DID needed in local mode
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal bind request: %w", err)
	}

	url := bflUrl + "/bfl/settings/v1alpha1/binding"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create bind request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Authorization", accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bind request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bind request returned %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("User zone binding response: %s", string(respBody))
	return nil
}
