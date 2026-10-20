package trade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GOGUIDE_TEST_DSN")
	if dsn == "" {
		t.Skip("set GOGUIDE_TEST_DSN for MySQL integration tests")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func fixture(t *testing.T, db *sql.DB, expired bool) (string, int64, int64) {
	t.Helper()
	ctx := context.Background()
	tag := uuid.NewString()
	user, err := db.ExecContext(ctx, "INSERT INTO users(email,password_hash) VALUES(?,?)", tag+"@test.local", "test")
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := user.LastInsertId()
	product, err := db.ExecContext(ctx, "INSERT INTO products(category,brand,title,description) VALUES('laptop','Test',?,'integration test')", tag)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := product.LastInsertId()
	sku, err := db.ExecContext(ctx, "INSERT INTO skus(product_id,label,specs,price_cents,available,reserved) VALUES(?,'test','{}',10000,0,1)", pid)
	if err != nil {
		t.Fatal(err)
	}
	skuID, _ := sku.LastInsertId()
	orderID := uuid.NewString()
	interval := 3600
	if expired {
		interval = -1
	}
	_, err = db.ExecContext(ctx, "INSERT INTO orders(id,user_id,request_id,preview_id,status,amount_cents,address_snapshot,expires_at) VALUES(?,?,?,?,'PENDING_PAYMENT',10000,'{}',DATE_ADD(NOW(6),INTERVAL ? SECOND))", orderID, uid, tag, uuid.NewString(), interval)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO order_items(order_id,sku_id,product_title,sku_label,specs_snapshot,unit_price_cents,price_version,quantity) VALUES(?,?,'test','test','{}',10000,1,1)", orderID, skuID)
	if err != nil {
		t.Fatal(err)
	}
	return orderID, uid, skuID
}

func assertStock(t *testing.T, db *sql.DB, skuID int64, wantAvailable, wantReserved, wantSold int) {
	t.Helper()
	var a, r, s int
	if err := db.QueryRow("SELECT available,reserved,sold FROM skus WHERE id=?", skuID).Scan(&a, &r, &s); err != nil {
		t.Fatal(err)
	}
	if a != wantAvailable || r != wantReserved || s != wantSold {
		t.Fatalf("stock got %d/%d/%d want %d/%d/%d", a, r, s, wantAvailable, wantReserved, wantSold)
	}
}

func TestPaymentAndCancellationRace(t *testing.T) {
	db := testDB(t)
	orderID, uid, skuID := fixture(t, db, false)
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	var payStatus, cancelStatus string
	var payErr, cancelErr error
	go func() { defer wg.Done(); payStatus, payErr = Pay(ctx, db, orderID, uid) }()
	go func() { defer wg.Done(); cancelStatus, cancelErr = Close(ctx, db, orderID, "cancel", uid) }()
	wg.Wait()
	if payErr != nil && !errors.Is(payErr, ErrConflict) {
		t.Fatal(payErr)
	}
	if cancelErr != nil {
		t.Fatal(cancelErr)
	}
	var final string
	if err := db.QueryRow("SELECT status FROM orders WHERE id=?", orderID).Scan(&final); err != nil {
		t.Fatal(err)
	}
	switch final {
	case "PAID":
		assertStock(t, db, skuID, 0, 0, 1)
		if payStatus != "PAID" {
			t.Fatalf("pay status %s", payStatus)
		}
	case "CANCELLED":
		assertStock(t, db, skuID, 1, 0, 0)
		if cancelStatus != "CANCELLED" {
			t.Fatalf("cancel status %s", cancelStatus)
		}
	default:
		t.Fatalf("unexpected state %s", final)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM stock_movements WHERE order_id=? AND reason IN ('paid','released')", orderID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one final movement, got %d", n)
	}
}

func TestExpiredPaymentAndRecovery(t *testing.T) {
	db := testDB(t)
	orderID, uid, skuID := fixture(t, db, true)
	status, err := Pay(context.Background(), db, orderID, uid)
	if !errors.Is(err, ErrConflict) || status != "CLOSED" {
		t.Fatalf("pay got %s %v", status, err)
	}
	assertStock(t, db, skuID, 1, 0, 0)
	n, err := CloseDue(context.Background(), db, 50)
	if err != nil {
		t.Fatal(err)
	}
	_ = n
	assertStock(t, db, skuID, 1, 0, 0)
	var anomalies int
	if err := db.QueryRow("SELECT COUNT(*) FROM trade_anomalies WHERE order_id=?", orderID).Scan(&anomalies); err != nil {
		t.Fatal(err)
	}
	if anomalies != 1 {
		t.Fatalf("late anomaly count %d", anomalies)
	}
}

func TestWorkerIdempotentClose(t *testing.T) {
	db := testDB(t)
	orderID, _, skuID := fixture(t, db, true)
	for i := 0; i < 2; i++ {
		if _, err := CloseDue(context.Background(), db, 50); err != nil {
			t.Fatal(err)
		}
	}
	assertStock(t, db, skuID, 1, 0, 0)
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM stock_movements WHERE event_key=?", fmt.Sprintf("released:%s:%d", orderID, skuID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("release count %d", n)
	}
}

func TestSignedCallbackValidation(t *testing.T) {
	db := testDB(t)
	orderID, _, skuID := fixture(t, db, false)
	secret := []byte("integration-payment-secret")
	cb := Callback{EventID: orderID, OrderID: orderID, AmountCents: 10000, Outcome: "SUCCESS"}
	cb.Signature = SignCallback(secret, cb)
	tampered := cb
	tampered.AmountCents = 9999
	if _, err := ProcessCallback(context.Background(), db, secret, tampered); !errors.Is(err, ErrInvalidCallback) {
		t.Fatalf("tampered callback: %v", err)
	}
	assertStock(t, db, skuID, 0, 1, 0)
	status, err := ProcessCallback(context.Background(), db, secret, cb)
	if err != nil || status != "PAID" {
		t.Fatalf("valid callback: %s %v", status, err)
	}
	assertStock(t, db, skuID, 0, 0, 1)
}
