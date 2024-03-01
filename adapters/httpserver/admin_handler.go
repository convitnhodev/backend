package httpserver

import (
	"errors"
	"net/http"

	"github.com/SeaCloudHub/backend/adapters/httpserver/model"
	"github.com/SeaCloudHub/backend/domain/identity"
	"github.com/SeaCloudHub/backend/pkg/mycontext"

	"github.com/labstack/echo/v4"
)

func (s *Server) AdminLogin(c echo.Context) error {
	var (
		ctx = mycontext.NewEchoContextAdapter(c)
		req model.AdminLoginRequest
	)

	if err := c.Bind(&req); err != nil {
		return s.handleError(c, err, http.StatusBadRequest)
	}

	if err := req.Validate(); err != nil {
		return s.handleError(c, err, http.StatusBadRequest)
	}

	sessionToken, err := s.IdentityService.Login(ctx, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			return s.handleError(c, err, http.StatusBadRequest)
		}

		return s.handleError(c, err, http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, model.AdminLoginResponse{
		SessionToken: sessionToken,
	})
}

func (s *Server) AdminMe(c echo.Context) error {
	return c.JSON(http.StatusOK, c.Get("identity"))
}

func (s *Server) RegisterAdminRoutes(router *echo.Group) {
	router.POST("/login", s.AdminLogin)

	router.Use(s.adminMiddleware)
	router.GET("/me", s.AdminMe)
}

func (s *Server) adminMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var (
			ctx = mycontext.NewEchoContextAdapter(c)
		)

		identity, ok := c.Get("identity").(*identity.Identity)
		if !ok {
			return s.handleError(c, errors.New("identity not found"), http.StatusInternalServerError)
		}

		isAdmin, err := s.PermissionService.IsManager(ctx, identity.ID)
		if err != nil {
			return s.handleError(c, err, http.StatusInternalServerError)
		}

		if !isAdmin {
			return s.handleError(c, errors.New("permission denied"), http.StatusForbidden)
		}

		return next(c)
	}
}
