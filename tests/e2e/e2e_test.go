//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/model"
	"github.com/vooon/nebula-mnemosina/internal/sshclient"
)

const (
	e2eNamespace           = "nebula-mnemosina-e2e"
	mnemosinaSvc           = "nebula-mnemosina"
	mnemosinaHTTP          = 12142
	configuredLighthouses  = 2
	expectedNebulaSDGroups = 5
	expectedPeers          = 3
)

var apiBaseURL string

type prometheusTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func TestE2E(t *testing.T) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skipf("kubectl not found in PATH: %v", err)
	}

	t.Run("pods_ready", func(t *testing.T) {
		waitForPodReady(t, e2eNamespace, "app=postgres", 120*time.Second)
		waitForPodReady(t, e2eNamespace, "app=nebula", 120*time.Second)
		waitForPodReady(t, e2eNamespace, "app=nebula-mnemosina", 120*time.Second)
	})

	ensurePeerTunnels(t)
	apiBaseURL = startPortForward(t, e2eNamespace, mnemosinaSvc, mnemosinaHTTP)

	t.Run("healthz", func(t *testing.T) {
		status, body := apiRequest(t, http.MethodGet, "/healthz", nil)
		if status != http.StatusOK {
			t.Fatalf("expected /healthz HTTP 200, got %d: %s", status, body)
		}
		if !strings.Contains(string(body), "ok") {
			t.Fatalf("unexpected /healthz body: %s", body)
		}
	})

	t.Run("readyz", func(t *testing.T) {
		waitForCondition(t, "/readyz to become ready", 60*time.Second, time.Second, func() bool {
			status, _, err := apiRequestNoFail(http.MethodGet, "/readyz", nil)
			return err == nil && status == http.StatusOK
		})
	})

	t.Run("polls_persisted", func(t *testing.T) {
		waitForCondition(t, "successful lighthouse polls to be stored", 90*time.Second, time.Second, func() bool {
			count, err := postgresCountNoFail("SELECT count(*) FROM poll_runs WHERE success")
			return err == nil && count >= configuredLighthouses
		})

		waitForCondition(t, "peer hostmap entries to be stored", 90*time.Second, time.Second, func() bool {
			count, err := postgresCountNoFail("SELECT count(DISTINCT cert_name) FROM hostmap_entries WHERE cert_name IN ('peer1', 'peer2', 'peer3')")
			return err == nil && count >= expectedPeers
		})

		if count := postgresCount(t, "SELECT count(*) FROM raw_command_payloads WHERE command = 'version' AND success"); count < configuredLighthouses {
			t.Fatalf("expected at least %d successful version payloads, got %d", configuredLighthouses, count)
		}
		if count := postgresCount(t, "SELECT count(*) FROM raw_command_payloads WHERE command = 'list-lighthouse-addrmap -json' AND success"); count < configuredLighthouses {
			t.Fatalf("expected at least %d successful lighthouse addrmap payloads, got %d", configuredLighthouses, count)
		}
	})

	t.Run("metrics", func(t *testing.T) {
		status, body := apiRequest(t, http.MethodGet, "/metrics", nil)
		if status != http.StatusOK {
			t.Fatalf("expected /metrics HTTP 200, got %d: %s", status, body)
		}
		metricsText := string(body)
		for _, metric := range []string{
			"nebula_mnemosina_polls_total",
			"nebula_mnemosina_poll_duration_seconds",
			"nebula_mnemosina_ssh_commands_total",
		} {
			if !strings.Contains(metricsText, metric) {
				t.Fatalf("expected /metrics to contain %q", metric)
			}
		}
	})

	t.Run("prometheus_sd", func(t *testing.T) {
		status, body := apiRequest(t, http.MethodGet, "/prometheus-sd", nil)
		if status != http.StatusOK {
			t.Fatalf("expected /prometheus-sd HTTP 200, got %d: %s", status, body)
		}

		var groups []prometheusTargetGroup
		if err := json.Unmarshal(body, &groups); err != nil {
			t.Fatalf("unmarshal prometheus sd response: %v\n%s", err, body)
		}
		if len(groups) < expectedNebulaSDGroups {
			t.Fatalf("expected at least %d target groups, got %d: %s", expectedNebulaSDGroups, len(groups), body)
		}

		targetsByName := map[string]string{}
		for _, group := range groups {
			if len(group.Targets) == 0 {
				t.Fatalf("target group without targets: %+v", group)
			}
			name := group.Labels["__meta_nebula_name"]
			if name == "" {
				t.Fatalf("target group without __meta_nebula_name: %+v", group)
			}
			targetsByName[name] = group.Targets[0]
			if group.Labels["__metrics_path__"] != "/metrics" {
				t.Fatalf("unexpected metrics path labels for %s: %+v", name, group.Labels)
			}
		}

		wantTargetsByName := map[string]string{
			"lh1":   "nebula-lh1:4280",
			"lh2":   "nebula-lh2:4280",
			"peer1": "192.168.110.103:4280",
			"peer2": "192.168.110.104:4280",
			"peer3": "192.168.110.105:4280",
		}
		for name, wantTarget := range wantTargetsByName {
			target := targetsByName[name]
			if target == "" {
				t.Fatalf("missing prometheus sd target for %s: %+v", name, groups)
			}
			if target != wantTarget {
				t.Fatalf("unexpected prometheus sd target for %s: got %q, want %q", name, target, wantTarget)
			}
		}
	})
}

