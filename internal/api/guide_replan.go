package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"goguide/internal/domain"
)

type replanSession struct {
	id      string
	userID  int64
	version int
}

// ReconcileProposals detects changed SKU facts, invalidates old proposals, and
// resumes pending replans. It is safe to call repeatedly after worker restarts.
func ReconcileProposals(ctx context.Context, db *sql.DB, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	a := &Server{DB: db, Guide: NewGuide(db)}
	if err := a.resumeReplans(ctx, limit); err != nil {
		return 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT gp.session_id,gp.user_id,gp.intent_version
		FROM guide_proposals gp JOIN guide_sessions gs ON gs.id=gp.session_id
		WHERE gp.status='ACTIVE' AND gp.confirmed_at IS NULL AND gp.intent_version=gs.intent_version`)
	if err != nil {
		return 0, err
	}
	sessions := []replanSession{}
	for rows.Next() {
		var s replanSession
		if err = rows.Scan(&s.id, &s.userID, &s.version); err != nil {
			rows.Close()
			return 0, err
		}
		sessions = append(sessions, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, session := range sessions {
		reason, err := a.proposalChangeReason(ctx, session)
		if err != nil {
			return changed, err
		}
		if reason == "" {
			continue
		}
		updated, err := a.invalidateSession(ctx, session, reason)
		if err != nil {
			return changed, err
		}
		if updated {
			changed++
		}
	}
	if changed > 0 {
		if err := a.resumeReplans(ctx, limit); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func (a *Server) proposalChangeReason(ctx context.Context, s replanSession) (string, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT items_json FROM guide_proposals WHERE session_id=? AND intent_version=? AND status='ACTIVE' AND confirmed_at IS NULL`, s.id, s.version)
	if err != nil {
		return "", err
	}
	sets := [][]proposalItem{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return "", err
		}
		var items []proposalItem
		if err = json.Unmarshal(raw, &items); err != nil {
			rows.Close()
			return "", err
		}
		sets = append(sets, items)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for _, items := range sets {
		for _, item := range items {
			current, err := a.findSKU(ctx, item.SKUID)
			if err == sql.ErrNoRows || (err == nil && (!current.Active || current.Available < item.Quantity)) {
				return fmt.Sprintf("SKU %d 已下架或库存不足", item.SKUID), nil
			}
			if err != nil {
				return "", err
			}
			if current.PriceVersion != item.PriceVersion || current.PriceCents != item.UnitPriceCents {
				return fmt.Sprintf("SKU %d 报价由 ¥%.2f 变化为 ¥%.2f", item.SKUID, float64(item.UnitPriceCents)/100, float64(current.PriceCents)/100), nil
			}
			if item.StockVersion != 0 && current.StockVersion != item.StockVersion {
				return fmt.Sprintf("SKU %d 库存版本已更新", item.SKUID), nil
			}
		}
	}
	return "", nil
}

func (a *Server) invalidateSession(ctx context.Context, s replanSession, reason string) (bool, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "SELECT intent_version FROM guide_sessions WHERE id=? AND user_id=? FOR UPDATE", s.id, s.userID).Scan(&version); err != nil {
		return false, err
	}
	if version != s.version {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, "UPDATE guide_sessions SET intent_version=intent_version+1 WHERE id=? AND intent_version=?", s.id, version); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE guide_proposals SET status='STALE' WHERE session_id=? AND intent_version=? AND confirmed_at IS NULL", s.id, version); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO guide_replans(session_id,user_id,old_intent_version,new_intent_version,reason) VALUES(?,?,?,?,?)", s.id, s.userID, version, version+1, reason); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (a *Server) resumeReplans(ctx context.Context, limit int) error {
	rows, err := a.DB.QueryContext(ctx, "SELECT id,session_id,user_id,new_intent_version,reason FROM guide_replans WHERE status='PENDING' OR (status='PROCESSING' AND locked_until<UTC_TIMESTAMP(6)) ORDER BY id LIMIT ?", limit)
	if err != nil {
		return err
	}
	type job struct {
		id      int64
		session string
		user    int64
		version int
		reason  string
	}
	jobs := []job{}
	for rows.Next() {
		var j job
		if err = rows.Scan(&j.id, &j.session, &j.user, &j.version, &j.reason); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		claimed, claimErr := a.DB.ExecContext(ctx, "UPDATE guide_replans SET status='PROCESSING',locked_until=UTC_TIMESTAMP(6)+INTERVAL 60 SECOND WHERE id=? AND (status='PENDING' OR (status='PROCESSING' AND locked_until<UTC_TIMESTAMP(6)))", j.id)
		if claimErr != nil {
			return claimErr
		}
		affected, _ := claimed.RowsAffected()
		if affected == 0 {
			continue
		}
		var raw []byte
		var version int
		if err = a.DB.QueryRowContext(ctx, "SELECT intent_json,intent_version FROM guide_sessions WHERE id=? AND user_id=?", j.session, j.user).Scan(&raw, &version); err != nil {
			return err
		}
		if version != j.version {
			_, err = a.DB.ExecContext(ctx, "UPDATE guide_replans SET status='FAILED',locked_until=NULL,error_message='会话条件已再次更新' WHERE id=? AND status='PROCESSING'", j.id)
			if err != nil {
				return err
			}
			continue
		}
		var count int
		if err = a.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM guide_proposals WHERE session_id=? AND intent_version=?", j.session, j.version).Scan(&count); err != nil {
			return err
		}
		message := "原方案失效：" + j.reason + "。已重新核算预算与兼容性，请查看最新方案。"
		if count == 0 {
			var intent domain.Intent
			if err = json.Unmarshal(raw, &intent); err != nil {
				return err
			}
			plans, planErr := a.buildProposals(ctx, j.session, j.user, j.version, intent)
			if planErr != nil {
				_, err = a.DB.ExecContext(ctx, "UPDATE guide_replans SET status='FAILED',locked_until=NULL,error_message=? WHERE id=? AND status='PROCESSING'", truncateReplanError(planErr.Error()), j.id)
				if err != nil {
					return err
				}
				continue
			}
			count = len(plans)
			if count == 0 {
				message = "原方案失效：" + j.reason + "。当前没有满足条件的替代方案，请修改预算或锁定条件。"
			}
			if count > 0 && a.Guide.Model != nil && a.Guide.InitErr == nil {
				if analysis, e := a.Guide.analyzeProposals(ctx, intent, plans); e == nil {
					message += " " + analysis
				}
			}
		}
		tx, err := a.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO guide_messages(session_id,user_id,role,content,replan_event_id) VALUES(?,?,'assistant',?,?)", j.session, j.user, message, j.id); err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE guide_replans SET status='DONE',locked_until=NULL,proposal_count=? WHERE id=? AND status='PROCESSING'", count, j.id); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func truncateReplanError(message string) string {
	if len(message) > 500 {
		return strings.ToValidUTF8(message[:490], "")
	}
	return message
}
