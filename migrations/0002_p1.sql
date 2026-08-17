-- P1 expand: new tables and nullable columns. Unique keys applied after AutoMigrate.
-- Contract phase (NOT NULL / drop old indexes) happens in a later release after backfill.

CREATE TABLE IF NOT EXISTS ledger_job_lease (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  job_name VARCHAR(64) NOT NULL,
  holder VARCHAR(128) NOT NULL,
  expires_at DATETIME NOT NULL,
  updated_at DATETIME,
  UNIQUE KEY uk_job_lease_name (job_name),
  KEY idx_job_lease_exp (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ledger_config_revision (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  revision_id VARCHAR(64) NOT NULL,
  version BIGINT NOT NULL,
  operator VARCHAR(64),
  checksum VARCHAR(64),
  payload TEXT,
  applied_at DATETIME,
  UNIQUE KEY uk_config_rev_id (revision_id),
  UNIQUE KEY uk_config_rev_ver (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ledger_mq_inbox (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  message_id VARCHAR(128) NOT NULL,
  topic VARCHAR(64) NOT NULL,
  schema_version INT NOT NULL DEFAULT 1,
  payload TEXT,
  status VARCHAR(16) NOT NULL,
  attempts INT,
  last_error TEXT,
  next_retry_at DATETIME,
  created_at DATETIME,
  updated_at DATETIME,
  UNIQUE KEY uk_mq_inbox_msg (message_id),
  KEY idx_mq_inbox_status (status),
  KEY idx_mq_inbox_retry (next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ledger_reconcile_diff_event (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  event_id VARCHAR(64) NOT NULL,
  diff_id VARCHAR(64) NOT NULL,
  action VARCHAR(32) NOT NULL,
  operator VARCHAR(64),
  detail TEXT,
  created_at DATETIME,
  UNIQUE KEY uk_diff_event_id (event_id),
  KEY idx_diff_event_diff (diff_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ledger_schema_migration (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  version VARCHAR(16) NOT NULL,
  name VARCHAR(128) NOT NULL,
  applied_at DATETIME,
  success TINYINT(1),
  error TEXT,
  UNIQUE KEY uk_schema_mig_ver (version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
