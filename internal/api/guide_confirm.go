package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"goguide/internal/domain"
)

func (a *Server) confirmProposal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProposalVersion     int   `json:"proposal_version"`
		ExpectedAmountCents int64 `json:"expected_amount_cents"`
		Items               []struct {
			SKUID    int64 `json:"sku_id"`
			Quantity int   `json:"quantity"`
		} `json:"items"`
	}
	if readJSON(r, &in) != nil || in.ProposalVersion < 1 || len(in.Items) == 0 {
		fail(w, 400, "invalid_input", "")
		return
	}
	id := pathString(r)
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer tx.Rollback()
	var sid string
	var intentVersion, proposalVersion int
	var raw []byte
	var amount int64
	var confirmed sql.NullTime
	err = tx.QueryRowContext(r.Context(), "SELECT session_id,intent_version,proposal_version,items_json,amount_cents,confirmed_at FROM guide_proposals WHERE id=? AND user_id=? FOR UPDATE", id, current(r).UserID).Scan(&sid, &intentVersion, &proposalVersion, &raw, &amount, &confirmed)
	if err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	if proposalVersion != in.ProposalVersion || amount != in.ExpectedAmountCents {
		fail(w, 409, "confirmation_mismatch", "确认对象与方案不一致")
		return
	}
	var items []proposalItem
	if json.Unmarshal(raw, &items) != nil || len(items) != len(in.Items) {
		fail(w, 409, "confirmation_mismatch", "")
		return
	}
	confirmedItems := map[int64]int{}
	for _, it := range in.Items {
		confirmedItems[it.SKUID] = it.Quantity
	}
	for _, it := range items {
		if confirmedItems[it.SKUID] != it.Quantity {
			fail(w, 409, "confirmation_mismatch", "")
			return
		}
	}
	var activeVersion int
	err = tx.QueryRowContext(r.Context(), "SELECT intent_version FROM guide_sessions WHERE id=? AND user_id=? FOR UPDATE", sid, current(r).UserID).Scan(&activeVersion)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if activeVersion != intentVersion {
		fail(w, 409, "stale_proposal", "方案已更新，请确认最新版本")
		return
	}
	if confirmed.Valid {
		respond(w, 200, map[string]any{"confirmed": true, "idempotent": true, "proposal_id": id})
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SKUID < items[j].SKUID })
	currentSKUs := []domain.SKU{}
	var latestAmount int64
	for _, it := range items {
		var s domain.SKU
		var specs []byte
		err = tx.QueryRowContext(r.Context(), `SELECT s.id,s.product_id,p.category,p.brand,p.title,s.label,s.specs,s.price_cents,s.price_version,s.available,(s.active AND p.active) FROM skus s JOIN products p ON p.id=s.product_id WHERE s.id=? FOR UPDATE`, it.SKUID).Scan(&s.ID, &s.ProductID, &s.Category, &s.Brand, &s.Title, &s.Label, &specs, &s.PriceCents, &s.PriceVersion, &s.Available, &s.Active)
		if err != nil || !s.Active || s.Available < it.Quantity {
			fail(w, 409, "sku_changed", "商品已下架或库存不足，请重规划")
			return
		}
		if s.PriceCents != it.UnitPriceCents || s.PriceVersion != it.PriceVersion {
			fail(w, 409, "price_changed", "商品价格已变化，请重规划")
			return
		}
		if json.Unmarshal(specs, &s.Specs) != nil {
			fail(w, 500, "invalid_specs", "")
			return
		}
		latestAmount += s.PriceCents * int64(it.Quantity)
		currentSKUs = append(currentSKUs, s)
	}
	if latestAmount != amount || domain.CheckCompatibility(currentSKUs).Status != domain.Supported {
		fail(w, 409, "proposal_changed", "组合已无法通过校验，请重规划")
		return
	}
	for _, it := range items {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO cart_items(user_id,sku_id,quantity,selected) VALUES(?,?,?,TRUE) ON DUPLICATE KEY UPDATE quantity=quantity+VALUES(quantity),selected=TRUE", current(r).UserID, it.SKUID, it.Quantity)
		if err != nil {
			logErr(r, err)
			fail(w, 500, "internal", "")
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), "UPDATE guide_proposals SET confirmed_at=? WHERE id=?", time.Now().UTC(), id)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"confirmed": true, "proposal_id": id, "added_items": items, "next": "请前往购物车并单独预览、确认订单"})
}
