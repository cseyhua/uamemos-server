package service

import (
	"context"
	"database/sql"
	"net/http"
	"time"
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
	g  *gin.Engine
	db *sql.DB

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
	g := gin.New()

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
		ctx.Next()
	})
}
