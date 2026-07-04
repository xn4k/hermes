package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"golang.org/x/crypto/bcrypt"
)

type App struct {
	db         *pgxpool.Pool
	cookieName string
	news       *NewsService
}

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL ist nicht gesetzt")
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		log.Fatalf("Datenbank-Pool konnte nicht erstellt werden: %v", err)
	}
	defer db.Close()

	if err := db.Ping(startupCtx); err != nil {
		log.Fatalf("Datenbank ist nicht erreichbar: %v", err)
	}

	if err := migrate(startupCtx, db); err != nil {
		log.Fatalf("Migration fehlgeschlagen: %v", err)
	}

	if err := ensureAdminUser(startupCtx, db); err != nil {
		log.Fatalf("Admin-User konnte nicht vorbereitet werden: %v", err)
	}

	cookieName := os.Getenv("SESSION_COOKIE_NAME")
	if cookieName == "" {
		cookieName = "hermes_session"
	}

	app := &App{
		db:         db,
		cookieName: cookieName,
		news:       NewNewsService(newsSources),
	}

	if err := migrateFeed(startupCtx, app); err != nil {
		log.Fatalf("Feed-Migration fehlgeschlagen: %v", err)
	}

	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	e.GET("/api/health", app.handleHealth)
	e.POST("/api/login", app.handleLogin)
	e.GET("/api/me", app.handleMe)
	e.POST("/api/logout", app.handleLogout)

	e.GET("/api/feed", app.handleListFeed)
	e.POST("/api/feed", app.handleCreateFeedEntry)
	e.DELETE("/api/feed/:id", app.handleDeleteFeedEntry)
	e.PATCH("/api/feed/:id/pin", app.handleTogglePinFeedEntry)

	e.GET("/api/news", app.handleGetNews)
	e.POST("/api/news/refresh", app.handleRefreshNews)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Hermes API läuft auf Port %s", port)

	if err := e.Start(":" + port); err != nil {
		log.Fatal(err)
	}
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
		`
		CREATE TABLE IF NOT EXISTS sessions (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_sessions_token_hash
		ON sessions(token_hash);
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_sessions_expires_at
		ON sessions(expires_at);
		`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			return err
		}
	}

	_, _ = db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW();`)

	return nil
}

func ensureAdminUser(ctx context.Context, db *pgxpool.Pool) error {
	username := strings.TrimSpace(os.Getenv("HERMES_ADMIN_USERNAME"))
	displayName := strings.TrimSpace(os.Getenv("HERMES_ADMIN_DISPLAY_NAME"))
	password := os.Getenv("HERMES_ADMIN_PASSWORD")

	if username == "" || password == "" {
		log.Println("Kein Admin-User geseedet: HERMES_ADMIN_USERNAME oder HERMES_ADMIN_PASSWORD fehlt")
		return nil
	}

	if displayName == "" {
		displayName = username
	}

	var existingID int64
	err := db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, username).Scan(&existingID)
	if err == nil {
		log.Printf("Admin-User %q existiert bereits", username)
		return nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO users (username, display_name, password_hash)
		VALUES ($1, $2, $3)
		`,
		username,
		displayName,
		string(passwordHash),
	)

	if err != nil {
		return err
	}

	log.Printf("Admin-User %q wurde erstellt", username)

	return nil
}

func (app *App) handleHealth(c *echo.Context) error {
	checkCtx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()

	if err := app.db.Ping(checkCtx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status":   "error",
			"database": "unreachable",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"service":  "hermes-api",
		"status":   "ok",
		"database": "connected",
	})
}

func (app *App) handleLogin(c *echo.Context) error {
	var req loginRequest

	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "invalid_json",
		})
	}

	req.Username = strings.TrimSpace(req.Username)

	if req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "username_and_password_required",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	var user User
	var passwordHash string

	err := app.db.QueryRow(
		ctx,
		`
		SELECT id, username, display_name, password_hash
		FROM users
		WHERE username = $1
		`,
		req.Username,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &passwordHash)

	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "invalid_credentials",
		})
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "database_error",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "invalid_credentials",
		})
	}

	token, err := newSessionToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "session_token_failed",
		})
	}

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	tokenHash := hashSessionToken(token)

	_, _ = app.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW();`)

	_, err = app.db.Exec(
		ctx,
		`
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		`,
		user.ID,
		tokenHash,
		expiresAt,
	)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "session_create_failed",
		})
	}

	c.SetCookie(&http.Cookie{
		Name:     app.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	return c.JSON(http.StatusOK, map[string]any{
		"user": user,
	})
}

func (app *App) handleMe(c *echo.Context) error {
	user, err := app.currentUser(c)

	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"user": user,
	})
}

func (app *App) handleLogout(c *echo.Context) error {
	cookie, err := c.Cookie(app.cookieName)

	if err == nil && cookie.Value != "" {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
		defer cancel()

		_, _ = app.db.Exec(
			ctx,
			`DELETE FROM sessions WHERE token_hash = $1`,
			hashSessionToken(cookie.Value),
		)
	}

	c.SetCookie(&http.Cookie{
		Name:     app.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	return c.JSON(http.StatusOK, map[string]any{
		"status": "logged_out",
	})
}

func (app *App) handleGetNews(c *echo.Context) error {
	if _, err := app.currentUser(c); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	articles, cached := app.news.getCachedArticles()
	if cached {
		return c.JSON(http.StatusOK, map[string]any{
			"source":   "cache",
			"count":    len(articles),
			"articles": articles,
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 12*time.Second)
	defer cancel()

	articles, err := app.news.refresh(ctx)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error":   "news_refresh_failed",
			"details": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"source":   "refresh",
		"count":    len(articles),
		"articles": articles,
	})
}

func (app *App) handleRefreshNews(c *echo.Context) error {
	if _, err := app.currentUser(c); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error": "not_authenticated",
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 12*time.Second)
	defer cancel()

	articles, err := app.news.refresh(ctx)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error":   "news_refresh_failed",
			"details": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"source":   "manual_refresh",
		"count":    len(articles),
		"articles": articles,
	})
}

func (app *App) currentUser(c *echo.Context) (User, error) {
	cookie, err := c.Cookie(app.cookieName)
	if err != nil || cookie.Value == "" {
		return User{}, errors.New("missing_session_cookie")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	var user User

	err = app.db.QueryRow(
		ctx,
		`
		SELECT u.id, u.username, u.display_name
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > NOW()
		`,
		hashSessionToken(cookie.Value),
	).Scan(&user.ID, &user.Username, &user.DisplayName)

	if err != nil {
		return User{}, err
	}

	return user, nil
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)

	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
