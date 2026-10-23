package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

func TestPriceWatchVersionIdempotencyAndOwnership(t *testing.T) {
	db := integrationDB(t)
	owner, _, skus := orderFixture(t, db, 1)
	other, _, _ := orderFixture(t, db, 1)
	a := New(db, "test-secret-only")
	body, _ := json.Marshal(map[string]int64{"sku_id": skus[0], "target_price_cents": 10000})
	req := httptest.NewRequest(http.MethodPost, "/v1/price-watches", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), authKey{}, &userClaims{UserID: owner}))
	rec := httptest.NewRecorder()
	a.upsertPriceWatch(rec, req)
	if rec.Code != 200 {
		t.Fatalf("create watch: %d %s", rec.Code, rec.Body.String())
	}
	for i := 0; i < 2; i++ {
		if _, err := CheckPriceWatches(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	var watchID int64
	if err := db.QueryRow("SELECT id FROM price_watches WHERE user_id=? AND sku_id=?", owner, skus[0]).Scan(&watchID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM price_watch_events WHERE watch_id=?", watchID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("first event count=%d err=%v", count, err)
	}
	if _, err := db.Exec("UPDATE skus SET price_cents=9000,price_version=price_version+1 WHERE id=?", skus[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckPriceWatches(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM price_watch_events WHERE watch_id=?", watchID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("second version count=%d err=%v", count, err)
	}
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/price-watches/"+strconv.FormatInt(watchID, 10), nil)
	deleteReq = pathvar.WithVars(deleteReq, map[string]string{"id": strconv.FormatInt(watchID, 10)})
	deleteReq = deleteReq.WithContext(context.WithValue(deleteReq.Context(), authKey{}, &userClaims{UserID: other}))
	otherRec := httptest.NewRecorder()
	a.deletePriceWatch(otherRec, deleteReq)
	if otherRec.Code != 404 {
		t.Fatalf("other user closed watch: %d", otherRec.Code)
	}
	listReq := httptest.NewRequest(http.MethodGet, "/v1/price-watch-events", nil)
	listReq = listReq.WithContext(context.WithValue(listReq.Context(), authKey{}, &userClaims{UserID: other}))
	listRec := httptest.NewRecorder()
	a.listPriceWatchEvents(listRec, listReq)
	if listRec.Code != 200 || !bytes.Contains(listRec.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatalf("other user events: %d %s", listRec.Code, listRec.Body.String())
	}
}
