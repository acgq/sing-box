package queqiao

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/sagernet/sing-box/option"
	Q "github.com/sagernet/sing-box/third_party/queqiao"
)

func parseRootCertificate(value string) (*x509.Certificate, error) {
	block, rest := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("root_certificate must contain one PEM certificate")
	}
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse root_certificate: %w", err)
	}
	return root, nil
}

func inlineClientCredentials(options option.QueqiaoOutboundOptions) (Q.ClientCredentials, error) {
	if options.ProviderID == "" || options.GatewayID == "" || options.RootCertificate == "" ||
		options.DeviceCertificate == "" || options.DevicePrivateKey == "" {
		return Q.ClientCredentials{}, errors.New("missing inline Queqiao client identity")
	}
	root, err := parseRootCertificate(options.RootCertificate)
	if err != nil {
		return Q.ClientCredentials{}, err
	}
	certificate, err := tls.X509KeyPair([]byte(options.DeviceCertificate), []byte(options.DevicePrivateKey))
	if err != nil {
		return Q.ClientCredentials{}, fmt.Errorf("load device certificate: %w", err)
	}
	credentials := Q.ClientCredentials{
		ProviderID: options.ProviderID, GatewayID: options.GatewayID,
		Root: root, RootPin: Q.RootPin(root), Certificate: certificate,
	}
	if err = credentials.Validate(time.Now()); err != nil {
		return Q.ClientCredentials{}, err
	}
	return credentials, nil
}

func inlineServerCredentials(options option.QueqiaoInboundOptions) (Q.ServerCredentials, error) {
	if options.ProviderID == "" || options.GatewayID == "" || options.RootCertificate == "" ||
		options.GatewayCertificate == "" || options.GatewayPrivateKey == "" {
		return Q.ServerCredentials{}, errors.New("missing inline Queqiao gateway identity")
	}
	root, err := parseRootCertificate(options.RootCertificate)
	if err != nil {
		return Q.ServerCredentials{}, err
	}
	certificate, err := tls.X509KeyPair([]byte(options.GatewayCertificate), []byte(options.GatewayPrivateKey))
	if err != nil {
		return Q.ServerCredentials{}, fmt.Errorf("load gateway certificate: %w", err)
	}
	users := make([]Q.StaticUser, len(options.Users))
	for index, user := range options.Users {
		users[index] = Q.StaticUser{
			Name: user.Name, DeviceName: user.DeviceName,
			AccountID: user.AccountID, DeviceID: user.DeviceID,
			PublicKey: user.PublicKey, MaxFlows: user.MaxFlows,
			MaxClients: user.MaxClients, ExpiresAt: user.ExpiresAt,
		}
	}
	store, err := Q.NewStaticStore(users)
	if err != nil {
		return Q.ServerCredentials{}, err
	}
	credentials := Q.ServerCredentials{
		ProviderID: options.ProviderID, GatewayID: options.GatewayID,
		Root: root, Certificate: certificate, Store: store,
	}
	if err = credentials.Validate(); err != nil {
		return Q.ServerCredentials{}, err
	}
	return credentials, nil
}
