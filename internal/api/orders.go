package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"goguide/internal/trade"
)

type previewItem struct {
	SKUID          int64           `json:"sku_id"`
	Quantity       int             `json:"quantity"`
	Title          string          `json:"title"`
	Label          string          `json:"label"`
	Specs          json.RawMessage `json:"specs"`
	UnitPriceCents int64           `json:"unit_price_cents"`
	PriceVersion   int64           `json:"price_version"`
	Available      int             `json:"available"`
}
type addressSnapshot struct {
	Recipient string `json:"recipient"`
	Phone     string `json:"phone"`
	Detail    string `json:"detail"`
}

func (a *Server) preview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AddressID int64 `json:"address_id"`
	}
	if readJSON(r, &in) != nil || in.AddressID < 1 {
		fail(w, 400, "invalid_input", "")
		return
	}
	var addr addressSnapshot
	err := a.DB.QueryRowContext(r.Context(), "SELECT recipient,phone,detail FROM addresses WHERE id=? AND user_id=?", in.AddressID, current(r).UserID).Scan(&addr.Recipient, &addr.Phone, &addr.Detail)
	if err == sql.ErrNoRows {
		fail(w, 404, "address_not_found", "")
		return
	}
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT c.sku_id,c.quantity,p.title,s.label,s.specs,s.price_cents,s.price_version,s.available,(s.active AND p.active) FROM cart_items c JOIN skus s ON s.id=c.sku_id JOIN products p ON p.id=s.product_id WHERE c.user_id=? AND c.selected=TRUE ORDER BY c.sku_id`, current(r).UserID)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	items := []previewItem{}
	var amount int64
	for rows.Next() {
		var it previewItem
		var active bool
		if err = rows.Scan(&it.SKUID, &it.Quantity, &it.Title, &it.Label, &it.Specs, &it.UnitPriceCents, &it.PriceVersion, &it.Available, &active); err != nil {
			break
		}
		if !active || it.Available < it.Quantity {
			err = fmt.Errorf("sku %d unavailable", it.SKUID)
			break
		}
		amount += it.UnitPriceCents * int64(it.Quantity)
		items = append(items, it)
	}
	if e := rows.Err(); e != nil {
		err = e
	}
	rows.Close()
	if err != nil {
		fail(w, 409, "cart_changed", err.Error())
		return
	}
	if len(items) == 0 {
		fail(w, 400, "empty_cart", "请先选择购物车商品")
		return
	}
	id := uuid.NewString()
	_, err = a.DB.ExecContext(r.Context(), "INSERT INTO checkout_previews(id,user_id,address_id,items_snapshot,address_snapshot,amount_cents,expires_at) VALUES(?,?,?,?,?,?,DATE_ADD(NOW(6),INTERVAL 5 MINUTE))", id, current(r).UserID, in.AddressID, mustJSON(items), mustJSON(addr), amount)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	var expires time.Time
	if err = a.DB.QueryRowContext(r.Context(), "SELECT expires_at FROM checkout_previews WHERE id=?", id).Scan(&expires); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"preview_id": id, "items": items, "address": addr, "amount_cents": amount, "expires_at": expires})
}

func (a *Server) submitOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PreviewID string `json:"preview_id"`
		RequestID string `json:"request_id"`
	}
	if readJSON(r, &in) != nil || in.PreviewID == "" || in.RequestID == "" || len(in.RequestID) > 100 {
		fail(w, 400, "invalid_input", "")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer tx.Rollback()
	var itemRaw, addrRaw []byte
	var previewAmount int64
	var addressID int64
	var previewExpired bool
	err = tx.QueryRowContext(r.Context(), "SELECT items_snapshot,address_snapshot,amount_cents,address_id,expires_at<=NOW(6) FROM checkout_previews WHERE id=? AND user_id=?", in.PreviewID, current(r).UserID).Scan(&itemRaw, &addrRaw, &previewAmount, &addressID, &previewExpired)
	if err == sql.ErrNoRows {
		fail(w, 404, "preview_not_found", "")
		return
	}
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	if previewExpired {
		fail(w, 409, "preview_expired", "请重新预览订单")
		return
	}
	var items []previewItem
	if json.Unmarshal(itemRaw, &items) != nil || len(items) == 0 {
		fail(w, 500, "invalid_preview", "")
		return
	}
	var addressExists int
	err = tx.QueryRowContext(r.Context(), "SELECT 1 FROM addresses WHERE id=? AND user_id=?", addressID, current(r).UserID).Scan(&addressExists)
	if err != nil {
		fail(w, 409, "address_changed", "收货地址已失效")
		return
	}
	orderID := uuid.NewString()
	_, err = tx.ExecContext(r.Context(), "INSERT INTO orders(id,user_id,request_id,preview_id,status,amount_cents,address_snapshot,expires_at) VALUES(?,?,?,?,'PENDING_PAYMENT',?,?,DATE_ADD(NOW(6),INTERVAL 15 MINUTE))", orderID, current(r).UserID, in.RequestID, in.PreviewID, previewAmount, addrRaw)
	if err != nil {
		tx.Rollback()
		var existing string
		lookup := a.DB.QueryRowContext(r.Context(), "SELECT id FROM orders WHERE user_id=? AND request_id=?", current(r).UserID, in.RequestID).Scan(&existing)
		if lookup == nil {
			respond(w, 200, map[string]any{"order_id": existing, "idempotent": true})
			return
		}
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SKUID < items[j].SKUID })
	var actualAmount int64
	for _, it := range items {
		var price, version int64
		var available int
		var title, label string
		var specs []byte
		var active bool
		err = tx.QueryRowContext(r.Context(), `SELECT s.price_cents,s.price_version,s.available,p.title,s.label,s.specs,(s.active AND p.active) FROM skus s JOIN products p ON p.id=s.product_id WHERE s.id=? FOR UPDATE`, it.SKUID).Scan(&price, &version, &available, &title, &label, &specs, &active)
		if err != nil || !active {
			fail(w, 409, "sku_unavailable", fmt.Sprintf("SKU %d 已下架或不存在", it.SKUID))
			return
		}
		if price != it.UnitPriceCents || version != it.PriceVersion {
			fail(w, 409, "price_changed", fmt.Sprintf("SKU %d 价格已变化：%d → %d 分，请重新预览", it.SKUID, it.UnitPriceCents, price))
			return
		}
		if available < it.Quantity {
			fail(w, 409, "out_of_stock", fmt.Sprintf("SKU %d 库存不足", it.SKUID))
			return
		}
		actualAmount += price * int64(it.Quantity)
		_, err = tx.ExecContext(r.Context(), "UPDATE skus SET available=available-?,reserved=reserved+?,stock_version=stock_version+1 WHERE id=? AND available>=?", it.Quantity, it.Quantity, it.SKUID, it.Quantity)
		if err != nil {
			logErr(r, err)
			fail(w, 500, "internal", "")
			return
		}
		_, err = tx.ExecContext(r.Context(), "INSERT INTO order_items(order_id,sku_id,product_title,sku_label,specs_snapshot,unit_price_cents,price_version,quantity) VALUES(?,?,?,?,?,?,?,?)", orderID, it.SKUID, title, label, specs, price, version, it.Quantity)
		if err != nil {
			logErr(r, err)
			fail(w, 500, "internal", "")
			return
		}
		_, err = tx.ExecContext(r.Context(), "INSERT INTO stock_movements(sku_id,order_id,event_key,delta_available,delta_reserved,delta_sold,reason) VALUES(?,?,?,?,?,?,?)", it.SKUID, orderID, fmt.Sprintf("reserve:%s:%d", orderID, it.SKUID), -it.Quantity, it.Quantity, 0, "order reserve")
		if err != nil {
			logErr(r, err)
			fail(w, 500, "internal", "")
			return
		}
	}
	if actualAmount != previewAmount {
		fail(w, 409, "amount_changed", "订单金额变化，请重新预览")
		return
	}
	for _, it := range items {
		if _, err = tx.ExecContext(r.Context(), "DELETE FROM cart_items WHERE user_id=? AND sku_id=? AND quantity=?", current(r).UserID, it.SKUID, it.Quantity); err != nil {
			logErr(r, err)
			fail(w, 500, "internal", "")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 201, map[string]any{"order_id": orderID, "status": "PENDING_PAYMENT", "amount_cents": actualAmount})
}

func (a *Server) orders(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), "SELECT id,status,amount_cents,expires_at,created_at FROM orders WHERE user_id=? ORDER BY created_at DESC LIMIT 100", current(r).UserID)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, status string
		var amount int64
		var expiry, created time.Time
		if rows.Scan(&id, &status, &amount, &expiry, &created) == nil {
			list = append(list, map[string]any{"id": id, "status": status, "amount_cents": amount, "expires_at": expiry, "created_at": created})
		}
	}
	respond(w, 200, map[string]any{"items": list})
}
func (a *Server) order(w http.ResponseWriter, r *http.Request) {
	id := pathString(r)
	var status string
	var amount int64
	var address []byte
	var expiry, created time.Time
	err := a.DB.QueryRowContext(r.Context(), "SELECT status,amount_cents,address_snapshot,expires_at,created_at FROM orders WHERE id=? AND user_id=?", id, current(r).UserID).Scan(&status, &amount, &address, &expiry, &created)
	if err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), "SELECT sku_id,product_title,sku_label,unit_price_cents,quantity FROM order_items WHERE order_id=? ORDER BY sku_id", id)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var sku, price int64
		var title, label string
		var qty int
		if rows.Scan(&sku, &title, &label, &price, &qty) == nil {
			items = append(items, map[string]any{"sku_id": sku, "title": title, "label": label, "unit_price_cents": price, "quantity": qty})
		}
	}
	respond(w, 200, map[string]any{"id": id, "status": status, "amount_cents": amount, "address": json.RawMessage(address), "items": items, "expires_at": expiry, "created_at": created})
}
func (a *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	status, err := trade.Close(r.Context(), a.DB, pathString(r), "cancel", current(r).UserID)
	if err == trade.ErrNotFound {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"status": status})
}
func (a *Server) mockPay(w http.ResponseWriter, r *http.Request) {
	var amount int64
	orderID := pathString(r)
	err := a.DB.QueryRowContext(r.Context(), "SELECT amount_cents FROM orders WHERE id=? AND user_id=?", orderID, current(r).UserID).Scan(&amount)
	if err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	cb := trade.Callback{EventID: orderID, OrderID: orderID, AmountCents: amount, Outcome: "SUCCESS"}
	cb.Signature = trade.SignCallback(a.PaymentSecret, cb)
	status, err := trade.ProcessCallback(r.Context(), a.DB, a.PaymentSecret, cb)
	writePaymentResult(w, r, status, err)
}
func (a *Server) paymentCallback(w http.ResponseWriter, r *http.Request) {
	var cb trade.Callback
	if readJSON(r, &cb) != nil {
		fail(w, 400, "invalid_input", "")
		return
	}
	status, err := trade.ProcessCallback(r.Context(), a.DB, a.PaymentSecret, cb)
	writePaymentResult(w, r, status, err)
}
func writePaymentResult(w http.ResponseWriter, r *http.Request, status string, err error) {
	if errors.Is(err, trade.ErrNotFound) {
		fail(w, 404, "not_found", "")
		return
	}
	if errors.Is(err, trade.ErrConflict) {
		fail(w, 409, "order_not_payable", status)
		return
	}
	if errors.Is(err, trade.ErrInvalidCallback) {
		fail(w, 401, "invalid_callback", "支付事件验证失败")
		return
	}
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"status": status, "mock": true})
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
