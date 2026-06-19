package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"
)

type FeedEntry struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type createFeedEntryRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

func migrateFeed(ctx context.Context, app *App) error {
	_, err := app.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS feed_entries (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type TEXT NOT NULL DEFAULT 'note',
			title TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			pinned BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_feed_entries_user_created
		ON feed_entries(user_id, created_at DESC);
	`)

	return err
}

func (app *App) handleListFeed(c *echo.Context) error {
	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	rows, err := app.db.Query(
		ctx,
		`
		SELECT id, type, title, content, pinned, created_at, updated_at
		FROM feed_entries
		WHERE user_id = $1
		ORDER BY pinned DESC, created_at DESC
		LIMIT 50
		`,
		user.ID,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "database_error",
		})
	}
	defer rows.Close()

	entries := make([]FeedEntry, 0)

	for rows.Next() {
		var entry FeedEntry

		if err := rows.Scan(
			&entry.ID,
			&entry.Type,
			&entry.Title,
			&entry.Content,
			&entry.Pinned,
			&entry.CreatedAt,
			&entry.UpdatedAt,
		); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"error": "scan_error",
			})
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "rows_error",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"entries": entries,
	})
}

func (app *App) handleCreateFeedEntry(c *echo.Context) error {
	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	var req createFeedEntryRequest

	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_json",
		})
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	req.Type = strings.TrimSpace(req.Type)

	if req.Type == "" {
		req.Type = "note"
	}

	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "content_required",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	var entry FeedEntry

	err = app.db.QueryRow(
		ctx,
		`
		INSERT INTO feed_entries (user_id, type, title, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, type, title, content, pinned, created_at, updated_at
		`,
		user.ID,
		req.Type,
		req.Title,
		req.Content,
	).Scan(
		&entry.ID,
		&entry.Type,
		&entry.Title,
		&entry.Content,
		&entry.Pinned,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "database_error",
		})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"entry": entry,
	})
}

func (app *App) handleDeleteFeedEntry(c *echo.Context) error {
	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_id",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	result, err := app.db.Exec(
		ctx,
		`
		DELETE FROM feed_entries
		WHERE id = $1 AND user_id = $2
		`,
		id,
		user.ID,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "database_error",
		})
	}

	if result.RowsAffected() == 0 {
		return c.JSON(http.StatusNotFound, map[string]any{
			"error": "entry_not_found",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "deleted",
	})
}

func (app *App) handleTogglePinFeedEntry(c *echo.Context) error {
	user, err := app.currentUser(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_id",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	var entry FeedEntry

	err = app.db.QueryRow(
		ctx,
		`
		UPDATE feed_entries
		SET pinned = NOT pinned,
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, type, title, content, pinned, created_at, updated_at
		`,
		id,
		user.ID,
	).Scan(
		&entry.ID,
		&entry.Type,
		&entry.Title,
		&entry.Content,
		&entry.Pinned,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]any{
			"error": "entry_not_found",
		})
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "database_error",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"entry": entry,
	})
}
