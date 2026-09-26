package identity

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateInlineBundleTenYears(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	bundle, err := GenerateInlineBundle("provider", "account", "device", 10, now)
	if err != nil {
		t.Fatal(err)
	}
	rootBlock, _ := pem.Decode([]byte(bundle.RootCertificate))
	if rootBlock == nil {
		t.Fatal("missing root certificate")
	}
	root, err := x509.ParseCertificate(rootBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := tls.X509KeyPair([]byte(bundle.GatewayCertificate), []byte(bundle.GatewayPrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	device, err := tls.X509KeyPair([]byte(bundle.DeviceCertificate), []byte(bundle.DevicePrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	for name, chain := range map[string]tls.Certificate{"gateway": gateway, "device": device} {
		leaf, err := x509.ParseCertificate(chain.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		if !leaf.NotAfter.Equal(now.AddDate(10, 0, 0)) {
			t.Fatalf("%s expires at %s", name, leaf.NotAfter)
		}
	}
	store, err := NewStaticStore([]StaticUser{{Name: "account", DeviceName: "device", AccountID: bundle.AccountID, DeviceID: bundle.DeviceID, PublicKey: bundle.DevicePublicKey}})
	if err != nil {
		t.Fatal(err)
	}
	if err := (ServerCredentials{ProviderID: bundle.ProviderID, GatewayID: bundle.GatewayID, Root: root, Certificate: gateway, Store: store}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ClientCredentials{ProviderID: bundle.ProviderID, GatewayID: bundle.GatewayID, Root: root, RootPin: RootPin(root), Certificate: device}).Validate(now); err != nil {
		t.Fatal(err)
	}
}
