package qrlogin

import (
	"context"
	"database/sql"

	"easyserver/internal/infra/database"
	"fmt"
	"time"
)

type sqliteRepo struct {
	db database.DB
}

func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: database.Wrap(db)}
}

func (r *sqliteRepo) Create(ctx context.Context, s *QRLoginSession) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO qr_login_sessions
		(qr_token, status, user_id, web_token, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.QRToken, s.Status, s.UserID, s.WebToken, s.CreatedAt, s.ExpiresAt)
	if err != nil {
		return 0, fmt.Errorf("create qr login session: %w", err)
	}
	return res.LastInsertId()
}

func (r *sqliteRepo) GetByToken(ctx context.Context, qrToken string) (*QRLoginSession, error) {
	s := &QRLoginSession{}
	var confirmedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id, qr_token, status, user_id, web_token, user_json,
		created_at, expires_at, confirmed_at
		FROM qr_login_sessions WHERE qr_token = ?`, qrToken).Scan(
		&s.ID, &s.QRToken, &s.Status, &s.UserID, &s.WebToken, &s.UserJSON,
		&s.CreatedAt, &s.ExpiresAt, &confirmedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get qr login session: %w", err)
	}
	if confirmedAt.Valid {
		t := confirmedAt.Time
		s.ConfirmedAt = &t
	}
	return s, nil
}

func (r *sqliteRepo) MarkConfirmed(ctx context.Context, qrToken string, userID int64, webToken string, userJSON string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE qr_login_sessions
		SET status = ?, user_id = ?, web_token = ?, user_json = ?, confirmed_at = ?
		WHERE qr_token = ? AND status = ?`,
		StatusConfirmed, userID, webToken, userJSON, time.Now(), qrToken, StatusPending)
	if err != nil {
		return fmt.Errorf("mark qr login confirmed: %w", err)
	}
	return nil
}

func (r *sqliteRepo) Consume(ctx context.Context, qrToken string) (*QRLoginSession, bool, error) {
	// 抢锁：仅当仍是 confirmed 时置为 consumed。并发 poll 中只有一个会获行。
	res, err := r.db.ExecContext(ctx, `UPDATE qr_login_sessions
		SET status = ? WHERE qr_token = ? AND status = ?`,
		StatusConsumed, qrToken, StatusConfirmed)
	if err != nil {
		return nil, false, fmt.Errorf("consume qr login session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if n == 0 {
		// 没抢到锁：已被消费 / 已取消 / 仍 pending
		return nil, false, nil
	}
	s, err := r.GetByToken(ctx, qrToken)
	if err != nil {
		return nil, false, err
	}
	return s, true, nil
}

func (r *sqliteRepo) Delete(ctx context.Context, qrToken string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM qr_login_sessions WHERE qr_token = ?", qrToken)
	return err
}

func (r *sqliteRepo) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM qr_login_sessions
		WHERE expires_at < ? OR status IN (?, ?)`, time.Now(), StatusCancelled, StatusConsumed)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
