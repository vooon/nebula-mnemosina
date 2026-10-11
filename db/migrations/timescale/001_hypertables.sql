CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Unique constraints on a hypertable must include the partitioning column.
ALTER TABLE raw_command_payloads DROP CONSTRAINT IF EXISTS raw_command_payloads_pkey;
ALTER TABLE raw_command_payloads ADD PRIMARY KEY (id, observed_at);

-- migrate_data allows enabling TimescaleDB on a database that already has
-- history. It copies existing rows into chunks, so it needs free disk space.
SELECT create_hypertable('hostmap_entries', 'observed_at', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE, migrate_data => TRUE);
SELECT create_hypertable('lighthouse_addrmap_entries', 'observed_at', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE, migrate_data => TRUE);
SELECT create_hypertable('raw_command_payloads', 'observed_at', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE, migrate_data => TRUE);
SELECT create_hypertable('relay_snapshots', 'observed_at', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE, migrate_data => TRUE);
