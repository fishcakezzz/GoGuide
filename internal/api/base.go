package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

type Server struct {
	DB            *sql.DB
	Secret        []byte
	PaymentSecret []byte
	Guide         *Guide
}

type userClaims struct {
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type authKey struct{}

func OpenDB() (*sql.DB, error) {
	dsn := os.Getenv("GOGUIDE_DSN")
	if dsn == "" {
		dsn = "goguide:goguide@tcp(127.0.0.1:3306)/goguide?parseTime=true&charset=utf8mb4"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func New(db *sql.DB, secret string) *Server {
	paymentSecret := os.Getenv("GOGUIDE_PAYMENT_SECRET")
	if paymentSecret == "" {
		paymentSecret = secret
	}
	return &Server{DB: db, Secret: []byte(secret), PaymentSecret: []byte(paymentSecret), Guide: NewGuide(db)}
}

func (a *Server) Routes(server *rest.Server) {
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/", Handler: a.home},
		{Method: http.MethodGet, Path: "/healthz", Handler: a.health},
		{Method: http.MethodPost, Path: "/v1/auth/register", Handler: a.register},
		{Method: http.MethodPost, Path: "/v1/auth/login", Handler: a.login},
		{Method: http.MethodPost, Path: "/v1/auth/logout", Handler: a.withAuth(a.logout)},
		{Method: http.MethodGet, Path: "/v1/me", Handler: a.withAuth(a.me)},
		{Method: http.MethodPut, Path: "/v1/me", Handler: a.withAuth(a.updateMe)},
		{Method: http.MethodGet, Path: "/v1/addresses", Handler: a.withAuth(a.listAddresses)},
		{Method: http.MethodPost, Path: "/v1/addresses", Handler: a.withAuth(a.addAddress)},
		{Method: http.MethodPut, Path: "/v1/addresses/:id", Handler: a.withAuth(a.updateAddress)},
		{Method: http.MethodDelete, Path: "/v1/addresses/:id", Handler: a.withAuth(a.deleteAddress)},
		{Method: http.MethodGet, Path: "/v1/products", Handler: a.products},
		{Method: http.MethodGet, Path: "/v1/skus/:id", Handler: a.sku},
		{Method: http.MethodGet, Path: "/v1/compare", Handler: a.compare},
		{Method: http.MethodGet, Path: "/v1/cart", Handler: a.withAuth(a.cart)},
		{Method: http.MethodPost, Path: "/v1/cart/items", Handler: a.withAuth(a.addCart)},
		{Method: http.MethodPut, Path: "/v1/cart/items/:id", Handler: a.withAuth(a.updateCart)},
		{Method: http.MethodDelete, Path: "/v1/cart/items/:id", Handler: a.withAuth(a.deleteCart)},
		{Method: http.MethodPost, Path: "/v1/orders/preview", Handler: a.withAuth(a.preview)},
		{Method: http.MethodPost, Path: "/v1/orders", Handler: a.withAuth(a.submitOrder)},
		{Method: http.MethodGet, Path: "/v1/orders", Handler: a.withAuth(a.orders)},
		{Method: http.MethodGet, Path: "/v1/orders/:id", Handler: a.withAuth(a.order)},
		{Method: http.MethodPost, Path: "/v1/orders/:id/cancel", Handler: a.withAuth(a.cancelOrder)},
		{Method: http.MethodPost, Path: "/v1/orders/:id/mock-pay", Handler: a.withAuth(a.mockPay)},
		{Method: http.MethodPost, Path: "/v1/internal/mock-payment-callback", Handler: a.paymentCallback},
		{Method: http.MethodPost, Path: "/v1/guide/sessions", Handler: a.withAuth(a.newSession)},
		{Method: http.MethodGet, Path: "/v1/guide/sessions/:id", Handler: a.withAuth(a.getSession)},
		{Method: http.MethodGet, Path: "/v1/guide/sessions/:id/proposals", Handler: a.withAuth(a.listProposals)},
		{Method: http.MethodGet, Path: "/v1/guide/sessions/:id/tool-runs", Handler: a.withAuth(a.listToolRuns)},
		{Method: http.MethodPost, Path: "/v1/guide/sessions/:id/messages", Handler: a.withAuth(a.guideMessage)},
		{Method: http.MethodPost, Path: "/v1/guide/proposals/:id/confirm", Handler: a.withAuth(a.confirmProposal)},
		{Method: http.MethodPost, Path: "/v1/admin/products", Handler: a.withAdmin(a.createProduct)},
		{Method: http.MethodPut, Path: "/v1/admin/products/:id", Handler: a.withAdmin(a.updateProduct)},
		{Method: http.MethodPost, Path: "/v1/admin/skus", Handler: a.withAdmin(a.createSKU)},
		{Method: http.MethodPut, Path: "/v1/admin/skus/:id", Handler: a.withAdmin(a.updateSKU)},
		{Method: http.MethodPut, Path: "/v1/admin/skus/:id/price", Handler: a.withAdmin(a.updatePrice)},
		{Method: http.MethodPut, Path: "/v1/admin/skus/:id/stock", Handler: a.withAdmin(a.adjustStock)},
	})
}

func (a *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := a.DB.PingContext(r.Context()); err != nil {
		fail(w, http.StatusServiceUnavailable, "db_unavailable", "")
		return
	}
	respond(w, http.StatusOK, map[string]string{"status": "ok"})
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
func readJSON(r *http.Request, dst any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("extra JSON content")
	}
	return nil
}
func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(pathvar.Vars(r)["id"], 10, 64) }
func pathString(r *http.Request) string     { return pathvar.Vars(r)["id"] }

func (a *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		part := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if part == "" || part == r.Header.Get("Authorization") {
			fail(w, 401, "unauthorized", "请先登录")
			return
		}
		claims := &userClaims{}
		token, err := jwt.ParseWithClaims(part, claims, func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected algorithm")
			}
			return a.Secret, nil
		})
		if err != nil || !token.Valid || claims.UserID < 1 || claims.ID == "" || claims.ExpiresAt == nil {
			fail(w, 401, "unauthorized", "登录已失效")
			return
		}
		var revoked int
		err = a.DB.QueryRowContext(r.Context(), "SELECT 1 FROM revoked_tokens WHERE jti=?", claims.ID).Scan(&revoked)
		if err == nil {
			fail(w, 401, "unauthorized", "登录已退出")
			return
		}
		if err != sql.ErrNoRows {
			fail(w, 503, "auth_unavailable", "")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authKey{}, claims)))
	}
}
func (a *Server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if current(r).Role != "admin" {
			fail(w, 403, "forbidden", "")
			return
		}
		next(w, r)
	})
}
func current(r *http.Request) *userClaims { return r.Context().Value(authKey{}).(*userClaims) }
func logErr(r *http.Request, err error) {
	slog.Error("request failed", "path", r.URL.Path, "error", err)
}
