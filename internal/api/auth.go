package api

import (
	"database/sql"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *Server) register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if readJSON(r, &in) != nil || len(in.Password) < 8 {
		fail(w, 400, "invalid_input", "邮箱或密码无效，密码至少 8 位")
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(in.Email); err != nil {
		fail(w, 400, "invalid_email", "")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "INSERT INTO users(email,password_hash) VALUES (?,?)", in.Email, string(hash))
	if err != nil {
		fail(w, 409, "email_exists", "邮箱已注册")
		return
	}
	id, _ := res.LastInsertId()
	respond(w, 201, map[string]any{"user_id": id, "token": a.sign(id, "user")})
}

func (a *Server) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if readJSON(r, &in) != nil {
		fail(w, 400, "invalid_input", "")
		return
	}
	var id int64
	var hash, role string
	err := a.DB.QueryRowContext(r.Context(), "SELECT id,password_hash,role FROM users WHERE email=?", strings.ToLower(strings.TrimSpace(in.Email))).Scan(&id, &hash, &role)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		fail(w, 401, "invalid_credentials", "账号或密码错误")
		return
	}
	respond(w, 200, map[string]any{"user_id": id, "role": role, "token": a.sign(id, role)})
}

func (a *Server) sign(id int64, role string) string {
	claims := userClaims{UserID: id, Role: role, RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.Secret)
	return s
}
func (a *Server) logout(w http.ResponseWriter, r *http.Request) {
	claims := current(r)
	_, err := a.DB.ExecContext(r.Context(), "INSERT IGNORE INTO revoked_tokens(jti,expires_at) VALUES(?,?)", claims.ID, claims.ExpiresAt.Time)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]bool{"logged_out": true})
}
func (a *Server) me(w http.ResponseWriter, r *http.Request) {
	var email, name string
	err := a.DB.QueryRowContext(r.Context(), "SELECT email,display_name FROM users WHERE id=?", current(r).UserID).Scan(&email, &name)
	if err != nil {
		fail(w, 404, "not_found", "")
		return
	}
	respond(w, 200, map[string]any{"id": current(r).UserID, "email": email, "display_name": name, "role": current(r).Role})
}
func (a *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if readJSON(r, &in) != nil || len(in.DisplayName) > 100 {
		fail(w, 400, "invalid_input", "")
		return
	}
	_, err := a.DB.ExecContext(r.Context(), "UPDATE users SET display_name=? WHERE id=?", strings.TrimSpace(in.DisplayName), current(r).UserID)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"display_name": in.DisplayName})
}

type addressInput struct {
	Recipient string `json:"recipient"`
	Phone     string `json:"phone"`
	Detail    string `json:"detail"`
	Default   bool   `json:"default"`
}

func (a *Server) listAddresses(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.QueryContext(r.Context(), "SELECT id,recipient,phone,detail,is_default FROM addresses WHERE user_id=? ORDER BY is_default DESC,id", current(r).UserID)
	if err != nil {
		logErr(r, err)
		fail(w, 500, "internal", "")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var recipient, phone, detail string
		var def bool
		if rows.Scan(&id, &recipient, &phone, &detail, &def) == nil {
			items = append(items, map[string]any{"id": id, "recipient": recipient, "phone": phone, "detail": detail, "default": def})
		}
	}
	respond(w, 200, map[string]any{"items": items})
}
func (a *Server) addAddress(w http.ResponseWriter, r *http.Request) {
	var in addressInput
	if readJSON(r, &in) != nil || strings.TrimSpace(in.Recipient) == "" || strings.TrimSpace(in.Detail) == "" || strings.TrimSpace(in.Phone) == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer tx.Rollback()
	if in.Default {
		if _, err = tx.ExecContext(r.Context(), "UPDATE addresses SET is_default=FALSE WHERE user_id=?", current(r).UserID); err != nil {
			fail(w, 500, "internal", "")
			return
		}
	}
	res, err := tx.ExecContext(r.Context(), "INSERT INTO addresses(user_id,recipient,phone,detail,is_default) VALUES(?,?,?,?,?)", current(r).UserID, in.Recipient, in.Phone, in.Detail, in.Default)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	id, _ := res.LastInsertId()
	if err = tx.Commit(); err != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 201, map[string]any{"id": id})
}
func (a *Server) updateAddress(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	var in addressInput
	if readJSON(r, &in) != nil || in.Recipient == "" || in.Phone == "" || in.Detail == "" {
		fail(w, 400, "invalid_input", "")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	defer tx.Rollback()
	if in.Default {
		if _, err = tx.ExecContext(r.Context(), "UPDATE addresses SET is_default=FALSE WHERE user_id=?", current(r).UserID); err != nil {
			fail(w, 500, "internal", "")
			return
		}
	}
	res, err := tx.ExecContext(r.Context(), "UPDATE addresses SET recipient=?,phone=?,detail=?,is_default=? WHERE id=? AND user_id=?", in.Recipient, in.Phone, in.Detail, in.Default, id, current(r).UserID)
	if err != nil {
		fail(w, 500, "internal", "")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "not_found", "")
		return
	}
	if tx.Commit() != nil {
		fail(w, 500, "internal", "")
		return
	}
	respond(w, 200, map[string]any{"id": id})
}
func (a *Server) deleteAddress(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, 400, "invalid_id", "")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), "DELETE FROM addresses WHERE id=? AND user_id=?", id, current(r).UserID)
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
