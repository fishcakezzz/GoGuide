package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func (a *Server) listProposals(w http.ResponseWriter, r *http.Request) {
	sid := pathString(r)
	var version int
	err := a.DB.QueryRowContext(r.Context(), "SELECT intent_version FROM guide_sessions WHERE id=? AND user_id=?", sid, current(r).UserID).Scan(&version)
	if err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), "SELECT id,intent_version,proposal_version,items_json,evidence_json,amount_cents,confirmed_at,created_at,status FROM guide_proposals WHERE session_id=? AND user_id=? ORDER BY proposal_version DESC LIMIT 30", sid, current(r).UserID)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id string
		var iv, pv int
		var items, evidence []byte
		var amount int64
		var confirmed sql.NullTime
		var created time.Time
		var status string
		if err := rows.Scan(&id, &iv, &pv, &items, &evidence, &amount, &confirmed, &created, &status); err != nil {
			fail(w, 500, "internal", "")
			return
		}
		out = append(out, map[string]any{"id": id, "intent_version": iv, "proposal_version": pv, "items": json.RawMessage(items), "evidence": json.RawMessage(evidence), "amount_cents": amount, "confirmed": confirmed.Valid, "stale": iv != version || status != "ACTIVE", "status": status, "created_at": created})
	}
	respond(w, 200, map[string]any{"items": out})
}

func (a *Server) listReplans(w http.ResponseWriter, r *http.Request) {
	sid := pathString(r)
	var version int
	if err := a.DB.QueryRowContext(r.Context(), "SELECT intent_version FROM guide_sessions WHERE id=? AND user_id=?", sid, current(r).UserID).Scan(&version); err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	} else if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), "SELECT id,old_intent_version,new_intent_version,reason,status,proposal_count,error_message,created_at FROM guide_replans WHERE session_id=? AND user_id=? ORDER BY id DESC LIMIT 30", sid, current(r).UserID)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var oldVersion, newVersion, count int
		var reason, status, errorMessage string
		var created time.Time
		if err := rows.Scan(&id, &oldVersion, &newVersion, &reason, &status, &count, &errorMessage, &created); err != nil {
			fail(w, 500, "internal", "")
			return
		}
		out = append(out, map[string]any{"id": id, "old_intent_version": oldVersion, "new_intent_version": newVersion, "reason": reason, "status": status, "proposal_count": count, "error_message": errorMessage, "created_at": created})
	}
	if rows.Err() != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"intent_version": version, "items": out})
}

func (a *Server) listToolRuns(w http.ResponseWriter, r *http.Request) {
	sid := pathString(r)
	var exists int
	err := a.DB.QueryRowContext(r.Context(), "SELECT 1 FROM guide_sessions WHERE id=? AND user_id=?", sid, current(r).UserID).Scan(&exists)
	if err == sql.ErrNoRows {
		fail(w, 404, "not_found", "")
		return
	}
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), "SELECT id,tool_name,input_summary,result_summary,duration_ms,error_code,created_at FROM guide_tool_runs WHERE session_id=? ORDER BY id DESC LIMIT 100", sid)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, code string
		var input, result []byte
		var duration int
		var at time.Time
		if rows.Scan(&id, &name, &input, &result, &duration, &code, &at) == nil {
			out = append(out, map[string]any{"id": id, "tool": name, "input": json.RawMessage(input), "result": json.RawMessage(result), "duration_ms": duration, "error_code": code, "created_at": at})
		}
	}
	respond(w, 200, map[string]any{"items": out})
}
