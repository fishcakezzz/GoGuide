ALTER TABLE guide_proposals
  ADD COLUMN status ENUM('ACTIVE','STALE') NOT NULL DEFAULT 'ACTIVE',
  ADD INDEX idx_guide_active (status,confirmed_at,intent_version);

ALTER TABLE guide_messages
  ADD COLUMN replan_event_id BIGINT NULL,
  ADD UNIQUE KEY uniq_replan_message (replan_event_id);

CREATE TABLE IF NOT EXISTS guide_replans (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  session_id CHAR(36) NOT NULL,
  user_id BIGINT NOT NULL,
  old_intent_version INT NOT NULL,
  new_intent_version INT NOT NULL,
  reason VARCHAR(500) NOT NULL,
  status ENUM('PENDING','PROCESSING','DONE','FAILED') NOT NULL DEFAULT 'PENDING',
  locked_until DATETIME(6) NULL,
  proposal_count INT NOT NULL DEFAULT 0,
  error_message VARCHAR(500) NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  UNIQUE KEY uniq_replan_version (session_id,new_intent_version),
  INDEX idx_replan_pending (status,id),
  INDEX idx_replan_user (user_id,session_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
