package trade

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("order not found")
var ErrConflict = errors.New("order state conflict")
var ErrInvalidCallback = errors.New("invalid payment callback")

type Callback struct {
	EventID     string `json:"event_id"`
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Outcome     string `json:"outcome"`
	Signature   string `json:"signature"`
}

func SignCallback(secret []byte, c Callback) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%d|%s", c.EventID, c.OrderID, c.AmountCents, c.Outcome)
	return hex.EncodeToString(mac.Sum(nil))
}
func ProcessCallback(ctx context.Context, db *sql.DB, secret []byte, c Callback) (string, error) {
	if c.OrderID == "" || c.EventID != c.OrderID || c.AmountCents < 0 || c.Outcome != "SUCCESS" || !hmac.Equal([]byte(c.Signature), []byte(SignCallback(secret, c))) {
		return "", ErrInvalidCallback
	}
	var owner, amount int64
	err := db.QueryRowContext(ctx, "SELECT user_id,amount_cents FROM orders WHERE id=?", c.OrderID).Scan(&owner, &amount)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if amount != c.AmountCents {
		return "", ErrInvalidCallback
	}
	return Pay(ctx, db, c.OrderID, owner)
}

type Item struct {
	SKUID          int64  `json:"sku_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	PriceVersion   int64  `json:"price_version"`
	Title          string `json:"title"`
	Label          string `json:"label"`
}

func moveItems(ctx context.Context, tx *sql.Tx, orderID, kind string) error {
	rows, err := tx.QueryContext(ctx, "SELECT sku_id,quantity FROM order_items WHERE order_id=? ORDER BY sku_id", orderID)
	if err != nil {
		return err
	}
	items := []Item{}
	for rows.Next() {
		var it Item
		if err = rows.Scan(&it.SKUID, &it.Quantity); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, it := range items {
		var da, dr, ds int
		var q string
		switch kind {
		case "paid":
			da, dr, ds = 0, -it.Quantity, it.Quantity
			q = "UPDATE skus SET reserved=reserved-?,sold=sold+?,stock_version=stock_version+1 WHERE id=? AND reserved>=?"
		case "released":
			da, dr, ds = it.Quantity, -it.Quantity, 0
			q = "UPDATE skus SET reserved=reserved-?,available=available+?,stock_version=stock_version+1 WHERE id=? AND reserved>=?"
		default:
			return fmt.Errorf("invalid movement %s", kind)
		}
		res, err := tx.ExecContext(ctx, q, it.Quantity, it.Quantity, it.SKUID, it.Quantity)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return fmt.Errorf("reservation mismatch for sku %d", it.SKUID)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO stock_movements(sku_id,order_id,event_key,delta_available,delta_reserved,delta_sold,reason) VALUES(?,?,?,?,?,?,?)", it.SKUID, orderID, fmt.Sprintf("%s:%s:%d", kind, orderID, it.SKUID), da, dr, ds, kind)
		if err != nil {
			return err
		}
	}
	return nil
}

func Close(ctx context.Context, db *sql.DB, orderID, reason string, userID int64) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var owner int64
	var status string
	var due time.Time
	err = tx.QueryRowContext(ctx, "SELECT user_id,status,expires_at FROM orders WHERE id=? FOR UPDATE", orderID).Scan(&owner, &status, &due)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if userID > 0 && owner != userID {
		return "", ErrNotFound
	}
	if status != "PENDING_PAYMENT" {
		return status, nil
	}
	if reason == "timeout" {
		var expired bool
		if err = tx.QueryRowContext(ctx, "SELECT NOW(6)>=?", due).Scan(&expired); err != nil {
			return "", err
		}
		if !expired {
			return status, nil
		}
	}
	to := "CLOSED"
	if reason == "cancel" {
		to = "CANCELLED"
	}
	if _, err = tx.ExecContext(ctx, "UPDATE orders SET status=?,closed_at=NOW(6) WHERE id=? AND status='PENDING_PAYMENT'", to, orderID); err != nil {
		return "", err
	}
	if err = moveItems(ctx, tx, orderID, "released"); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return to, nil
}

func Pay(ctx context.Context, db *sql.DB, orderID string, userID int64) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var owner, amount int64
	var status string
	var due time.Time
	err = tx.QueryRowContext(ctx, "SELECT user_id,status,amount_cents,expires_at FROM orders WHERE id=? FOR UPDATE", orderID).Scan(&owner, &status, &amount, &due)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if owner != userID {
		return "", ErrNotFound
	}
	if status == "PAID" {
		return status, nil
	}
	var expired bool
	if err = tx.QueryRowContext(ctx, "SELECT NOW(6)>=?", due).Scan(&expired); err != nil {
		return "", err
	}
	if status == "PENDING_PAYMENT" && expired {
		if _, err = tx.ExecContext(ctx, "UPDATE orders SET status='CLOSED',closed_at=NOW(6) WHERE id=?", orderID); err != nil {
			return "", err
		}
		if err = moveItems(ctx, tx, orderID, "released"); err != nil {
			return "", err
		}
		status = "CLOSED"
	}
	if status != "PENDING_PAYMENT" {
		if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO payment_events(event_id,order_id,amount_cents,outcome) VALUES(?,?,?,'LATE_SUCCESS')", orderID, orderID, amount); err != nil {
			return "", err
		}
		if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO trade_anomalies(order_id,event_id,reason) VALUES(?,?,?)", orderID, orderID, "mock payment attempted after "+status); err != nil {
			return "", err
		}
		if err = tx.Commit(); err != nil {
			return "", err
		}
		return status, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO payment_events(event_id,order_id,amount_cents,outcome) VALUES(?,?,?,'SUCCESS')", orderID, orderID, amount); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE orders SET status='PAID',paid_at=NOW(6) WHERE id=? AND status='PENDING_PAYMENT'", orderID); err != nil {
		return "", err
	}
	if err = moveItems(ctx, tx, orderID, "paid"); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return "PAID", nil
}

func CloseDue(ctx context.Context, db *sql.DB, limit int) (int, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, "SELECT id FROM orders WHERE status='PENDING_PAYMENT' AND expires_at<=NOW(6) ORDER BY expires_at LIMIT ?", limit)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		status, err := Close(ctx, db, id, "timeout", 0)
		if err != nil {
			return count, err
		}
		if status == "CLOSED" {
			count++
		}
	}
	return count, nil
}
