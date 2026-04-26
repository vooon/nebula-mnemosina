package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/model"
)

type Client struct {
	signer          ssh.Signer
	hostKeyCallback ssh.HostKeyCallback
	timeout         time.Duration
}

func New(cfg config.SSHConfig) (*Client, error) {
	signer, err := loadSigner(cfg)
	if err != nil {
		return nil, err
	}
	callback, err := hostKeyCallback(cfg.HostKeyMode, cfg.KnownHostsPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		signer:          signer,
		hostKeyCallback: callback,
		timeout:         cfg.Timeout,
	}, nil
}

func (c *Client) Run(ctx context.Context, lighthouse model.Lighthouse, command string) (result model.CommandResult) {
	startedAt := time.Now()
	result = model.CommandResult{
		Command:   command,
		StartedAt: startedAt,
	}
	defer func() {
		result.FinishedAt = time.Now()
	}()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	conn, err := (&net.Dialer{Timeout: c.timeout}).DialContext(ctx, "tcp", lighthouse.Address)
	if err != nil {
		result.Err = fmt.Errorf("dial ssh: %w", err)
		return result
	}
	defer conn.Close()

	clientConfig := &ssh.ClientConfig{
		User:            lighthouse.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(c.signer)},
		HostKeyCallback: c.hostKeyCallback,
		Timeout:         c.timeout,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, lighthouse.Address, clientConfig)
	if err != nil {
		result.Err = fmt.Errorf("open ssh client: %w", err)
		return result
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		result.Err = fmt.Errorf("open ssh session: %w", err)
		return result
	}
	defer session.Close()

	var output bytes.Buffer
	session.Stdout = &output
	session.Stderr = &output

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err = <-done:
		result.Output = output.String()
		if err != nil {
			result.Err = fmt.Errorf("run %q: %w", command, err)
		}
	case <-ctx.Done():
		_ = client.Close()
		result.Output = output.String()
		result.Err = ctx.Err()
	}

	return result
}

func loadSigner(cfg config.SSHConfig) (ssh.Signer, error) {
	var key []byte
	switch {
	case cfg.PrivateKey != "":
		key = []byte(normalizePrivateKey(cfg.PrivateKey))
	case cfg.KeyFile != "":
		data, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read ssh key file: %w", err)
		}
		key = data
	default:
		return nil, fmt.Errorf("ssh private key is required")
	}

	if cfg.KeyPassphrase != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(key, []byte(cfg.KeyPassphrase))
		if err != nil {
			return nil, fmt.Errorf("parse encrypted ssh private key: %w", err)
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse ssh private key: %w", err)
	}
	return signer, nil
}

func normalizePrivateKey(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, `\n`) && !strings.Contains(value, "\n") {
		value = strings.ReplaceAll(value, `\n`, "\n")
	}
	return value + "\n"
}

func hostKeyCallback(mode, path string) (ssh.HostKeyCallback, error) {
	switch mode {
	case "insecure":
		return ssh.InsecureIgnoreHostKey(), nil
	case "strict":
		if path == "" {
			return nil, fmt.Errorf("known_hosts path is required in strict mode")
		}
		return knownhosts.New(path)
	case "accept-new":
		if path == "" {
			return nil, fmt.Errorf("known_hosts path is required in accept-new mode")
		}
		return acceptNewHostKeyCallback(path)
	default:
		return nil, fmt.Errorf("unsupported host key mode %q", mode)
	}
}

func acceptNewHostKeyCallback(path string) (ssh.HostKeyCallback, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create known_hosts directory: %w", err)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create known_hosts file: %w", err)
		}
		_ = file.Close()
	} else if err != nil {
		return nil, fmt.Errorf("stat known_hosts file: %w", err)
	}

	var mu sync.Mutex
	current, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		mu.Lock()
		defer mu.Unlock()

		err := current(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			return fmt.Errorf("ssh host key mismatch for %s: %w", hostname, err)
		}

		if err := appendKnownHost(path, hostname, key); err != nil {
			return err
		}
		reloaded, err := knownhosts.New(path)
		if err != nil {
			return fmt.Errorf("reload known_hosts after learning %s: %w", hostname, err)
		}
		current = reloaded
		return nil
	}, nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open known_hosts for append: %w", err)
	}
	defer file.Close()

	line := knownhosts.Line([]string{hostname}, key)
	if _, err := file.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("append known host %s: %w", hostname, err)
	}
	return nil
}
