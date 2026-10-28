package api

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"goguide/internal/domain"
)

func TestLockedSKUCannotBypassIntentConstraints(t *testing.T) {
	db := integrationDB(t)
	tag := uuid.NewString()
	user, err := db.Exec("INSERT INTO users(email,password_hash) VALUES(?, 'test')", tag+"@test.local")
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := user.LastInsertId()
	sid := uuid.NewString()
	if _, err = db.Exec("INSERT INTO guide_sessions(id,user_id,intent_json) VALUES(?,?,?)", sid, uid, `{}`); err != nil {
		t.Fatal(err)
	}
	product, err := db.Exec("INSERT INTO products(category,brand,title,description) VALUES('gpu','七彩虹',?,'test')", "GeForce RTX 4060 "+tag)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := product.LastInsertId()
	sku, err := db.Exec("INSERT INTO skus(product_id,label,specs,price_cents,available) VALUES(?,'8GB',?,100000,2)", pid, `{"gpu_vram_gb":8}`)
	if err != nil {
		t.Fatal(err)
	}
	skuID, _ := sku.LastInsertId()
	a := New(db, "test-secret-only")
	intent := domain.Intent{BudgetCents: 200000, Categories: []string{"gpu"}, GPUModel: "RTX 4060", MinGPUVRAMGB: 12, LockedSKUs: []int64{skuID}}
	if _, err = a.buildProposals(context.Background(), sid, uid, 1, intent); err == nil || !strings.Contains(err.Error(), "conflicts with current shopping requirements") {
		t.Fatalf("locked 8GB GPU bypassed 12GB requirement: %v", err)
	}
	intent.MinGPUVRAMGB = 8
	intent.GPUModel = "RTX 4070 Ti"
	if _, err = a.buildProposals(context.Background(), sid, uid, 1, intent); err == nil || !strings.Contains(err.Error(), "conflicts with current shopping requirements") {
		t.Fatalf("locked RTX 4060 bypassed model requirement: %v", err)
	}
	intent.GPUModel = "RTX 4060"
	plans, err := a.buildProposals(context.Background(), sid, uid, 1, intent)
	if err != nil || len(plans) != 1 || plans[0].Items[0].SKUID != skuID {
		t.Fatalf("valid lock rejected: plans=%+v err=%v", plans, err)
	}
}
