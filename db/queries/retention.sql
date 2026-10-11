-- name: ListExpiredPollRunIDs :many
SELECT id
FROM poll_runs
WHERE started_at < $1
ORDER BY started_at
LIMIT $2;

-- name: DeletePollRuns :execrows
DELETE FROM poll_runs
WHERE id = ANY(@ids::bigint[]);

-- name: DeleteExpiredRawCommandPayloads :execrows
DELETE FROM raw_command_payloads d
WHERE d.observed_at < @cutoff
  AND d.id IN (
    SELECT r.id
    FROM raw_command_payloads r
    WHERE r.observed_at < @cutoff
    ORDER BY r.observed_at
    LIMIT @batch_size
);
