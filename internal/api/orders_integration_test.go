package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GOGUIDE_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOGUIDE_TEST_DSN")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func orderFixture(t *testing.T, db *sql.DB, skuCount int) (int64, string, []int64) {
	t.Helper()
	tag := uuid.NewString()
	u, err := db.Exec("INSERT INTO users(email,password_hash) VALUES(?,?)", tag+"@test.local", "test")
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := u.LastInsertId()
	addr, err := db.Exec("INSERT INTO addresses(user_id,recipient,phone,detail) VALUES(?,'Test','000','test')", uid)
	if err != nil {
		t.Fatal(err)
	}
	aid, _ := addr.LastInsertId()
	prod, err := db.Exec("INSERT INTO products(category,brand,title,description) VALUES('laptop','Test',?,'test')", tag)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := prod.LastInsertId()
	skus := []int64{}
	items := []previewItem{}
	for i := 0; i < skuCount; i++ {
		res, err := db.Exec("INSERT INTO skus(product_id,label,specs,price_cents,available) VALUES(?,'test','{}',10000,1)", pid)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		skus = append(skus, id)
		items = append(items, previewItem{SKUID: id, Quantity: 1, Title: tag, Label: "test", Specs: json.RawMessage(`{}`), UnitPriceCents: 10000, PriceVersion: 1, Available: 1})
	}
	previewID := uuid.NewString()
	_, err = db.Exec("INSERT INTO checkout_previews(id,user_id,address_id,items_snapshot,address_snapshot,amount_cents,expires_at) VALUES(?,?,?,?,?,?,DATE_ADD(NOW(6),INTERVAL 5 MINUTE))", previewID, uid, aid, mustJSON(items), `{"recipient":"Test","phone":"000","detail":"test"}`, 10000*skuCount)
	if err != nil {
		t.Fatal(err)
	}
	return uid, previewID, skus
}
func submitForTest(a *Server, uid int64, previewID, requestID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"preview_id": previewID, "request_id": requestID})
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), authKey{}, &userClaims{UserID: uid, Role: "user"}))
	rec := httptest.NewRecorder()
	a.submitOrder(rec, req)
	return rec
}
func stockForTest(t *testing.T, db *sql.DB, skuID int64) (int, int) {
	t.Helper()
	var available, reserved int
	if err := db.QueryRow("SELECT available,reserved FROM skus WHERE id=?", skuID).Scan(&available, &reserved); err != nil {
		t.Fatal(err)
	}
	return available, reserved
}

func TestConcurrentLastStockAndIdempotency(t *testing.T) {
	db := integrationDB(t)
	uid, previewID, ids := orderFixture(t, db, 1)
	a := New(db, "test-secret-only")
	var wg sync.WaitGroup
	responses := make([]*httptest.ResponseRecorder, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			responses[i] = submitForTest(a, uid, previewID, fmt.Sprintf("request-%d-%s", i, previewID))
		}()
	}
	wg.Wait()
	success, conflict := 0, 0
	var successfulRequest string
	for i, r := range responses {
		switch r.Code {
		case 201:
			success++
			successfulRequest = fmt.Sprintf("request-%d-%s", i, previewID)
		case 409:
			conflict++
		default:
			t.Fatalf("unexpected status %d: %s", r.Code, r.Body.String())
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	available, reserved := stockForTest(t, db, ids[0])
	if available != 0 || reserved != 1 {
		t.Fatalf("stock %d/%d", available, reserved)
	}
	retry := submitForTest(a, uid, previewID, successfulRequest)
	if retry.Code != 200 {
		t.Fatalf("idempotent retry %d: %s", retry.Code, retry.Body.String())
	}
	available, reserved = stockForTest(t, db, ids[0])
	if available != 0 || reserved != 1 {
		t.Fatalf("retry changed stock %d/%d", available, reserved)
	}
}

func TestMultiSKUFailureRollsBack(t *testing.T) {
	db := integrationDB(t)
	uid, previewID, ids := orderFixture(t, db, 2)
	if _, err := db.Exec("UPDATE skus SET available=0 WHERE id=?", ids[1]); err != nil {
		t.Fatal(err)
	}
	rec := submitForTest(New(db, "test-secret-only"), uid, previewID, uuid.NewString())
	if rec.Code != 409 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	available, reserved := stockForTest(t, db, ids[0])
	if available != 1 || reserved != 0 {
		t.Fatalf("partial reserve %d/%d", available, reserved)
	}
}

func TestPriceChangeNeedsNewPreview(t *testing.T) {
	db := integrationDB(t)
	uid, previewID, ids := orderFixture(t, db, 1)
	if _, err := db.Exec("UPDATE skus SET price_cents=11000,price_version=price_version+1 WHERE id=?", ids[0]); err != nil {
		t.Fatal(err)
	}
	rec := submitForTest(New(db, "test-secret-only"), uid, previewID, uuid.NewString())
	if rec.Code != 409 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	available, reserved := stockForTest(t, db, ids[0])
	if available != 1 || reserved != 0 {
		t.Fatalf("price change reserved stock %d/%d", available, reserved)
	}
}
