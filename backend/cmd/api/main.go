package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL ist nicht gesetzt")
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := pgxpool.New(startupCtx, databaseURL)
	if err != nil {
		log.Fatalf("Datenbank-Pool konnte nicht erstellt werden: %v", err)
	}
	defer db.Close()

	if err := db.Ping(startupCtx); err != nil {
		log.Fatalf("Datenbank ist nicht erreichbar: %v", err)
	}

	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	e.GET("/api/health", func(c *echo.Context) error {
		checkCtx, cancel := context.WithTimeout(
			c.Request().Context(),
			2*time.Second,
		)
		defer cancel()

		if err := db.Ping(checkCtx); err != nil {
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
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Hermes API läuft auf Port %s", port)

	if err := e.Start(":" + port); err != nil {
		log.Fatal(err)
	}
}
