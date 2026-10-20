package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"goguide/internal/domain"
)

type searchFilter struct {
	Category, Brand, Keyword            string
	MinPrice, MaxPrice                  int64
	InStock                             bool
	MinMemoryGB, MinWidth, MinRefreshHz int
	Limit, Offset                       int
}

func (a *Server) searchSKUs(ctx context.Context, f searchFilter) ([]domain.SKU, error) {
	q := `SELECT s.id,s.product_id,p.category,p.brand,p.title,s.label,p.description,p.image_url,p.source_kind,p.source_url,s.specs,s.price_cents,s.price_version,s.available,s.active
 FROM skus s JOIN products p ON p.id=s.product_id WHERE s.active=TRUE AND p.active=TRUE`
	args := []any{}
	if f.Category != "" {
		q += " AND p.category=?"
		args = append(args, f.Category)
	}
	if f.Brand != "" {
		q += " AND p.brand=?"
		args = append(args, f.Brand)
	}
	if f.Keyword != "" {
		q += " AND (p.title LIKE ? OR p.description LIKE ? OR s.label LIKE ?)"
		k := "%" + f.Keyword + "%"
		args = append(args, k, k, k)
	}
	if f.MinPrice > 0 {
		q += " AND s.price_cents>=?"
		args = append(args, f.MinPrice)
	}
	if f.MaxPrice > 0 {
		q += " AND s.price_cents<=?"
		args = append(args, f.MaxPrice)
	}
	if f.InStock {
		q += " AND s.available>0"
	}
	if f.MinMemoryGB > 0 {
		q += " AND CAST(JSON_UNQUOTE(JSON_EXTRACT(s.specs,'$.memory_gb')) AS UNSIGNED)>=?"
		args = append(args, f.MinMemoryGB)
	}
	if f.MinWidth > 0 {
		q += " AND CAST(JSON_UNQUOTE(JSON_EXTRACT(s.specs,'$.width')) AS UNSIGNED)>=?"
		args = append(args, f.MinWidth)
	}
	if f.MinRefreshHz > 0 {
		q += " AND CAST(JSON_UNQUOTE(JSON_EXTRACT(s.specs,'$.refresh_hz')) AS UNSIGNED)>=?"
		args = append(args, f.MinRefreshHz)
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 30
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	q += " ORDER BY s.price_cents ASC,s.id ASC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)
	rows, err := a.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SKU{}
	for rows.Next() {
		var s domain.SKU
		var raw []byte
		var active bool
		if err := rows.Scan(&s.ID, &s.ProductID, &s.Category, &s.Brand, &s.Title, &s.Label, &s.Description, &s.ImageURL, &s.SourceKind, &s.SourceURL, &raw, &s.PriceCents, &s.PriceVersion, &s.Available, &active); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &s.Specs); err != nil {
			return nil, err
		}
		s.Active = active
		s.ObservedAt = time.Now().UTC().Format(time.RFC3339)
		out = append(out, s)
	}
	return out, rows.Err()
}
func (a *Server) findSKU(ctx context.Context, id int64) (domain.SKU, error) {
	var s domain.SKU
	var raw []byte
	err := a.DB.QueryRowContext(ctx, `SELECT s.id,s.product_id,p.category,p.brand,p.title,s.label,p.description,p.image_url,p.source_kind,p.source_url,s.specs,s.price_cents,s.price_version,s.available,s.active AND p.active FROM skus s JOIN products p ON p.id=s.product_id WHERE s.id=?`, id).Scan(&s.ID, &s.ProductID, &s.Category, &s.Brand, &s.Title, &s.Label, &s.Description, &s.ImageURL, &s.SourceKind, &s.SourceURL, &raw, &s.PriceCents, &s.PriceVersion, &s.Available, &s.Active)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s.Specs)
	s.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	return s, err
}
func (a *Server) products(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := searchFilter{Category: q.Get("category"), Brand: q.Get("brand"), Keyword: q.Get("q"), InStock: q.Get("in_stock") == "true"}
	f.MinPrice, _ = strconv.ParseInt(q.Get("min_price_cents"), 10, 64)
	f.MaxPrice, _ = strconv.ParseInt(q.Get("max_price_cents"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	f.Offset, _ = strconv.Atoi(q.Get("offset"))
	f.MinMemoryGB, _ = strconv.Atoi(q.Get("min_memory_gb"))
	f.MinWidth, _ = strconv.Atoi(q.Get("min_width"))
	f.MinRefreshHz, _ = strconv.Atoi(q.Get("min_refresh_hz"))
	items, err := a.searchSKUs(r.Context(), f)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"items": items, "limit": f.Limit, "offset": f.Offset, "demo_data": true})
}
func (a *Server) sku(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	s, err := a.findSKU(r.Context(), id)
	if err == sql.ErrNoRows || !s.Active {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, s)
}

func (a *Server) compare(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Query().Get("sku_ids"), ",")
	if len(parts) < 2 || len(parts) > 4 {
		fail(w, 400, "invalid_input", "请选择 2 到 4 个 SKU")
		return
	}
	items := []domain.SKU{}
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			fail(w, 400, "invalid_id", "")
			return
		}
		s, err := a.findSKU(r.Context(), id)
		if err != nil || !s.Active {
			fail(w, 404, "sku_unavailable", "")
			return
		}
		items = append(items, s)
	}
	respond(w, 200, map[string]any{"items": items, "compatibility": domain.CheckCompatibility(items)})
}