func kubectl(t *testing.T, args ...string) string {
	t.Helper()
	t.Logf("kubectl %s", strings.Join(args, " "))

	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func kubectlNoFail(args ...string) (string, error) {
	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kubectl %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

func waitForPodReady(t *testing.T, namespace, labelSelector string, timeout time.Duration) {
	t.Helper()
	kubectl(
		t,
		"-n", namespace,
		"wait",
		"--for=condition=Ready",
		"pod",
		"-l", labelSelector,
		"--timeout="+timeout.String(),
	)
}

func waitForCondition(t *testing.T, description string, timeout, interval time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s after %s", description, timeout)
		}
		time.Sleep(interval)
	}
}

func startPortForward(t *testing.T, namespace, svcName string, remotePort int) string {
	t.Helper()
	address := startPortForwardAddress(t, namespace, svcName, remotePort)
	baseURL := "http://" + address
	waitForCondition(t, "kubectl port-forward HTTP endpoint to become reachable", 20*time.Second, 250*time.Millisecond, func() bool {
		status, _, err := apiRequestAbsoluteNoFail(baseURL, http.MethodGet, "/healthz", nil)
		return err == nil && status == http.StatusOK
	})
	return baseURL
}

func startPortForwardAddress(t *testing.T, namespace, svcName string, remotePort int) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localPort := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(
		ctx,
		"kubectl",
		"-n", namespace,
		"port-forward",
		"--address", "127.0.0.1",
		"svc/"+svcName,
		fmt.Sprintf("%d:%d", localPort, remotePort),
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("kubectl port-forward output:\n%s", out.String())
		}
	})

	address := fmt.Sprintf("127.0.0.1:%d", localPort)
	waitForCondition(t, "kubectl port-forward to become reachable", 20*time.Second, 250*time.Millisecond, func() bool {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	})

	return address
}

func ensurePeerTunnels(t *testing.T) {
	t.Helper()

	address := startPortForwardAddress(t, e2eNamespace, "nebula-lh1", 4222)
	client, err := sshclient.New(config.SSHConfig{
		KeyFile:        "tests/e2e/generated/ssh_client_key",
		HostKeyMode:    "insecure",
		KnownHostsPath: "tests/e2e/generated/known_hosts",
		Timeout:        10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	lighthouse := model.Lighthouse{Name: "lh1", User: "nebula", Address: address}
	peerAddrs := []string{
		"192.168.110.103",
		"192.168.110.104",
		"192.168.110.105",
	}

	waitForCondition(t, "peers to report to lighthouse", 60*time.Second, time.Second, func() bool {
		for _, peerAddr := range peerAddrs {
			result := client.Run(context.Background(), lighthouse, "query-lighthouse "+peerAddr)
			if result.Err != nil || strings.TrimSpace(result.Output) == "null" {
				return false
			}
		}
		return true
	})

	for _, peerAddr := range peerAddrs {
		peerAddr := peerAddr
		waitForCondition(t, "lighthouse tunnel to "+peerAddr, 60*time.Second, time.Second, func() bool {
			result := client.Run(context.Background(), lighthouse, "print-tunnel "+peerAddr)
			if result.Err == nil {
				return true
			}

			result = client.Run(context.Background(), lighthouse, "create-tunnel "+peerAddr)
			if result.Err != nil {
				return false
			}

			result = client.Run(context.Background(), lighthouse, "print-tunnel "+peerAddr)
			return result.Err == nil
		})
	}
}

func apiRequest(t *testing.T, method, path string, body io.Reader) (int, []byte) {
	t.Helper()
	status, respBody, err := apiRequestNoFail(method, path, body)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	return status, respBody
}

func apiRequestNoFail(method, path string, body io.Reader) (int, []byte, error) {
	if apiBaseURL == "" {
		return 0, nil, fmt.Errorf("API base URL is not initialized")
	}
	return apiRequestAbsoluteNoFail(apiBaseURL, method, path, body)
}

func apiRequestAbsoluteNoFail(baseURL, method, path string, body io.Reader) (int, []byte, error) {
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return 0, nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, respBody, nil
}

func postgresCount(t *testing.T, query string) int {
	t.Helper()
	count, err := postgresCountNoFail(query)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func postgresCountNoFail(query string) (int, error) {
	raw, err := kubectlNoFail(
		"exec", "-n", e2eNamespace, "deploy/postgres", "--",
		"env", "PGPASSWORD=nebula_mnemosina",
		"psql", "-U", "nebula_mnemosina", "-d", "nebula_mnemosina", "-Atc", query,
	)
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("parse count %q: %w", raw, err)
	}
	return count, nil
}
