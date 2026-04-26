package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vooon/nebula-mnemosina/internal/db"
	"github.com/vooon/nebula-mnemosina/internal/model"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: db.New(pool),
	}
}

func (s *Store) SavePollResult(ctx context.Context, result model.PollResult) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin poll result transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	q := s.queries.WithTx(tx)
	pollRunID, err := q.CreatePollRun(ctx, db.CreatePollRunParams{
		StartedAt:      timestamptz(result.StartedAt),
		LighthouseName: result.Lighthouse.Name,
		SshTarget:      result.Lighthouse.Target(),
	})
	if err != nil {
		return fmt.Errorf("create poll run: %w", err)
	}

	for _, command := range result.Commands {
		if err := q.InsertRawCommandPayload(ctx, db.InsertRawCommandPayloadParams{
			PollRunID:      pollRunID,
			ObservedAt:     timestamptz(command.StartedAt),
			LighthouseName: result.Lighthouse.Name,
			Command:        command.Command,
			Success:        command.Success(),
			DurationMs:     millis(command.Duration()),
			Output:         command.Output,
			Error:          nullableError(command.Err),
		}); err != nil {
			return fmt.Errorf("insert raw command %q: %w", command.Command, err)
		}
	}

	if err := s.insertHostmapEntries(ctx, q, pollRunID, result, "hostmap", result.Hostmap); err != nil {
		return err
	}
	if err := s.insertHostmapEntries(ctx, q, pollRunID, result, "pending", result.Pending); err != nil {
		return err
	}
	if err := s.insertLighthouseAddrmap(ctx, q, pollRunID, result); err != nil {
		return err
	}
	if len(result.Relays) > 0 {
		if err := q.InsertRelaySnapshot(ctx, db.InsertRelaySnapshotParams{
			PollRunID:      pollRunID,
			ObservedAt:     timestamptz(result.StartedAt),
			LighthouseName: result.Lighthouse.Name,
			Relays:         result.Relays,
		}); err != nil {
			return fmt.Errorf("insert relay snapshot: %w", err)
		}
	}

	if err := q.CompletePollRun(ctx, db.CompletePollRunParams{
		ID:            pollRunID,
		FinishedAt:    timestamptz(result.FinishedAt),
		Success:       result.Success(),
		DurationMs:    millis(result.Duration()),
		Error:         nullableError(result.Err),
		NebulaVersion: nullableString(result.NebulaVersion),
		DeviceName:    nullableString(result.Device.Name),
		DeviceCidrs:   nonNil(result.Device.CIDR),
		CommandCount:  int32(len(result.Commands)),
	}); err != nil {
		return fmt.Errorf("complete poll run: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit poll result: %w", err)
	}
	return nil
}

func (s *Store) RefreshGrafanaViews(ctx context.Context) error {
	views := []string{
		"mnemo_current_peers",
		"mnemo_current_lighthouse_addrmap",
		"mnemo_poll_health_5m",
		"mnemo_peer_cert_inventory",
		"mnemo_lighthouse_disagreement",
	}
	for _, view := range views {
		if _, err := s.pool.Exec(ctx, "REFRESH MATERIALIZED VIEW "+view); err != nil {
			return fmt.Errorf("refresh materialized view %s: %w", view, err)
		}
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) ListPresentHostmapPeers(ctx context.Context) ([]model.PresentPeer, error) {
	rows, err := s.queries.ListPresentHostmapPeers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list present hostmap peers: %w", err)
	}

	peers := make([]model.PresentPeer, 0, len(rows))
	for _, row := range rows {
		peers = append(peers, model.PresentPeer{
			LighthouseName:  row.LighthouseName,
			PeerKey:         row.PeerKey,
			PrimaryVPNAddr:  textValue(row.PrimaryVpnAddr),
			VPNAddrs:        nonNil(row.VpnAddrs),
			CertName:        textValue(row.CertName),
			CertFingerprint: textValue(row.CertFingerprint),
			CertGroups:      nonNil(row.CertGroups),
			NebulaVersion:   textValue(row.NebulaVersion),
		})
	}
	return peers, nil
}

