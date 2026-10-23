package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// Price watches observe local simulated prices. Events are unique per watch and price version.
func (a *Server) upsertPriceWatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SKUId            int64 `json:"sku_id"`
		TargetPriceCents int64 `json:"target_price_cents"`
	}
	if readJSON(r, &in) != nil || in.SKUId <= 0 || in.TargetPriceCents <= 0 {
		fail(w, 400, "invalid_watch", "请选择商品并输入正数目标价")
		return
	}
	var exists int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT 1 FROM skus s JOIN products p ON p.id=s.product_id WHERE s.id=? AND s.active=1 AND p.active=1`, in.SKUId).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			fail(w, 404, "sku_not_found", "商品不存在或已下架")
		} else {
			logErr(r, err)
			fail(w, 503, "watch_unavailable", "")
		}
		return
	}
	_, err := a.DB.ExecContext(r.Context(), `INSERT INTO price_watches(user_id,sku_id,target_price_cents,active) VALUES(?,?,?,1)
		ON DUPLICATE KEY UPDATE target_price_cents=VALUES(target_price_cents),active=1`, current(r).UserID, in.SKUId, in.TargetPriceCents)
	if err != nil {
		logErr(r, err)
		fail(w, 503, "watch_unavailable", "")
		return
	}
	respond(w, 200, map[string]any{"sku_id": in.SKUId, "target_price_cents": in.TargetPriceCents, "active": true})
}

func (a *Server) listPriceWatches(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `SELECT w.id,w.sku_id,w.target_price_cents,w.active,s.price_cents,s.price_version,p.title,s.label
		FROM price_watches w JOIN skus s ON s.id=w.sku_id JOIN products p ON p.id=s.product_id WHERE w.user_id=? ORDER BY w.id DESC LIMIT 100`, current(r).UserID)
	if err != nil {
		logErr(r, err)
		fail(w, 503, "watch_unavailable", "")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, skuID, target, price, version int64
		var active bool
		var title, label string
		if err = rows.Scan(&id, &skuID, &target, &active, &price, &version, &title, &label); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "sku_id": skuID, "target_price_cents": target, "active": active, "current_price_cents": price, "price_version": version, "title": title, "label": label})
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		logErr(r, err)
		fail(w, 503, "watch_unavailable", "")
		return
	}
	respond(w, 200, map[string]any{"items": items})
}

func (a *Server) deletePriceWatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		fail(w, 400, "invalid_watch", "")
		return
	}
	result, err := a.DB.ExecContext(r.Context(), `UPDATE price_watches SET active=0 WHERE id=? AND user_id=? AND active=1`, id, current(r).UserID)
	if err != nil {
		logErr(r, err)
		fail(w, 503, "watch_unavailable", "")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		fail(w, 404, "watch_not_found", "")
		return
	}
	respond(w, 200, map[string]any{"id": id, "active": false})
}

func (a *Server) listPriceWatchEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `SELECT e.id,e.watch_id,w.sku_id,e.price_version,e.observed_price_cents,e.created_at,p.title,s.label
		FROM price_watch_events e JOIN price_watches w ON w.id=e.watch_id JOIN skus s ON s.id=w.sku_id JOIN products p ON p.id=s.product_id
		WHERE w.user_id=? ORDER BY e.id DESC LIMIT 100`, current(r).UserID)
	if err != nil {
		logErr(r, err)
		fail(w, 503, "watch_unavailable", "")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, watchID, skuID, version, price int64
		var at time.Time
		var title, label string
		if err = rows.Scan(&id, &watchID, &skuID, &version, &price, &at, &title, &label); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "watch_id": watchID, "sku_id": skuID, "price_version": version, "observed_price_cents": price, "created_at": at, "title": title, "label": label})
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		logErr(r, err)
		fail(w, 503, "watch_unavailable", "")
		return
	}
	respond(w, 200, map[string]any{"items": items})
}

// CheckPriceWatches is safe to retry and to run in multiple workers.
func CheckPriceWatches(ctx context.Context, db *sql.DB) (int64, error) {
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO price_watch_events(watch_id,price_version,observed_price_cents)
		SELECT w.id,s.price_version,s.price_cents FROM price_watches w
		JOIN skus s ON s.id=w.sku_id JOIN products p ON p.id=s.product_id
		WHERE w.active=1 AND s.active=1 AND p.active=1 AND s.available>0 AND s.price_cents<=w.target_price_cents`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
