-- Retention deletes poll_runs and relies on ON DELETE CASCADE. Without these
-- indexes every deleted poll run scans the whole child table.
CREATE INDEX IF NOT EXISTS hostmap_entries_poll_run_id_idx ON hostmap_entries (poll_run_id);
CREATE INDEX IF NOT EXISTS lighthouse_addrmap_entries_poll_run_id_idx ON lighthouse_addrmap_entries (poll_run_id);
CREATE INDEX IF NOT EXISTS relay_snapshots_poll_run_id_idx ON relay_snapshots (poll_run_id);
