CREATE EXTENSION IF NOT EXISTS timescaledb;

SELECT create_hypertable('hostmap_entries', 'observed_at', if_not_exists => TRUE);
SELECT create_hypertable('lighthouse_addrmap_entries', 'observed_at', if_not_exists => TRUE);
SELECT create_hypertable('raw_command_payloads', 'observed_at', if_not_exists => TRUE);
SELECT create_hypertable('relay_snapshots', 'observed_at', if_not_exists => TRUE);
