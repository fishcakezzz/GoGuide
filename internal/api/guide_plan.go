package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"goguide/internal/domain"
)

type proposalItem struct {
	SKUID          int64  `json:"sku_id"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	PriceVersion   int64  `json:"price_version"`
	Title          string `json:"title"`
	Label          string `json:"label"`
	Category       string `json:"category"`
}
type proposal struct {
	ID              string               `json:"id"`
	Version         int                  `json:"version"`
	IntentVersion   int                  `json:"intent_version"`
	Items           []proposalItem       `json:"items"`
	AmountCents     int64                `json:"amount_cents"`
	BudgetLeftCents int64                `json:"budget_left_cents"`
	Compatibility   domain.Compatibility `json:"compatibility"`
	Evidence        []domain.SKU         `json:"evidence"`
	Reasons         []string             `json:"reasons"`
}

func (a *Server) guideMessage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text            string  `json:"text"`
		ExpectedVersion int     `json:"expected_version"`
		LockedSKUs      []int64 `json:"locked_skus"`
		ReplaceSKUID    int64   `json:"replace_sku_id"`
		WithSKUID       int64   `json:"with_sku_id"`
	}
	if readJSON(r, &in) != nil || strings.TrimSpace(in.Text) == "" || in.ExpectedVersion < 1 || len(in.Text) > 4000 {
		fail(w, 400, "invalid_input", "")
		return
	}
	sid := pathString(r)
	var raw []byte
	var version int
	err := a.DB.QueryRowContext(r.Context(), "SELECT intent_json,intent_version FROM guide_sessions WHERE id=? AND user_id=?", sid, current(r).UserID).Scan(&raw, &version)
	if err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	if version != in.ExpectedVersion {
		fail(w, 409, "intent_version_conflict", "会话已有更新，请刷新")
		return
	}
	var old domain.Intent
	if json.Unmarshal(raw, &old) != nil {
		fail(w, 500, "invalid_intent", "")
		return
	}
	if a.Guide.Model == nil || a.Guide.InitErr != nil {
		fail(w, http.StatusServiceUnavailable, "model_not_configured", "智能导购需要配置 DEEPSEEK_API_KEY，启动 Go API 后才可使用")
		return
	}
	intent, err := a.Guide.extract(r.Context(), in.Text, old)
	if err != nil {
		logErr(r, err)
		fail(w, 503, "model_unavailable", "需求理解失败，请稍后重试")
		return
	}
	if in.LockedSKUs != nil {
		intent.LockedSKUs = in.LockedSKUs
	}
	if in.ReplaceSKUID > 0 && in.WithSKUID > 0 {
		oldSKU, e1 := a.findSKU(r.Context(), in.ReplaceSKUID)
		newSKU, e2 := a.findSKU(r.Context(), in.WithSKUID)
		if e1 != nil || e2 != nil || oldSKU.Category != newSKU.Category || !newSKU.Active {
			fail(w, 400, "invalid_replacement", "替换商品必须为同品类有效 SKU")
			return
		}
		locks := []int64{}
		for _, id := range intent.LockedSKUs {
			if id != in.ReplaceSKUID {
				locks = append(locks, id)
			}
		}
		intent.LockedSKUs = append(locks, in.WithSKUID)
	}
	if len(intent.LockedSKUs) > 3 {
		fail(w, 400, "too_many_locks", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "UPDATE guide_sessions SET intent_json=?,intent_version=intent_version+1 WHERE id=? AND user_id=? AND intent_version=?", domain.Encode(intent), sid, current(r).UserID, version)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		fail(w, 409, "intent_version_conflict", "会话已有更新，请刷新")
		return
	}
	version++
	_, _ = a.DB.ExecContext(r.Context(), "INSERT INTO guide_messages(session_id,user_id,role,content) VALUES(?,?,'user',?)", sid, current(r).UserID, in.Text)
	if intent.BudgetCents == 0 || len(intent.Categories) == 0 {
		question := "请提供总预算（元）和要购买的品类。"
		if intent.BudgetCents > 0 {
			question = "请确认要购买哪些电脑配件或外设。"
		}
		if len(intent.Categories) > 0 {
			question = "请提供总预算（元）。"
		}
		if intent.Questions >= 2 {
			question = "关键预算或购买品类仍未明确，暂时无法给出已验证方案。请直接补充这些条件。"
		} else {
			intent.Questions++
		}
		_, _ = a.DB.ExecContext(r.Context(), "UPDATE guide_sessions SET intent_json=? WHERE id=? AND intent_version=?", domain.Encode(intent), sid, version)
		_, _ = a.DB.ExecContext(r.Context(), "INSERT INTO guide_messages(session_id,user_id,role,content) VALUES(?,?,'assistant',?)", sid, current(r).UserID, question)
		respond(w, 200, map[string]any{"intent": intent, "intent_version": version, "question": question, "proposals": []proposal{}, "mode": a.Guide.Mode})
		return
	}
	if len(intent.Hard) > 0 {
		message := "这些必需条件缺少可核验的结构化参数，暂时不能作为已验证方案：" + strings.Join(intent.Hard, "；")
		_, _ = a.DB.ExecContext(r.Context(), "INSERT INTO guide_messages(session_id,user_id,role,content) VALUES(?,?,'assistant',?)", sid, current(r).UserID, message)
		respond(w, 200, map[string]any{"intent": intent, "intent_version": version, "message": message, "proposals": []proposal{}, "mode": a.Guide.Mode})
		return
	}
	plans, err := a.buildProposals(r.Context(), sid, current(r).UserID, version, intent)
	if err != nil {
		logErr(r, err)
		fail(w, 409, "planning_failed", err.Error())
		return
	}
	message := "已按当前预算、库存和兼容性规则生成方案。价格与库存会在加购和下单时复核。"
	if len(plans) == 0 {
		message = "当前数据中没有满足预算、库存和兼容性硬条件的组合；请调整预算或条件。"
	} else if analysis, err := a.Guide.analyzeProposals(r.Context(), intent, plans); err == nil {
		message = analysis
	} else {
		logErr(r, err)
	}
	_, _ = a.DB.ExecContext(r.Context(), "INSERT INTO guide_messages(session_id,user_id,role,content) VALUES(?,?,'assistant',?)", sid, current(r).UserID, message)
	respond(w, 200, map[string]any{"intent": intent, "intent_version": version, "message": message, "proposals": plans, "mode": a.Guide.Mode})
}

func (a *Server) buildProposals(ctx context.Context, sid string, uid int64, version int, intent domain.Intent) ([]proposal, error) {
	categories := []string{}
	for _, cat := range intent.Categories {
		if !hasCategory(categories, cat) {
			categories = append(categories, cat)
		}
	}
	if intent.RequireOneCable && !hasCategory(categories, "dock") {
		categories = append(categories, "dock")
	}
	if len(categories) > 10 {
		return nil, fmt.Errorf("too many categories")
	}
	byCategory := map[string][]domain.SKU{}
	for _, cat := range categories {
		start := time.Now()
		candidates, err := a.searchSKUs(ctx, searchFilter{Category: cat, InStock: true, MaxPrice: intent.BudgetCents, Limit: 30})
		if err != nil {
			return nil, err
		}
		filtered := []domain.SKU{}
		for _, s := range candidates {
			if intent.Brand != "" && (len(categories) == 1 || cat == "laptop") && !strings.EqualFold(intent.Brand, s.Brand) {
				continue
			}
			if (cat == "laptop" || cat == "ram") && s.Specs.MemoryGB < intent.MinMemoryGB {
				continue
			}
			if (cat == "ram" || cat == "motherboard") && intent.MemoryType != "" && !strings.EqualFold(s.Specs.MemoryType, intent.MemoryType) {
				continue
			}
			if cat == "ram" && s.Specs.MemorySpeedMT < intent.MinMemorySpeedMT {
				continue
			}
			if cat == "storage" && s.Specs.StorageGB < intent.MinStorageGB {
				continue
			}
			if cat == "gpu" && s.Specs.GPUVRAMGB < intent.MinGPUVRAMGB {
				continue
			}
			if cat == "monitor" && s.Specs.RefreshHz < intent.MinRefreshHz {
				continue
			}
			filtered = append(filtered, s)
		}
		if len(filtered) > 12 {
			filtered = filtered[:12]
		}
		byCategory[cat] = filtered
		_, _ = a.DB.ExecContext(ctx, "INSERT INTO guide_tool_runs(session_id,tool_name,input_summary,result_summary,duration_ms) VALUES(?,'SearchSKU',?,?,?)", sid, mustJSON(map[string]any{"category": cat, "budget_cents": intent.BudgetCents}), mustJSON(map[string]any{"found": len(filtered)}), time.Since(start).Milliseconds())
	}
	for _, id := range intent.LockedSKUs {
		s, err := a.findSKU(ctx, id)
		if err != nil || !s.Active || s.Available < 1 || !hasCategory(categories, s.Category) {
			return nil, fmt.Errorf("locked SKU %d unavailable or outside requested categories", id)
		}
		byCategory[s.Category] = []domain.SKU{s}
	}
	combos := [][]domain.SKU{{}}
	for _, cat := range categories {
		next := [][]domain.SKU{}
		for _, prefix := range combos {
			for _, s := range byCategory[cat] {
				copyItems := append(append([]domain.SKU{}, prefix...), s)
				next = append(next, copyItems)
			}
		}
		combos = pruneCombos(next, intent.BudgetCents, 1200)
	}
	valid := []proposal{}
	for _, items := range combos {
		var amount int64
		for _, s := range items {
			amount += s.PriceCents
		}
		if amount > intent.BudgetCents {
			continue
		}
		compat := domain.CheckCompatibility(items)
		if compat.Status != domain.Supported {
			continue
		}
		p := proposal{IntentVersion: version, AmountCents: amount, BudgetLeftCents: intent.BudgetCents - amount, Compatibility: compat, Evidence: items}
		for _, s := range items {
			p.Items = append(p.Items, proposalItem{SKUID: s.ID, Quantity: 1, UnitPriceCents: s.PriceCents, PriceVersion: s.PriceVersion, Title: s.Title, Label: s.Label, Category: s.Category})
			var minPrice, maxPrice int64
			for _, alternative := range byCategory[s.Category] {
				if alternative.ProductID != s.ProductID {
					continue
				}
				if minPrice == 0 || alternative.PriceCents < minPrice {
					minPrice = alternative.PriceCents
				}
				if alternative.PriceCents > maxPrice {
					maxPrice = alternative.PriceCents
				}
			}
			if maxPrice > minPrice {
				p.Reasons = append(p.Reasons, fmt.Sprintf("%s 同配置模拟报价范围 ¥%.2f–¥%.2f，本方案选 ¥%.2f", s.Title, float64(minPrice)/100, float64(maxPrice)/100, float64(s.PriceCents)/100))
			}
			switch s.Category {
			case "laptop":
				if s.Specs.MemoryGB > 0 {
					p.Reasons = append(p.Reasons, fmt.Sprintf("%s：%dGB 内存（SKU %d）", s.Title, s.Specs.MemoryGB, s.ID))
				}
			case "monitor":
				if s.Specs.Width > 0 && s.Specs.Height > 0 {
					p.Reasons = append(p.Reasons, fmt.Sprintf("%s：%d×%d、%dHz（SKU %d）", s.Title, s.Specs.Width, s.Specs.Height, s.Specs.RefreshHz, s.ID))
				}
			case "dock":
				if s.Specs.PDOutputW > 0 {
					p.Reasons = append(p.Reasons, fmt.Sprintf("%s：供电 %dW（SKU %d）", s.Title, s.Specs.PDOutputW, s.ID))
				}
			}
		}
		p.Reasons = append(p.Reasons, fmt.Sprintf("组合总价 %d 分，预算剩余 %d 分；%s", amount, p.BudgetLeftCents, strings.Join(compat.Reasons, "；")))
		valid = append(valid, p)
	}
	if len(intent.Soft) > 0 {
		sort.Slice(valid, func(i, j int) bool {
			si, sj := preferenceScore(valid[i], intent), preferenceScore(valid[j], intent)
			if si == sj {
				return valid[i].AmountCents < valid[j].AmountCents
			}
			return si > sj
		})
	} else {
		sort.Slice(valid, func(i, j int) bool { return valid[i].AmountCents < valid[j].AmountCents })
	}
	chosen := []proposal{}
	if len(valid) > 0 {
		indices := []int{0, len(valid) / 2, len(valid) - 1}
		if len(intent.Soft) > 0 {
			indices = []int{0, 1, 2}
		}
		seen := map[int]bool{}
		for _, i := range indices {
			if i < len(valid) && !seen[i] {
				chosen = append(chosen, valid[i])
				seen[i] = true
			}
		}
	}
	for i := range chosen {
		for _, item := range chosen[i].Items {
			var low sql.NullInt64
			if err := a.DB.QueryRowContext(ctx, `SELECT MIN(price_cents) FROM demo_price_snapshots WHERE sku_id=? AND observed_on>=UTC_DATE()-INTERVAL 29 DAY`, item.SKUID).Scan(&low); err != nil {
				return nil, err
			}
			if low.Valid && low.Int64 > 0 {
				chosen[i].Reasons = append(chosen[i].Reasons, fmt.Sprintf("%s 的模拟30天轨迹低点 ¥%.2f，当前模拟报价 ¥%.2f；不代表淘宝历史价格", item.Title, float64(low.Int64)/100, float64(item.UnitPriceCents)/100))
			}
		}
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var currentVersion, maxVersion int
	if err = tx.QueryRowContext(ctx, "SELECT intent_version FROM guide_sessions WHERE id=? AND user_id=? FOR UPDATE", sid, uid).Scan(&currentVersion); err != nil {
		return nil, err
	}
	if currentVersion != version {
		return nil, fmt.Errorf("intent changed during planning")
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(proposal_version),0) FROM guide_proposals WHERE session_id=?", sid).Scan(&maxVersion); err != nil {
		return nil, err
	}
	for i := range chosen {
		chosen[i].ID = uuid.NewString()
		chosen[i].Version = maxVersion + i + 1
		_, err = tx.ExecContext(ctx, "INSERT INTO guide_proposals(id,session_id,user_id,intent_version,proposal_version,items_json,evidence_json,amount_cents) VALUES(?,?,?,?,?,?,?,?)", chosen[i].ID, sid, uid, version, chosen[i].Version, mustJSON(chosen[i].Items), mustJSON(map[string]any{"compatibility": chosen[i].Compatibility, "skus": chosen[i].Evidence, "reasons": chosen[i].Reasons}), chosen[i].AmountCents)
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	_, _ = a.DB.ExecContext(ctx, "INSERT INTO guide_tool_runs(session_id,tool_name,input_summary,result_summary,duration_ms) VALUES(?,'CheckCompatibility',?,?,0)", sid, mustJSON(map[string]any{"combinations": len(combos)}), mustJSON(map[string]any{"valid": len(valid), "returned": len(chosen)}))
	return chosen, nil
}

func pruneCombos(combos [][]domain.SKU, budget int64, limit int) [][]domain.SKU {
	priced := make([][]domain.SKU, 0, len(combos))
	for _, combo := range combos {
		var amount int64
		for _, sku := range combo {
			amount += sku.PriceCents
		}
		if amount <= budget && domain.CheckCompatibility(combo).Status == domain.Supported {
			priced = append(priced, combo)
		}
	}
	if len(priced) <= limit {
		return priced
	}
	sort.Slice(priced, func(i, j int) bool { return comboPrice(priced[i]) < comboPrice(priced[j]) })
	selected := make([][]domain.SKU, 0, limit)
	for i := 0; i < limit; i++ {
		index := i * (len(priced) - 1) / (limit - 1)
		selected = append(selected, priced[index])
	}
	return selected
}

func comboPrice(items []domain.SKU) int64 {
	var total int64
	for _, sku := range items {
		total += sku.PriceCents
	}
	return total
}

func preferenceScore(p proposal, intent domain.Intent) int64 {
	var score int64
	for _, pref := range intent.Soft {
		switch pref {
		case "low_price":
			score += p.BudgetLeftCents / 10
		case "performance":
			for _, s := range p.Evidence {
				if s.Category == "laptop" {
					score += int64(s.Specs.MemoryGB) * 1000
				}
			}
		case "large_display":
			for _, s := range p.Evidence {
				if s.Category == "monitor" {
					score += int64(s.Specs.Width*s.Specs.Height) / 100
				}
			}
		case "lightweight":
			for _, s := range p.Evidence {
				if s.Category == "laptop" && s.Specs.WeightGrams > 0 {
					score += int64(max(0, 3000-s.Specs.WeightGrams)) * 100
				}
			}
		}
	}
	return score
}
