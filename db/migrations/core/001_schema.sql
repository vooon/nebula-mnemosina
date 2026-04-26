CREATE TABLE IF NOT EXISTS poll_runs (
    id bigserial PRIMARY KEY,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    lighthouse_name text NOT NULL,
    ssh_target text NOT NULL,
    success boolean NOT NULL DEFAULT false,
    duration_ms bigint NOT NULL DEFAULT 0,
    error text,
    nebula_version text,
    device_name text,
    device_cidrs text[] NOT NULL DEFAULT '{}',
    command_count integer NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS poll_runs_started_at_idx ON poll_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS poll_runs_lighthouse_started_at_idx ON poll_runs (lighthouse_name, started_at DESC);

CREATE TABLE IF NOT EXISTS raw_command_payloads (
    id bigserial PRIMARY KEY,
    poll_run_id bigint NOT NULL REFERENCES poll_runs(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    lighthouse_name text NOT NULL,
    command text NOT NULL,
    success boolean NOT NULL,
    duration_ms bigint NOT NULL,
    output text NOT NULL DEFAULT '',
    error text
);

CREATE INDEX IF NOT EXISTS raw_command_payloads_poll_run_id_idx ON raw_command_payloads (poll_run_id);
CREATE INDEX IF NOT EXISTS raw_command_payloads_observed_at_idx ON raw_command_payloads (observed_at DESC);

CREATE TABLE IF NOT EXISTS hostmap_entries (
    id bigserial,
    poll_run_id bigint NOT NULL REFERENCES poll_runs(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    lighthouse_name text NOT NULL,
    source text NOT NULL CHECK (source IN ('hostmap', 'pending', 'tunnel')),
    entry_index integer NOT NULL DEFAULT 0,
    vpn_addrs text[] NOT NULL DEFAULT '{}',
    primary_vpn_addr text,
    local_index bigint NOT NULL DEFAULT 0,
    remote_index bigint NOT NULL DEFAULT 0,
    remote_addrs text[] NOT NULL DEFAULT '{}',
    message_counter bigint NOT NULL DEFAULT 0,
    current_remote text NOT NULL DEFAULT '',
    current_relays_to_me text[] NOT NULL DEFAULT '{}',
    current_relays_through_me text[] NOT NULL DEFAULT '{}',
    cert_curve text,
    cert_name text,
    cert_fingerprint text,
    cert_public_key text,
    cert_signature text,
    cert_version integer,
    cert_groups text[] NOT NULL DEFAULT '{}',
    cert_is_ca boolean,
    cert_issuer text,
    cert_networks text[] NOT NULL DEFAULT '{}',
    cert_unsafe_networks text[] NOT NULL DEFAULT '{}',
    cert_not_before timestamptz,
    cert_not_after timestamptz,
    raw jsonb NOT NULL
);

CREATE INDEX IF NOT EXISTS hostmap_entries_observed_at_idx ON hostmap_entries (observed_at DESC);
CREATE INDEX IF NOT EXISTS hostmap_entries_lighthouse_observed_at_idx ON hostmap_entries (lighthouse_name, observed_at DESC);
CREATE INDEX IF NOT EXISTS hostmap_entries_primary_vpn_addr_idx ON hostmap_entries (primary_vpn_addr);
CREATE INDEX IF NOT EXISTS hostmap_entries_cert_fingerprint_idx ON hostmap_entries (cert_fingerprint);
CREATE INDEX IF NOT EXISTS hostmap_entries_source_idx ON hostmap_entries (source);

CREATE TABLE IF NOT EXISTS lighthouse_addrmap_entries (
    id bigserial,
    poll_run_id bigint NOT NULL REFERENCES poll_runs(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    lighthouse_name text NOT NULL,
    vpn_addr text NOT NULL,
    reporter_vpn_addr text,
    learned_addrs text[] NOT NULL DEFAULT '{}',
    reported_addrs text[] NOT NULL DEFAULT '{}',
    relay_vpn_addrs text[] NOT NULL DEFAULT '{}',
    raw jsonb NOT NULL
);

CREATE INDEX IF NOT EXISTS lighthouse_addrmap_entries_observed_at_idx ON lighthouse_addrmap_entries (observed_at DESC);
CREATE INDEX IF NOT EXISTS lighthouse_addrmap_entries_lighthouse_observed_at_idx ON lighthouse_addrmap_entries (lighthouse_name, observed_at DESC);
CREATE INDEX IF NOT EXISTS lighthouse_addrmap_entries_vpn_addr_idx ON lighthouse_addrmap_entries (vpn_addr);

CREATE TABLE IF NOT EXISTS relay_snapshots (
    id bigserial,
    poll_run_id bigint NOT NULL REFERENCES poll_runs(id) ON DELETE CASCADE,
    observed_at timestamptz NOT NULL,
    lighthouse_name text NOT NULL,
    relays jsonb NOT NULL
);

CREATE INDEX IF NOT EXISTS relay_snapshots_observed_at_idx ON relay_snapshots (observed_at DESC);
CREATE INDEX IF NOT EXISTS relay_snapshots_lighthouse_observed_at_idx ON relay_snapshots (lighthouse_name, observed_at DESC);
