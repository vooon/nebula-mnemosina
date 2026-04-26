DROP MATERIALIZED VIEW IF EXISTS mnemo_lighthouse_disagreement;
DROP MATERIALIZED VIEW IF EXISTS mnemo_peer_cert_inventory;
DROP MATERIALIZED VIEW IF EXISTS mnemo_poll_health_5m;
DROP MATERIALIZED VIEW IF EXISTS mnemo_current_lighthouse_addrmap;
DROP MATERIALIZED VIEW IF EXISTS mnemo_current_peers;

CREATE MATERIALIZED VIEW mnemo_current_peers AS
WITH ranked AS (
    SELECT
        h.*,
        p.nebula_version,
        COALESCE(h.cert_fingerprint, h.primary_vpn_addr, h.local_index::text || ':' || h.remote_index::text) AS peer_key,
        row_number() OVER (
            PARTITION BY h.lighthouse_name, COALESCE(h.cert_fingerprint, h.primary_vpn_addr, h.local_index::text || ':' || h.remote_index::text)
            ORDER BY h.observed_at DESC, h.entry_index ASC
        ) AS row_rank
    FROM hostmap_entries h
    JOIN poll_runs p ON p.id = h.poll_run_id
    WHERE h.source = 'hostmap'
)
SELECT
    observed_at,
    lighthouse_name,
    peer_key,
    primary_vpn_addr,
    vpn_addrs,
    cert_name,
    cert_fingerprint,
    cert_groups,
    cert_networks,
    cert_unsafe_networks,
    cert_not_before,
    cert_not_after,
    current_remote,
    remote_addrs,
    message_counter,
    local_index,
    remote_index,
    nebula_version
FROM ranked
WHERE row_rank = 1;

CREATE UNIQUE INDEX mnemo_current_peers_uq ON mnemo_current_peers (lighthouse_name, peer_key);
CREATE INDEX mnemo_current_peers_primary_vpn_addr_idx ON mnemo_current_peers (primary_vpn_addr);

CREATE MATERIALIZED VIEW mnemo_current_lighthouse_addrmap AS
WITH ranked AS (
    SELECT
        *,
        row_number() OVER (
            PARTITION BY lighthouse_name, vpn_addr, COALESCE(reporter_vpn_addr, '')
            ORDER BY observed_at DESC
        ) AS row_rank
    FROM lighthouse_addrmap_entries
)
SELECT
    observed_at,
    lighthouse_name,
    vpn_addr,
    reporter_vpn_addr,
    learned_addrs,
    reported_addrs,
    relay_vpn_addrs
FROM ranked
WHERE row_rank = 1;

CREATE UNIQUE INDEX mnemo_current_lighthouse_addrmap_uq
    ON mnemo_current_lighthouse_addrmap (lighthouse_name, vpn_addr, COALESCE(reporter_vpn_addr, ''));

CREATE MATERIALIZED VIEW mnemo_poll_health_5m AS
SELECT
    date_bin('5 minutes', started_at, '2000-01-01 00:00:00+00'::timestamptz) AS bucket,
    lighthouse_name,
    count(*) AS polls,
    count(*) FILTER (WHERE success) AS successful_polls,
    count(*) FILTER (WHERE NOT success) AS failed_polls,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95_duration_ms,
    max(started_at) AS latest_poll_at
FROM poll_runs
GROUP BY bucket, lighthouse_name;

CREATE UNIQUE INDEX mnemo_poll_health_5m_uq ON mnemo_poll_health_5m (bucket, lighthouse_name);

CREATE MATERIALIZED VIEW mnemo_peer_cert_inventory AS
WITH ranked AS (
    SELECT
        *,
        row_number() OVER (
            PARTITION BY COALESCE(cert_fingerprint, primary_vpn_addr)
            ORDER BY observed_at DESC
        ) AS row_rank
    FROM hostmap_entries
    WHERE source = 'hostmap'
)
SELECT
    observed_at,
    COALESCE(cert_fingerprint, primary_vpn_addr) AS peer_key,
    primary_vpn_addr,
    vpn_addrs,
    cert_name,
    cert_fingerprint,
    cert_public_key,
    cert_groups,
    cert_networks,
    cert_unsafe_networks,
    cert_not_before,
    cert_not_after
FROM ranked
WHERE row_rank = 1;

CREATE UNIQUE INDEX mnemo_peer_cert_inventory_uq ON mnemo_peer_cert_inventory (peer_key);

CREATE MATERIALIZED VIEW mnemo_lighthouse_disagreement AS
SELECT
    vpn_addr,
    count(DISTINCT lighthouse_name) AS lighthouses_reporting,
    count(DISTINCT learned_addrs::text) AS distinct_learned_sets,
    array_agg(DISTINCT lighthouse_name || '=' || learned_addrs::text ORDER BY lighthouse_name || '=' || learned_addrs::text) AS learned_by_lighthouse,
    max(observed_at) AS latest_observed_at
FROM mnemo_current_lighthouse_addrmap
GROUP BY vpn_addr
HAVING count(DISTINCT lighthouse_name) > 1
   AND count(DISTINCT learned_addrs::text) > 1;

CREATE UNIQUE INDEX mnemo_lighthouse_disagreement_uq ON mnemo_lighthouse_disagreement (vpn_addr);
