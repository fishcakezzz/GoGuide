CREATE TABLE IF NOT EXISTS price_watches (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  sku_id BIGINT NOT NULL,
  target_price_cents BIGINT NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uniq_watch_user_sku (user_id,sku_id),
  INDEX idx_watch_active (active,sku_id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (sku_id) REFERENCES skus(id),
  CONSTRAINT chk_watch_target CHECK (target_price_cents > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS price_watch_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  watch_id BIGINT NOT NULL,
  price_version BIGINT NOT NULL,
  observed_price_cents BIGINT NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uniq_watch_price_version (watch_id,price_version),
  INDEX idx_watch_event_time (watch_id,created_at),
  FOREIGN KEY (watch_id) REFERENCES price_watches(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
