package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// InlineBundle is a self-contained gateway and one enrolled device. The CA
// private keys are discarded after generation, so the resulting configuration
// cannot issue another device or renew these certificates.
type InlineBundle struct {
	ProviderID         string
	GatewayID          string
	RootCertificate    string
	GatewayCertificate string
	GatewayPrivateKey  string
	AccountID          string
	DeviceID           string
	DevicePublicKey    string
	DeviceCertificate  string
	DevicePrivateKey   string
	ExpiresAt          time.Time
}

// GenerateInlineBundle creates a fresh Queqiao trust domain without provider
// state files. Validity is one to ten calendar years; the root and issuers
// outlive both leaves so their chains remain valid for the entire period.
func GenerateInlineBundle(providerName, accountName, deviceName string, validYears int, now time.Time) (InlineBundle, error) {
	for _, name := range []string{providerName, accountName, deviceName} {
		if name == "" || name != strings.TrimSpace(name) || len(name) > 128 {
			return InlineBundle{}, errors.New("Queqiao names must contain 1-128 non-whitespace characters")
		}
	}
	if validYears < 1 || validYears > 10 {
		return InlineBundle{}, errors.New("valid_years must be between 1 and 10")
	}
	if now.IsZero() {
		now = time.Now()
	}
	leafExpiry := now.AddDate(validYears, 0, 0)
	root, rootKey, err := newRootUntil(providerName, now, leafExpiry.AddDate(1, 0, 0))
	if err != nil {
		return InlineBundle{}, err
	}
	providerID := providerID(root)
	gatewayID, err := randomID()
	if err != nil {
		return InlineBundle{}, err
	}
	accountID, err := randomID()
	if err != nil {
		return InlineBundle{}, err
	}
	deviceID, err := randomID()
	if err != nil {
		return InlineBundle{}, err
	}
	issuerExpiry := leafExpiry.AddDate(0, 0, 1)
	gatewayIssuer, gatewayIssuerKey, err := newIssuerUntil(root, rootKey, providerName+" gateway issuer", providerID, "gateway-issuer", x509.ExtKeyUsageServerAuth, now, issuerExpiry)
	if err != nil {
		return InlineBundle{}, err
	}
	deviceIssuer, deviceIssuerKey, err := newIssuerUntil(root, rootKey, providerName+" device issuer", providerID, "device-issuer", x509.ExtKeyUsageClientAuth, now, issuerExpiry)
	if err != nil {
		return InlineBundle{}, err
	}
	gateway, gatewayKey, err := newLeaf(gatewayIssuer, gatewayIssuerKey, gatewayURI(providerID, gatewayID), x509.ExtKeyUsageServerAuth, leafExpiry.Sub(now), now)
	if err != nil {
		return InlineBundle{}, err
	}
	devicePublicKey, devicePrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return InlineBundle{}, fmt.Errorf("generate device key: %w", err)
	}
	provider := &Provider{Metadata: ProviderMetadata{ProviderID: providerID}, DeviceIssuer: deviceIssuer, DeviceIssuerKey: deviceIssuerKey}
	deviceCertificate, err := provider.issueDeviceWithTTL(accountID, deviceID, devicePublicKey, now, leafExpiry.Sub(now))
	if err != nil {
		return InlineBundle{}, err
	}
	return InlineBundle{
		ProviderID: providerID, GatewayID: gatewayID,
		RootCertificate:    string(encodeCertificate(root)),
		GatewayCertificate: string(encodeCertificateChain(gateway, gatewayIssuer, root)),
		GatewayPrivateKey:  string(encodePrivateKey(gatewayKey)),
		AccountID:          accountID, DeviceID: deviceID,
		DevicePublicKey:   base64.RawURLEncoding.EncodeToString(devicePublicKey),
		DeviceCertificate: string(deviceCertificate),
		DevicePrivateKey:  string(encodePrivateKey(devicePrivateKey)),
		ExpiresAt:         leafExpiry,
	}, nil
}
