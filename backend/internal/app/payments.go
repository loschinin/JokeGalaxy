package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type paymentPackage struct { Code string; Credits int; Amount int; Description string }
var paymentPackages = map[string]paymentPackage{
	"pack_50": {Code: "pack_50", Credits: 50, Amount: 50, Description: "50 генераций стендапа"},
	"pack_100": {Code: "pack_100", Credits: 100, Amount: 80, Description: "100 генераций стендапа"},
	"pack_200": {Code: "pack_200", Credits: 200, Amount: 140, Description: "200 генераций стендапа"},
}

func (s *Server) handleCreatePayment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	var input struct { PackageCode string `json:"package_code"`; Email string `json:"email"` }
	if !decodeJSON(w, r, &input) { return }
	pkg, exists := paymentPackages[input.PackageCode]
	if !exists { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неизвестный пакет генераций."}); return }
	if s.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Оплата доступна после подключения PostgreSQL и ЮKassa."}); return }
	if s.cfg.YooKassaShopID == "" || s.cfg.YooKassaSecretKey == "" { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ЮKassa ещё не настроена."}); return }
	email := strings.TrimSpace(input.Email)
	if email == "" { email = strings.TrimSpace(user.Email) }
	parsedEmail, emailErr := mail.ParseAddress(email)
	if emailErr != nil || parsedEmail.Address != email { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Укажите корректный email для электронного чека."}); return }

	var paymentRowID int64
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO payments (user_id, package_code, credits_amount, amount_rub, status) VALUES ($1,$2,$3,$4,'pending') RETURNING id`, user.ID, pkg.Code, pkg.Credits, pkg.Amount).Scan(&paymentRowID)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось создать запись платежа."}); return }

	idempotenceKey, err := newIdempotenceKey()
	if err != nil { _, _ = s.db.ExecContext(r.Context(), `UPDATE payments SET status='failed' WHERE id=$1`, paymentRowID); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось подготовить платёж."}); return }
	payload := map[string]any{
		"amount": map[string]string{"value": fmt.Sprintf("%d.00", pkg.Amount), "currency": "RUB"},
		"capture": true,
		"confirmation": map[string]string{"type": "redirect", "return_url": fmt.Sprintf("%s/standup?payment=return&payment_row_id=%d", s.cfg.SiteURL, paymentRowID)},
		"description": pkg.Description,
		"metadata": map[string]string{"user_id": strconv.FormatInt(user.ID, 10), "package_code": pkg.Code, "payment_row_id": strconv.FormatInt(paymentRowID, 10)},
		"receipt": map[string]any{
			"customer": map[string]string{"email": email},
			"items": []map[string]any{{
				"description": pkg.Description,
				"quantity": "1.00",
				"amount": map[string]string{"value": fmt.Sprintf("%d.00", pkg.Amount), "currency": "RUB"},
				"vat_code": 1,
				"payment_mode": "full_payment",
				"payment_subject": "service",
			}},
		},
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.yookassa.ru/v3/payments", bytes.NewReader(body))
	request.SetBasicAuth(s.cfg.YooKassaShopID, s.cfg.YooKassaSecretKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotence-Key", idempotenceKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil { _, _ = s.db.ExecContext(r.Context(), `UPDATE payments SET status='failed' WHERE id=$1`, paymentRowID); writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ЮKassa не ответила. Попробуйте ещё раз."}); return }
	defer response.Body.Close()
	var created struct {
		ID string `json:"id"`
		Status string `json:"status"`
		Confirmation struct { URL string `json:"confirmation_url"` } `json:"confirmation"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&created)
	if response.StatusCode < 200 || response.StatusCode >= 300 || decodeErr != nil || created.ID == "" || created.Confirmation.URL == "" {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE payments SET status='failed' WHERE id=$1`, paymentRowID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Не удалось создать страницу оплаты."})
		return
	}
	_, err = s.db.ExecContext(r.Context(), `UPDATE payments SET yookassa_id=$1, status=$2 WHERE id=$3`, created.ID, created.Status, paymentRowID)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Платёж создан, но не сохранён. Обратитесь в поддержку."}); return }
	writeJSON(w, http.StatusCreated, map[string]any{"payment_id": created.ID, "confirmation_url": created.Confirmation.URL, "status": created.Status})
}

func newIdempotenceKey() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil { return "", err }
	return hex.EncodeToString(value), nil
}

func (s *Server) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	if s.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Хранилище платежей недоступно."}); return }
	var event struct { Type string `json:"type"`; Event string `json:"event"`; Object json.RawMessage `json:"object"` }
	if !decodeJSON(w, r, &event) { return }
	var object struct { ID string `json:"id"` }
	if err := json.Unmarshal(event.Object, &object); err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Некорректное уведомление ЮKassa."}); return }
	if event.Event != "payment.succeeded" || object.ID == "" { writeJSON(w, http.StatusOK, map[string]bool{"ignored": true}); return }
	verifyContext, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	verified, err := s.fetchYooKassaPayment(verifyContext, object.ID)
	if err != nil || verified.ID != object.ID || verified.Status != "succeeded" || verified.Amount.Currency != "RUB" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Платёж не подтверждён."}); return }

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось обработать платёж."}); return }
	var rowID, userID int64
	var credits int
	var amount float64
	var status string
	err = tx.QueryRowContext(r.Context(), `SELECT id,user_id,credits_amount,amount_rub::float8,status FROM payments WHERE yookassa_id=$1 FOR UPDATE`, object.ID).Scan(&rowID, &userID, &credits, &amount, &status)
	if err == sql.ErrNoRows { _ = tx.Rollback(); writeJSON(w, http.StatusNotFound, map[string]string{"error": "Платёж не найден."}); return }
	if err != nil { _ = tx.Rollback(); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось прочитать платёж."}); return }
	if status == "succeeded" { _ = tx.Commit(); writeJSON(w, http.StatusOK, map[string]bool{"already_processed": true}); return }
	confirmedAmount, parseErr := strconv.ParseFloat(verified.Amount.Value, 64)
	if parseErr != nil || math.Abs(confirmedAmount-amount) > .009 { _ = tx.Rollback(); writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Сумма платежа не совпадает."}); return }
	if _, err := tx.ExecContext(r.Context(), `UPDATE users SET standup_credits=standup_credits+$1 WHERE id=$2`, credits, userID); err != nil { _ = tx.Rollback(); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось начислить генерации."}); return }
	if _, err := tx.ExecContext(r.Context(), `UPDATE payments SET status='succeeded', paid_at=NOW() WHERE id=$1`, rowID); err != nil { _ = tx.Rollback(); writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось отметить платёж."}); return }
	if err := tx.Commit(); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось завершить платёж."}); return }
	writeJSON(w, http.StatusOK, map[string]bool{"credited": true})
}

type yooKassaPayment struct {
	ID string `json:"id"`
	Status string `json:"status"`
	Amount struct { Value string `json:"value"`; Currency string `json:"currency"` } `json:"amount"`
}

func (s *Server) fetchYooKassaPayment(ctx context.Context, id string) (yooKassaPayment, error) {
	var payment yooKassaPayment
	if s.cfg.YooKassaShopID == "" || s.cfg.YooKassaSecretKey == "" { return payment, fmt.Errorf("YooKassa credentials missing") }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.yookassa.ru/v3/payments/"+url.PathEscape(id), nil)
	if err != nil { return payment, err }
	request.SetBasicAuth(s.cfg.YooKassaShopID, s.cfg.YooKassaSecretKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil { return payment, err }
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 { return payment, fmt.Errorf("YooKassa verification returned %s", response.Status) }
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payment)
	return payment, err
}

func (s *Server) handlePaymentHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok { return }
	if s.db == nil { writeJSON(w, http.StatusOK, map[string]any{"payments": []any{}}); return }
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,package_code,credits_amount,amount_rub,status,created_at,paid_at FROM payments WHERE user_id=$1 ORDER BY created_at DESC LIMIT 50`, user.ID)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Не удалось получить историю платежей."}); return }
	defer rows.Close()
	type payment struct { ID int64 `json:"id"`; Package string `json:"package_code"`; Credits int `json:"credits_amount"`; Amount float64 `json:"amount_rub"`; Status string `json:"status"`; Created time.Time `json:"created_at"`; Paid *time.Time `json:"paid_at,omitempty"` }
	items := make([]payment, 0, 20)
	for rows.Next() { var item payment; if rows.Scan(&item.ID,&item.Package,&item.Credits,&item.Amount,&item.Status,&item.Created,&item.Paid) == nil { items = append(items,item) } }
	writeJSON(w, http.StatusOK, map[string]any{"payments": items})
}
