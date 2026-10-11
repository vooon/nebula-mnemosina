package retention

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/vooon/nebula-mnemosina/internal/config"
)

type fakeStore struct {
	dropped         []string
	pollRuns        int64
	rawPayloads     int64
	pollCutoff      time.Time
	rawCutoff       time.Time
	pollDeleteCalls int
	rawDeleteCalls  int
}

func (f *fakeStore) DropChunks(_ context.Context, table string, _ time.Time) (int64, error) {
	f.dropped = append(f.dropped, table)
	return 0, nil
}

func (f *fakeStore) DeleteExpiredPollRuns(_ context.Context, cutoff time.Time, batch int32) (int64, error) {
	f.pollDeleteCalls++
	f.pollCutoff = cutoff
	n := min(f.pollRuns, int64(batch))
	f.pollRuns -= n
	return n, nil
}

func (f *fakeStore) DeleteExpiredRawCommandPayloads(_ context.Context, cutoff time.Time, batch int32) (int64, error) {
	f.rawDeleteCalls++
	f.rawCutoff = cutoff
	n := min(f.rawPayloads, int64(batch))
	f.rawPayloads -= n
	return n, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPruneDeletesInBatchesUntilDone(t *testing.T) {
	store := &fakeStore{pollRuns: 2*batchSize + 5, rawPayloads: batchSize}
	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	p := New(config.RetentionConfig{Period: 720 * time.Hour, RawPeriod: 168 * time.Hour}, false, store, discardLogger())

	if err := p.Prune(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if store.pollRuns != 0 || store.rawPayloads != 0 {
		t.Fatalf("rows left: poll_runs=%d raw=%d", store.pollRuns, store.rawPayloads)
	}
	if store.pollDeleteCalls != 3 {
		t.Fatalf("expected 3 poll run batches, got %d", store.pollDeleteCalls)
	}
	if store.rawDeleteCalls != 2 {
		t.Fatalf("expected 2 raw payload batches, got %d", store.rawDeleteCalls)
	}
	if !store.pollCutoff.Equal(now.Add(-720 * time.Hour)) {
		t.Fatalf("unexpected poll cutoff: %s", store.pollCutoff)
	}
	if !store.rawCutoff.Equal(now.Add(-168 * time.Hour)) {
		t.Fatalf("unexpected raw cutoff: %s", store.rawCutoff)
	}
	if len(store.dropped) != 0 {
		t.Fatalf("plain PostgreSQL must not drop chunks: %v", store.dropped)
	}
}

func TestPruneDropsTimescaleChunks(t *testing.T) {
	store := &fakeStore{}
	p := New(config.RetentionConfig{Period: 720 * time.Hour}, true, store, discardLogger())

	if err := p.Prune(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	want := []string{"raw_command_payloads", "hostmap_entries", "lighthouse_addrmap_entries", "relay_snapshots"}
	if !slices.Equal(store.dropped, want) {
		t.Fatalf("unexpected dropped tables: %v", store.dropped)
	}
	if store.rawDeleteCalls != 0 {
		t.Fatal("raw retention disabled; raw payloads expire with poll runs")
	}
}

func TestDisabledRetention(t *testing.T) {
	p := New(config.RetentionConfig{}, false, &fakeStore{}, discardLogger())
	if p.Enabled() {
		t.Fatal("expected retention to be disabled")
	}
}
