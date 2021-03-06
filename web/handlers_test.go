// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattermost/mattermost-server/app"
	"github.com/mattermost/mattermost-server/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func handlerForHTTPErrors(c *Context, w http.ResponseWriter, r *http.Request) {
	c.Err = model.NewAppError("loginWithSaml", "api.user.saml.not_available.app_error", nil, "", http.StatusFound)
}

func TestHandlerServeHTTPErrors(t *testing.T) {
	s, err := app.NewServer(app.StoreOverride(mainHelper.Store), app.DisableConfigWatch)
	require.Nil(t, err)
	defer s.Shutdown()

	web := New(s, s.AppOptions, s.Router)
	if err != nil {
		panic(err)
	}
	handler := web.NewHandler(handlerForHTTPErrors)

	var flagtests = []struct {
		name     string
		url      string
		mobile   bool
		redirect bool
	}{
		{"redirect on desktop non-api endpoint", "/login/sso/saml", false, true},
		{"not redirect on desktop api endpoint", "/api/v4/test", false, false},
		{"not redirect on mobile non-api endpoint", "/login/sso/saml", true, false},
		{"not redirect on mobile api endpoint", "/api/v4/test", true, false},
	}

	for _, tt := range flagtests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", tt.url, nil)
			if tt.mobile {
				request.Header.Add("X-Mobile-App", "mattermost")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if tt.redirect {
				assert.Equal(t, response.Code, http.StatusFound)
			} else {
				assert.NotContains(t, response.Body.String(), "/error?message=")
			}
		})
	}
}

func handlerForHTTPSecureTransport(c *Context, w http.ResponseWriter, r *http.Request) {
}

func TestHandlerServeHTTPSecureTransport(t *testing.T) {
	s, err := app.NewServer(app.StoreOverride(mainHelper.Store), app.DisableConfigWatch)
	require.Nil(t, err)
	defer s.Shutdown()

	a := s.FakeApp()

	a.UpdateConfig(func(config *model.Config) {
		*config.ServiceSettings.TLSStrictTransport = true
		*config.ServiceSettings.TLSStrictTransportMaxAge = 6000
	})

	web := New(s, s.AppOptions, s.Router)
	if err != nil {
		panic(err)
	}
	handler := web.NewHandler(handlerForHTTPSecureTransport)

	request := httptest.NewRequest("GET", "/api/v4/test", nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	header := response.Header().Get("Strict-Transport-Security")

	if header == "" {
		t.Errorf("Strict-Transport-Security expected but not existent")
	}

	if header != "max-age=6000" {
		t.Errorf("Expected max-age=6000, got %s", header)
	}

	a.UpdateConfig(func(config *model.Config) {
		*config.ServiceSettings.TLSStrictTransport = false
	})

	request = httptest.NewRequest("GET", "/api/v4/test", nil)

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	header = response.Header().Get("Strict-Transport-Security")

	if header != "" {
		t.Errorf("Strict-Transport-Security header is not expected, but returned")
	}
}

func handlerForCSPHeader(c *Context, w http.ResponseWriter, r *http.Request) {
}

func TestHandlerServeCSPHeader(t *testing.T) {
	t.Run("non-static", func(t *testing.T) {
		th := Setup().InitBasic()
		defer th.TearDown()

		web := New(th.Server, th.Server.AppOptions, th.Server.Router)

		handler := Handler{
			GetGlobalAppOptions: web.GetGlobalAppOptions,
			HandleFunc:          handlerForCSPHeader,
			RequireSession:      false,
			TrustRequester:      false,
			RequireMfa:          false,
			IsStatic:            false,
		}

		request := httptest.NewRequest("POST", "/api/v4/test", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, 200, response.Code)
		assert.Empty(t, response.Header()["Content-Security-Policy"])
	})

	t.Run("static, without subpath", func(t *testing.T) {
		th := Setup().InitBasic()
		defer th.TearDown()

		web := New(th.Server, th.Server.AppOptions, th.Server.Router)

		handler := Handler{
			GetGlobalAppOptions: web.GetGlobalAppOptions,
			HandleFunc:          handlerForCSPHeader,
			RequireSession:      false,
			TrustRequester:      false,
			RequireMfa:          false,
			IsStatic:            true,
		}

		request := httptest.NewRequest("POST", "/", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, 200, response.Code)
		assert.Equal(t, response.Header()["Content-Security-Policy"], []string{"frame-ancestors 'self'; script-src 'self' cdn.segment.com/analytics.js/"})
	})

	t.Run("static, with subpath", func(t *testing.T) {
		th := Setup().InitBasic()
		defer th.TearDown()

		th.App.UpdateConfig(func(cfg *model.Config) {
			*cfg.ServiceSettings.SiteURL = *cfg.ServiceSettings.SiteURL + "/subpath"
		})

		web := New(th.Server, th.Server.AppOptions, th.Server.Router)

		handler := Handler{
			GetGlobalAppOptions: web.GetGlobalAppOptions,
			HandleFunc:          handlerForCSPHeader,
			RequireSession:      false,
			TrustRequester:      false,
			RequireMfa:          false,
			IsStatic:            true,
		}

		request := httptest.NewRequest("POST", "/", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, 200, response.Code)
		assert.Equal(t, response.Header()["Content-Security-Policy"], []string{"frame-ancestors 'self'; script-src 'self' cdn.segment.com/analytics.js/"})

		// TODO: It's hard to unit test this now that the CSP directive is effectively
		// decided in Setup(). Circle back to this in master once the memory store is
		// merged, allowing us to mock the desired initial config to take effect in Setup().
		// assert.Contains(t, response.Header()["Content-Security-Policy"], "frame-ancestors 'self'; script-src 'self' cdn.segment.com/analytics.js/ 'sha256-tPOjw+tkVs9axL78ZwGtYl975dtyPHB6LYKAO2R3gR4='")

		th.App.UpdateConfig(func(cfg *model.Config) {
			*cfg.ServiceSettings.SiteURL = *cfg.ServiceSettings.SiteURL + "/subpath2"
		})

		request = httptest.NewRequest("POST", "/", nil)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, 200, response.Code)
		assert.Equal(t, response.Header()["Content-Security-Policy"], []string{"frame-ancestors 'self'; script-src 'self' cdn.segment.com/analytics.js/"})
		// TODO: See above.
		// assert.Contains(t, response.Header()["Content-Security-Policy"], "frame-ancestors 'self'; script-src 'self' cdn.segment.com/analytics.js/ 'sha256-tPOjw+tkVs9axL78ZwGtYl975dtyPHB6LYKAO2R3gR4='", "csp header incorrectly changed after subpath changed")
	})
}
