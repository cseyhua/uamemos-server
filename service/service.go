package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"uamemos/api"
	"uamemos/service/profile"
	"uamemos/store"
	"uamemos/store/db"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/secure"
	"github.com/gin-contrib/timeout"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// 定义服务
type Service struct {
	g    *gin.Engine
	http *http.Server
	db   *sql.DB

	ID      string
	Profile *profile.Profile
	Store   *store.Store
}

func timeoutMiddleware() gin.HandlerFunc {
	return timeout.New(
		timeout.WithTimeout(30*time.Second),
		timeout.WithHandler(func(c *gin.Context) {
			c.Next()
		}),
		timeout.WithResponse(func(ctx *gin.Context) {
			ctx.String(http.StatusRequestTimeout, "timeout")
		}),
	)
}

func NewService(ctx context.Context, profile *profile.Profile) (*Service, error) {
	gin.SetMode(gin.ReleaseMode)
	g := gin.Default()

	db := db.NewDB(profile)
	if err := db.Open(ctx); err != nil {
		return nil, errors.Wrap(err, "cannot open db")
	}

	s := &Service{
		g:       g,
		db:      db.DBInstance,
		Profile: profile,
	}

	storeInstance := store.New(db.DBInstance, profile)
	s.Store = storeInstance

	g.Use(gin.LoggerWithConfig(gin.LoggerConfig{}))

	g.Use(gzip.Gzip(gzip.DefaultCompression))

	g.Use(cors.Default())

	g.Use(secure.New(secure.Config{}))

	g.Use(timeoutMiddleware())

	serviceID, err := s.getSystemServiceID(ctx)
	if err != nil {
		return nil, err
	}

	s.ID = serviceID

	embedFrontend(g)

	secret := "uamemos"

	if profile.Mode == "prod" {
		// 没有认真看这个函数
		secret, err = s.getSystemSecretSessionName(ctx)
		if err != nil {
			return nil, err
		}
	}

	apiGroup := g.Group("/api")
	apiGroup.Use(func(ctx *gin.Context) {
		JWTMiddleware(s, ctx, secret)
	})
	s.registerSystemRoutes(apiGroup)
	s.registerAuthRoutes(apiGroup, secret)
	s.registerUserRoutes(apiGroup)
	s.registerMemoRoutes(apiGroup)
	s.registerTagRoutes(apiGroup)

	return s, nil
}

func (s *Service) Start(ctx context.Context) error {
	if err := s.createServerStartActivity(ctx); err != nil {
		return errors.Wrap(err, "failed to create activity")
	}
	server := &http.Server{
		Addr:    fmt.Sprint(":", s.Profile.Port),
		Handler: s.g,
	}
	s.http = server
	return server.ListenAndServe()
}

func (s *Service) Shutdown(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 关闭gin逻辑
	if err := s.http.Shutdown(ctx); err != nil {
		fmt.Printf("failed to shutdown service, error: %v\n", err)
	}

	if err := s.db.Close(); err != nil {
		fmt.Printf("failed to close database, err: %v\n", err)
	}
	fmt.Printf("uamemos stopped properly\n")
}

func (s *Service) createServerStartActivity(ctx context.Context) error {
	payload := api.ActivityServerStartPayload{
		ServerID: s.ID,
		Profile:  s.Profile,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: api.UnknownID,
		Type:      api.ActivityServerStart,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}
