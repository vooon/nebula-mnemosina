package model

import "time"

type Lighthouse struct {
	Name    string
	User    string
	Address string
}

func (l Lighthouse) Target() string {
	return l.User + "@" + l.Address
}

type CommandResult struct {
	Command    string
	StartedAt  time.Time
	FinishedAt time.Time
	Output     string
	Err        error
}

func (r CommandResult) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}

func (r CommandResult) Success() bool {
	return r.Err == nil
}

type PollResult struct {
	Lighthouse Lighthouse
	StartedAt  time.Time
	FinishedAt time.Time
	Commands   []CommandResult

	NebulaVersion string
	Device        DeviceInfo
	Hostmap       []HostmapEntry
	Pending       []HostmapEntry
	Addrmap       []LighthouseAddrmapEntry
	Relays        []byte

	Err error
}

func (r PollResult) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}

func (r PollResult) Success() bool {
	return r.Err == nil
}

type DeviceInfo struct {
	Name string   `json:"name"`
	CIDR []string `json:"cidr"`
}

type HostmapEntry struct {
	VPNAddrs               []string     `json:"vpnAddrs"`
	LocalIndex             uint32       `json:"localIndex"`
	RemoteIndex            uint32       `json:"remoteIndex"`
	RemoteAddrs            []string     `json:"remoteAddrs"`
	Cert                   *Certificate `json:"cert"`
	MessageCounter         uint64       `json:"messageCounter"`
	CurrentRemote          string       `json:"currentRemote"`
	CurrentRelaysToMe      []string     `json:"currentRelaysToMe"`
	CurrentRelaysThroughMe []string     `json:"currentRelaysThroughMe"`
	Raw                    []byte       `json:"-"`
}

func (e HostmapEntry) PrimaryVPNAddr() string {
	if len(e.VPNAddrs) == 0 {
		return ""
	}
	return e.VPNAddrs[0]
}

type Certificate struct {
	Curve       string             `json:"curve"`
	Details     CertificateDetails `json:"details"`
	Fingerprint string             `json:"fingerprint"`
	PublicKey   string             `json:"publicKey"`
	Signature   string             `json:"signature"`
	Version     int32              `json:"version"`
}

type CertificateDetails struct {
	Groups         []string  `json:"groups"`
	IsCA           bool      `json:"isCa"`
	Issuer         string    `json:"issuer"`
	Name           string    `json:"name"`
	Networks       []string  `json:"networks"`
	NotAfter       time.Time `json:"notAfter"`
	NotBefore      time.Time `json:"notBefore"`
	UnsafeNetworks []string  `json:"unsafeNetworks"`
}

type LighthouseAddrmapEntry struct {
	VPNAddr string                         `json:"vpnAddr"`
	Addrs   map[string]LighthouseAddrEntry `json:"addrs"`
	Raw     []byte                         `json:"-"`
}

type LighthouseAddrEntry struct {
	Learned  []string `json:"learned"`
	Reported []string `json:"reported"`
	Relay    []string `json:"relay"`
}
