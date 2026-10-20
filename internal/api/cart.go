package api

import (
	"database/sql"
	"net/http"
)

type cartItem struct {
	SKUID        int64  `json:"sku_id"`
	Quantity     int    `json:"quantity"`
	Selected     bool   `json:"selected"`
	Title        string `json:"title"`
	Label        string `json:"label"`
	PriceCents   int64  `json:"price_cents"`
	PriceVersion int64  `json:"price_version"`
	Available    int    `json:"available"`
	Active       bool   `json:"active"`
	LineCents    int64  `json:"line_cents"`
	Warning      string `json:"warning,omitempty"`
}

func (a *Server) cart(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), `SELECT c.sku_id,c.quantity,c.selected,p.title,s.label,s.price_cents,s.price_version,s.available,(s.active AND p.active) FROM cart_items c JOIN skus s ON c.sku_id=s.id JOIN products p ON p.id=s.product_id WHERE c.user_id=? ORDER BY c.updated_at DESC`, current(r).UserID)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	items := []cartItem{}
	var total int64
	for rows.Next() {
		var it cartItem
		if err := rows.Scan(&it.SKUID, &it.Quantity, &it.Selected, &it.Title, &it.Label, &it.PriceCents, &it.PriceVersion, &it.Available, &it.Active); err != nil {
			logErr(r, err)
			fail(w, 500, "internal", "")
			return
		}
		it.LineCents = it.PriceCents * int64(it.Quantity)
		if !it.Active {
			it.Warning = "商品已下架"
		} else if it.Available < it.Quantity {
			it.Warning = "库存不足"
		}
		if it.Selected && it.Warning == "" {
			total += it.LineCents
		}
		items = append(items, it)
	}
	respond(w, 200, map[string]any{"items": items, "selected_estimate_cents": total, "price_notice": "购物车金额仅为预估，下单时重新校验"})
}

func (a *Server) addCart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SKUID    int64 `json:"sku_id"`
		Quantity int   `json:"quantity"`
	}
	if readJSON(r, &in) != nil || in.SKUID < 1 || in.Quantity < 1 || in.Quantity > 99 {
		fail(w, 400, "invalid_input", "")
		return
	}
	s, err := a.findSKU(r.Context(), in.SKUID)
	if err != nil || !s.Active {
		fail(w, 404, "sku_unavailable", "")
		return
	}
	_, err = a.DB.ExecContext(r.Context(), `INSERT INTO cart_items(user_id,sku_id,quantity,selected) VALUES(?,?,?,TRUE) ON DUPLICATE KEY UPDATE quantity=quantity+VALUES(quantity),selected=TRUE`, current(r).UserID, in.SKUID, in.Quantity)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"sku_id": in.SKUID, "added": in.Quantity})
}
func (a *Server) updateCart(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	var in struct {
		Quantity int  `json:"quantity"`
		Selected bool `json:"selected"`
	}
	if readJSON(r, &in) != nil || in.Quantity < 1 || in.Quantity > 99 {
		fail(w, 400, "invalid_input", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "UPDATE cart_items SET quantity=?,selected=? WHERE user_id=? AND sku_id=?", in.Quantity, in.Selected, current(r).UserID, id)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "not_found", "")
		return
	}
	respond(w, 200, map[string]any{"sku_id": id})
}
func (a *Server) deleteCart(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "DELETE FROM cart_items WHERE user_id=? AND sku_id=?", current(r).UserID, id)
	if err != nil && err != sql.ErrNoRows {
		fail(w, 500, "internal", "")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "not_found", "")
		return
	}
	respond(w, 200, map[string]bool{"deleted": true})
}