type productInput struct {
	Category    string `json:"category"`
	Brand       string `json:"brand"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
	SourceKind  string `json:"source_kind"`
	SourceURL   string `json:"source_url"`
}

func (a *Server) createProduct(w http.ResponseWriter, r *http.Request) {
	var in productInput
	if readJSON(r, &in) != nil || in.Brand == "" || in.Title == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	if in.Category != "laptop" && in.Category != "monitor" && in.Category != "dock" {
		fail(w, 400, "invalid_category", "")
		return
	}
	if in.SourceKind == "" {
		in.SourceKind = "demo"
	}
	res, err := a.DB.ExecContext(r.Context(), "INSERT INTO products(category,brand,title,description,image_url,source_kind,source_url,source_captured_at) VALUES(?,?,?,?,?,?,?,?)", in.Category, in.Brand, in.Title, in.Description, in.ImageURL, in.SourceKind, in.SourceURL, time.Now().UTC())
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	id, _ := res.LastInsertId()
	respond(w, 201, map[string]any{"id": id})
}
func (a *Server) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	var in struct {
		Brand       string `json:"brand"`
		Title       string `json:"title"`
		Description string `json:"description"`
		ImageURL    string `json:"image_url"`
		Active      bool   `json:"active"`
	}
	if readJSON(r, &in) != nil || in.Brand == "" || in.Title == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "UPDATE products SET brand=?,title=?,description=?,image_url=?,active=? WHERE id=?", in.Brand, in.Title, in.Description, in.ImageURL, in.Active, id)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "not_found", "")
		return
	}
	respond(w, 200, map[string]any{"id": id, "active": in.Active})
}

type skuInput struct {
	ProductID  int64        `json:"product_id"`
	Label      string       `json:"label"`
	Specs      domain.Specs `json:"specs"`
	PriceCents int64        `json:"price_cents"`
	Available  int          `json:"available"`
}

func (a *Server) createSKU(w http.ResponseWriter, r *http.Request) {
	var in skuInput
	if readJSON(r, &in) != nil || in.ProductID < 1 || in.Label == "" || in.PriceCents < 0 || in.Available < 0 {
		fail(w, 400, "invalid_input", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "INSERT INTO skus(product_id,label,specs,price_cents,available) VALUES(?,?,?,?,?)", in.ProductID, in.Label, domain.Encode(in.Specs), in.PriceCents, in.Available)
	if err != nil {
		logErr(r, err)
		fail(w, 400, "invalid_product", "")
		return
	}
	id, _ := res.LastInsertId()
	respond(w, 201, map[string]any{"id": id})
}
func (a *Server) updateSKU(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	var in struct {
		Label  string       `json:"label"`
		Specs  domain.Specs `json:"specs"`
		Active bool         `json:"active"`
	}
	if readJSON(r, &in) != nil || in.Label == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "UPDATE skus SET label=?,specs=?,active=? WHERE id=?", in.Label, domain.Encode(in.Specs), in.Active, id)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "not_found", "")
		return
	}
	respond(w, 200, map[string]any{"id": id, "active": in.Active})
}
func (a *Server) updatePrice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	var in struct {
		PriceCents int64  `json:"price_cents"`
		Reason     string `json:"reason"`
	}
	if readJSON(r, &in) != nil || in.PriceCents < 0 || strings.TrimSpace(in.Reason) == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer tx.Rollback()
	var old, version int64
	if err = tx.QueryRowContext(r.Context(), "SELECT price_cents,price_version FROM skus WHERE id=? FOR UPDATE", id).Scan(&old, &version); err != nil {
		fail(w, 404, "not_found", "")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE skus SET price_cents=?,price_version=price_version+1 WHERE id=?", in.PriceCents, id); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO price_history(sku_id,old_price_cents,new_price_cents,price_version,actor_user_id,reason) VALUES(?,?,?,?,?,?)", id, old, in.PriceCents, version+1, current(r).UserID, in.Reason); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"price_version": version + 1})
}
func (a *Server) adjustStock(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	var in struct {
		Delta     int    `json:"delta"`
		Reason    string `json:"reason"`
		RequestID string `json:"request_id"`
	}
	if readJSON(r, &in) != nil || in.Delta == 0 || strings.TrimSpace(in.Reason) == "" || in.RequestID == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer tx.Rollback()
	var prior int
	err = tx.QueryRowContext(r.Context(), "SELECT delta_available FROM stock_movements WHERE event_key=?", "admin:"+in.RequestID).Scan(&prior)
	if err == nil {
		respond(w, 200, map[string]any{"already_applied": true})
		return
	}
	if err != sql.ErrNoRows {
		fail(w, 500, "internal", "")
		return
	}
	var available int
	if err = tx.QueryRowContext(r.Context(), "SELECT available FROM skus WHERE id=? FOR UPDATE", id).Scan(&available); err != nil {
		fail(w, 404, "not_found", "")
		return
	}
	if available+in.Delta < 0 {
		fail(w, 409, "insufficient_available", "调整后可售库存不能为负")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE skus SET available=available+?,stock_version=stock_version+1 WHERE id=?", in.Delta, id); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO stock_movements(sku_id,event_key,delta_available,delta_reserved,delta_sold,reason,actor_user_id) VALUES(?,?,?,?,?,?,?)", id, "admin:"+in.RequestID, in.Delta, 0, 0, in.Reason, current(r).UserID); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"available": available + in.Delta})
}

func expectCategory(v string) error {
	if v != "laptop" && v != "monitor" && v != "dock" {
		return fmt.Errorf("invalid category")
	}
	return nil
}
