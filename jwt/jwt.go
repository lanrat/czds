// Package jwt defines the JWT types used by the CZDS authentication API.
package jwt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Token stores a JWT token.
type Token struct {
	Header    Header
	Data      Data
	Signature []byte
}

// Header represents the JWT header.
type Header struct {
	Kid string `json:"kid"`
	Alg string `json:"alg"`
}

// Data represents the JWT payload data.
type Data struct {
	Ver        int64    `json:"ver"`
	Jti        string   `json:"jti"`
	Iss        string   `json:"iss"`
	Aud        string   `json:"aud"`
	Iat        int64    `json:"iat"`
	Exp        int64    `json:"exp"`
	Cid        string   `json:"cid"`
	UID        string   `json:"uid"`
	SCP        []string `json:"scp"`
	Sub        string   `json:"sub"`
	GivenName  string   `json:"given_name"`
	FamilyName string   `json:"family_name"`
	Email      string   `json:"email"`
}

// DecodeJWT decodes a JWT encoded string and returns the decoded Token.
func DecodeJWT(jwtStr string) (*Token, error) {
	token := &Token{}
	parts := strings.Split(jwtStr, ".")
	if len(parts) != 3 {
		return token, fmt.Errorf("JWT Token has %d parts not 3: %s", len(parts), jwtStr)
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return token, err
	}
	dataBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return token, err
	}
	token.Signature, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return token, err
	}
	err = json.Unmarshal(headerBytes, &token.Header)
	if err != nil {
		return token, err
	}
	err = json.Unmarshal(dataBytes, &token.Data)

	return token, err
}