func (s *Store) insertHostmapEntries(ctx context.Context, q *db.Queries, pollRunID int64, result model.PollResult, source string, entries []model.HostmapEntry) error {
	for i, entry := range entries {
		raw := entry.Raw
		if len(raw) == 0 {
			encoded, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("marshal %s entry raw: %w", source, err)
			}
			raw = encoded
		}

		params := db.InsertHostmapEntryParams{
			PollRunID:              pollRunID,
			ObservedAt:             timestamptz(result.StartedAt),
			LighthouseName:         result.Lighthouse.Name,
			Source:                 source,
			EntryIndex:             int32(i),
			VpnAddrs:               nonNil(entry.VPNAddrs),
			PrimaryVpnAddr:         nullableString(entry.PrimaryVPNAddr()),
			LocalIndex:             int64(entry.LocalIndex),
			RemoteIndex:            int64(entry.RemoteIndex),
			RemoteAddrs:            nonNil(entry.RemoteAddrs),
			MessageCounter:         uint64ToInt64(entry.MessageCounter),
			CurrentRemote:          entry.CurrentRemote,
			CurrentRelaysToMe:      nonNil(entry.CurrentRelaysToMe),
			CurrentRelaysThroughMe: nonNil(entry.CurrentRelaysThroughMe),
			CertGroups:             []string{},
			CertNetworks:           []string{},
			CertUnsafeNetworks:     []string{},
			Raw:                    raw,
		}
		if entry.Cert != nil {
			cert := entry.Cert
			params.CertCurve = nullableString(cert.Curve)
			params.CertName = nullableString(cert.Details.Name)
			params.CertFingerprint = nullableString(cert.Fingerprint)
			params.CertPublicKey = nullableString(cert.PublicKey)
			params.CertSignature = nullableString(cert.Signature)
			params.CertVersion = pgtype.Int4{Int32: cert.Version, Valid: true}
			params.CertGroups = nonNil(cert.Details.Groups)
			params.CertIsCa = pgtype.Bool{Bool: cert.Details.IsCA, Valid: true}
			params.CertIssuer = nullableString(cert.Details.Issuer)
			params.CertNetworks = nonNil(cert.Details.Networks)
			params.CertUnsafeNetworks = nonNil(cert.Details.UnsafeNetworks)
			params.CertNotBefore = nullableTime(cert.Details.NotBefore)
			params.CertNotAfter = nullableTime(cert.Details.NotAfter)
		}

		if err := q.InsertHostmapEntry(ctx, params); err != nil {
			return fmt.Errorf("insert %s entry %d: %w", source, i, err)
		}
	}
	return nil
}

func (s *Store) insertLighthouseAddrmap(ctx context.Context, q *db.Queries, pollRunID int64, result model.PollResult) error {
	for _, entry := range result.Addrmap {
		if len(entry.Addrs) == 0 {
			if err := q.InsertLighthouseAddrmapEntry(ctx, db.InsertLighthouseAddrmapEntryParams{
				PollRunID:       pollRunID,
				ObservedAt:      timestamptz(result.StartedAt),
				LighthouseName:  result.Lighthouse.Name,
				VpnAddr:         entry.VPNAddr,
				ReporterVpnAddr: pgtype.Text{},
				LearnedAddrs:    []string{},
				ReportedAddrs:   []string{},
				RelayVpnAddrs:   []string{},
				Raw:             entry.Raw,
			}); err != nil {
				return fmt.Errorf("insert empty lighthouse addrmap %s: %w", entry.VPNAddr, err)
			}
			continue
		}

		for reporter, addrs := range entry.Addrs {
			if err := q.InsertLighthouseAddrmapEntry(ctx, db.InsertLighthouseAddrmapEntryParams{
				PollRunID:       pollRunID,
				ObservedAt:      timestamptz(result.StartedAt),
				LighthouseName:  result.Lighthouse.Name,
				VpnAddr:         entry.VPNAddr,
				ReporterVpnAddr: nullableString(reporter),
				LearnedAddrs:    nonNil(addrs.Learned),
				ReportedAddrs:   nonNil(addrs.Reported),
				RelayVpnAddrs:   nonNil(addrs.Relay),
				Raw:             entry.Raw,
			}); err != nil {
				return fmt.Errorf("insert lighthouse addrmap %s reporter %s: %w", entry.VPNAddr, reporter, err)
			}
		}
	}
	return nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

func nullableTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}

func nullableString(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableError(err error) pgtype.Text {
	if err == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: err.Error(), Valid: true}
}

func millis(duration time.Duration) int64 {
	return int64(duration / time.Millisecond)
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func uint64ToInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
