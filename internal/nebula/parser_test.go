package nebula

import "testing"

func TestParseHostmap(t *testing.T) {
	output := `[{"vpnAddrs":["192.168.110.20","fd42:a3b0:110::20"],"localIndex":3022470974,"remoteIndex":538674575,"remoteAddrs":["31.185.6.161:65115"],"cert":{"curve":"CURVE25519","details":{"groups":["router"],"isCa":false,"issuer":"issuer","name":"plein-aire","networks":["192.168.110.20/24"],"notAfter":"2027-04-09T10:26:30Z","notBefore":"2026-04-09T10:27:32Z","unsafeNetworks":["192.168.20.0/22"]},"fingerprint":"fp","publicKey":"pk","signature":"sig","version":2},"messageCounter":441,"currentRemote":"31.185.6.161:65115","currentRelaysToMe":[],"currentRelaysThroughMe":[]},{"vpnAddrs":["192.168.110.10"],"localIndex":4112944129,"remoteIndex":0,"remoteAddrs":[],"cert":null,"messageCounter":2,"currentRemote":"","currentRelaysToMe":[],"currentRelaysThroughMe":[]}]`

	entries, err := ParseHostmap(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].PrimaryVPNAddr() != "192.168.110.20" {
		t.Fatalf("unexpected primary addr: %q", entries[0].PrimaryVPNAddr())
	}
	if entries[0].Cert == nil || entries[0].Cert.Details.Name != "plein-aire" {
		t.Fatalf("expected cert details to parse")
	}
	if entries[1].Cert != nil {
		t.Fatalf("expected pending host cert to be nil")
	}
	if len(entries[0].Raw) == 0 {
		t.Fatalf("expected raw entry payload")
	}
}

func TestParseLighthouseAddrmap(t *testing.T) {
	output := `[{"vpnAddr":"192.168.110.10","addrs":{}},{"vpnAddr":"192.168.110.20","addrs":{"192.168.110.20":{"learned":["31.185.6.161:65115"],"reported":["100.64.9.197:4242"],"relay":["192.168.110.1","192.168.110.2"]}}}]`

	entries, err := ParseLighthouseAddrmap(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if len(entries[0].Addrs) != 0 {
		t.Fatalf("expected empty addr map")
	}
	peer := entries[1].Addrs["192.168.110.20"]
	if got := peer.Learned[0]; got != "31.185.6.161:65115" {
		t.Fatalf("unexpected learned addr: %q", got)
	}
	if len(entries[1].Raw) == 0 {
		t.Fatalf("expected raw addrmap payload")
	}
}

func TestParseRelays(t *testing.T) {
	raw, err := ParseRelays(`{"Relays":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"Relays":null}` {
		t.Fatalf("unexpected relays raw: %s", raw)
	}
}
