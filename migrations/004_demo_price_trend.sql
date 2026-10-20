CREATE TABLE IF NOT EXISTS demo_price_snapshots (
  sku_id BIGINT NOT NULL,
  observed_on DATE NOT NULL,
  price_cents BIGINT NOT NULL,
  event_label VARCHAR(80) NOT NULL DEFAULT '',
  PRIMARY KEY (sku_id, observed_on),
  FOREIGN KEY (sku_id) REFERENCES skus(id),
  CONSTRAINT chk_demo_price_positive CHECK (price_cents > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
