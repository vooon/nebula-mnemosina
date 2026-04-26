package config

import "testing"

func TestParseLighthouses(t *testing.T) {
	targets, err := parseLighthouses([]string{
		"lh1=nebula@192.168.110.1:4222",
		"nebula@192.168.110.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Name != "lh1" || targets[0].User != "nebula" || targets[0].Address != "192.168.110.1:4222" {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[1].Name != "192.168.110.2" || targets[1].Address != "192.168.110.2:4222" {
		t.Fatalf("unexpected default target: %+v", targets[1])
	}
}

func TestParseLighthousesRejectsDuplicateNames(t *testing.T) {
	_, err := parseLighthouses([]string{
		"lh=nebula@192.168.110.1:4222",
		"lh=nebula@192.168.110.2:4222",
	})
	if err == nil {
		t.Fatalf("expected duplicate name error")
	}
}
