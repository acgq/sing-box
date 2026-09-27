// Package queqiao exposes the protocol engine for embedding in proxy applications.
// Authentication and protocol-1 framing remain compatible with queqiaod.
package queqiao

import (
	"github.com/sagernet/sing-box/third_party/queqiao/internal/identity"
	"github.com/sagernet/sing-box/third_party/queqiao/internal/pep"
)

type Client = pep.Client
type Server = pep.Server
type ClientConfig = pep.ClientConfig
type MemoryLimits = pep.MemoryLimits
type ServerConfig = pep.ServerConfig
type Principal = identity.Principal
type ClientProfile = identity.ClientProfile
type Provider = identity.Provider
type PacketAddress = pep.PacketAddress
type TransportKind = pep.TransportKind
type CongestionControlKind = pep.CongestionControlKind
type EnrollmentService = identity.EnrollmentService
type AccountLimits = identity.AccountLimits
type ClientCredentials = identity.ClientCredentials
type ServerCredentials = identity.ServerCredentials
type StaticUser = identity.StaticUser
type InlineBundle = identity.InlineBundle
type DialOptions = identity.DialOptions

var NewClient = pep.NewClient
var NewServer = pep.NewServer
var LoadClientProfile = identity.LoadClientProfile
var LoadProvider = identity.LoadProvider
var InitProvider = identity.InitProvider
var RenewProfileWithOptions = identity.RenewProfileWithOptions
var PeerAddressFromContext = pep.PeerAddressFromContext
var NewStaticStore = identity.NewStaticStore
var RootPin = identity.RootPin
var GenerateInlineBundle = identity.GenerateInlineBundle
