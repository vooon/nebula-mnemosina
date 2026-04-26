-- name: CreatePollRun :one
INSERT INTO poll_runs (
    started_at,
    lighthouse_name,
    ssh_target
) VALUES (
    $1, $2, $3
)
RETURNING id;

-- name: CompletePollRun :exec
UPDATE poll_runs
SET
    finished_at = $2,
    success = $3,
    duration_ms = $4,
    error = $5,
    nebula_version = $6,
    device_name = $7,
    device_cidrs = $8,
    command_count = $9
WHERE id = $1;

-- name: InsertRawCommandPayload :exec
INSERT INTO raw_command_payloads (
    poll_run_id,
    observed_at,
    lighthouse_name,
    command,
    success,
    duration_ms,
    output,
    error
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
);

-- name: InsertHostmapEntry :exec
INSERT INTO hostmap_entries (
    poll_run_id,
    observed_at,
    lighthouse_name,
    source,
    entry_index,
    vpn_addrs,
    primary_vpn_addr,
    local_index,
    remote_index,
    remote_addrs,
    message_counter,
    current_remote,
    current_relays_to_me,
    current_relays_through_me,
    cert_curve,
    cert_name,
    cert_fingerprint,
    cert_public_key,
    cert_signature,
    cert_version,
    cert_groups,
    cert_is_ca,
    cert_issuer,
    cert_networks,
    cert_unsafe_networks,
    cert_not_before,
    cert_not_after,
    raw
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
    $21, $22, $23, $24, $25, $26, $27, $28
);

-- name: InsertLighthouseAddrmapEntry :exec
INSERT INTO lighthouse_addrmap_entries (
    poll_run_id,
    observed_at,
    lighthouse_name,
    vpn_addr,
    reporter_vpn_addr,
    learned_addrs,
    reported_addrs,
    relay_vpn_addrs,
    raw
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
);

-- name: InsertRelaySnapshot :exec
INSERT INTO relay_snapshots (
    poll_run_id,
    observed_at,
    lighthouse_name,
    relays
) VALUES (
    $1, $2, $3, $4
);

-- name: ListPresentHostmapPeers :many
WITH latest_poll AS (
    SELECT DISTINCT ON (lighthouse_name)
        id,
        lighthouse_name,
        started_at,
        nebula_version
    FROM poll_runs
    WHERE success
    ORDER BY lighthouse_name, started_at DESC, id DESC
),
ranked AS (
    SELECT
        h.observed_at,
        h.lighthouse_name,
        COALESCE(h.cert_fingerprint, h.primary_vpn_addr, h.local_index::text || ':' || h.remote_index::text)::text AS peer_key,
        h.primary_vpn_addr,
        h.vpn_addrs,
        h.cert_name,
        h.cert_fingerprint,
        h.cert_groups,
        p.nebula_version,
        row_number() OVER (
            PARTITION BY COALESCE(h.cert_fingerprint, h.primary_vpn_addr, h.local_index::text || ':' || h.remote_index::text)
            ORDER BY h.observed_at DESC, h.lighthouse_name, h.entry_index
        ) AS row_rank
    FROM latest_poll p
    JOIN hostmap_entries h ON h.poll_run_id = p.id
    WHERE h.source = 'hostmap'
      AND h.primary_vpn_addr IS NOT NULL
      AND h.primary_vpn_addr <> ''
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
    nebula_version
FROM ranked
WHERE row_rank = 1
ORDER BY COALESCE(cert_name, primary_vpn_addr), primary_vpn_addr;
