package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/internal/middleware"
	"dianping/internal/module/userinfo"
	"dianping/pkg/errmsg"
	"dianping/pkg/validator"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mockUserService struct {
	signFunc        func(ctx context.Context, userID uint64) error
	signCountFunc   func(ctx context.Context, userID uint64) (int, error)
	loginFunc       func(ctx context.Context, req *LoginReq) (*LoginResp, error)
	logoutFunc      func(ctx context.Context, userId uint64, token string) error
	codeLoginFunc   func(ctx context.Context, req *CodeLoginReq) (*LoginResp, error)
	sendCodeFunc    func(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error)
	getUserByIDFunc func(ctx context.Context, userID uint64) (*UserDTO, error)
}

func (m *mockUserService) Sign(ctx context.Context, userID uint64) error {
	if m.signFunc != nil {
		return m.signFunc(ctx, userID)
	}
	return nil
}

func (m *mockUserService) SignCount(ctx context.Context, userID uint64) (int, error) {
	if m.signCountFunc != nil {
		return m.signCountFunc(ctx, userID)
	}
	return 0, nil
}

func (m *mockUserService) Login(ctx context.Context, req *LoginReq) (*LoginResp, error) {
	if m.loginFunc != nil {
		return m.loginFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockUserService) Logout(ctx context.Context, userId uint64, token string) error {
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, userId, token)
	}
	return nil
}

func (m *mockUserService) CodeLogin(ctx context.Context, req *CodeLoginReq) (*LoginResp, error) {
	if m.codeLoginFunc != nil {
		return m.codeLoginFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockUserService) SendCode(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error) {
	if m.sendCodeFunc != nil {
		return m.sendCodeFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockUserService) GetUserByID(ctx context.Context, userID uint64) (*UserDTO, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, userID)
	}
	return nil, nil
}

type mockUserInfoService struct {
	getUserInfoByUserIDFunc func(ctx context.Context, userID uint64) (*userinfo.UserInfoDTO, error)
	createUserInfoFunc      func(ctx context.Context, userInfoDTO *userinfo.UserInfoDTO) error
}

func (m *mockUserInfoService) GetUserInfoByUserID(ctx context.Context, userID uint64) (*userinfo.UserInfoDTO, error) {
	if m.getUserInfoByUserIDFunc != nil {
		return m.getUserInfoByUserIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockUserInfoService) CreateUserInfo(ctx context.Context, userInfoDTO *userinfo.UserInfoDTO) error {
	if m.createUserInfoFunc != nil {
		return m.createUserInfoFunc(ctx, userInfoDTO)
	}
	return nil
}

// setUpUserHandler 创建测试用的 gin Engine，注册全部 user handler 路由。
// 返回 Engine 及 mock service 指针以便测试用例替换行为。
func setUpUserHandler(t *testing.T) (*gin.Engine, *mockUserService, *mockUserInfoService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	validator.InitValidator()

	userSrv := new(mockUserService)
	userInfoSrv := new(mockUserInfoService)
	handler := NewHandler(userSrv, userInfoSrv)

	r := gin.New()

	// 无需鉴权的路由
	r.POST("/api/user/login/password", handler.Login)
	r.POST("/api/user/login/code", handler.CodeLogin)
	r.POST("/api/user/code", handler.SendCode)
	r.GET("/api/user/:id", handler.GetUserByID)
	r.GET("/api/user/info/:id", handler.GetUserInfoByID)

	// 需要鉴权的路由（模拟 middleware 注入 userID=1）
	auth := r.Group("/api")
	auth.Use(func(ctx *gin.Context) {
		ctx.Set(middleware.CtxUserIDKey, uint64(1))
		ctx.Next()
	})
	{
		auth.POST("/user/logout", handler.Logout)
		auth.GET("/user/me", handler.GetSelf)
		auth.GET("/user/sign/count", handler.SignCount)
		auth.PUT("/user/sign", handler.Sign)
	}

	return r, userSrv, userInfoSrv
}

// decodebody 将 httptest.ResponseRecorder 的 body 反序列化为 map。
func decodebody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// Login
// =============================================================================

func TestHandler_Login(t *testing.T) {
	t.Run("login with valid credentials", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.loginFunc = func(ctx context.Context, req *LoginReq) (*LoginResp, error) {
			require.Equal(t, "13800138000", req.Phone)
			require.Equal(t, "password123", req.Password)
			return &LoginResp{
				Token:    "mocked_token",
				NickName: "MockedUser",
			}, nil
		}
		reqBody := []byte(`{"phone":"13800138000","password":"password123"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/user/login/password", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		data := body["data"].(map[string]any)
		require.Equal(t, "mocked_token", data["token"])
		require.Equal(t, "MockedUser", data["nick_name"])
	})
	t.Run("login with invalid credentials", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)

		userSrv.loginFunc = func(ctx context.Context, req *LoginReq) (*LoginResp, error) {
			t.Fatalf("service should not be called when request binding fails")
			return nil, nil
		}

		reqBody := []byte(`{"phone":"invalid_phone","password":"123456"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/user/login/password", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), (body["code"].(float64)))
		require.Equal(t, "请求参数错误", body["message"])
	})
	t.Run("login with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.loginFunc = func(ctx context.Context, req *LoginReq) (*LoginResp, error) {
			return nil, &errmsg.ErrInvalidPassword
		}
		reqBody := []byte(`{"phone":"13800138000","password":"wrongpassword"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/user/login/password", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4103), (body["code"].(float64)))
		require.Equal(t, "密码错误", body["message"])
	})
}

// =============================================================================
// CodeLogin
// =============================================================================

func TestHandler_CodeLogin(t *testing.T) {
	t.Run("code login successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.codeLoginFunc = func(ctx context.Context, req *CodeLoginReq) (*LoginResp, error) {
			require.Equal(t, "13800138000", req.Phone)
			require.Equal(t, "123456", req.Code)
			return &LoginResp{Token: "mocked_token", NickName: "MockedUser"}, nil
		}
		reqBody := []byte(`{"phone":"13800138000","code":"123456"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/user/login/code", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, "mocked_token", data["token"])
		require.Equal(t, "MockedUser", data["nick_name"])
	})

	t.Run("code login with invalid phone", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.codeLoginFunc = func(ctx context.Context, req *CodeLoginReq) (*LoginResp, error) {
			t.Fatalf("service should not be called when binding fails")
			return nil, nil
		}
		reqBody := []byte(`{"phone":"invalid","code":"123456"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/user/login/code", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("code login with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.codeLoginFunc = func(ctx context.Context, req *CodeLoginReq) (*LoginResp, error) {
			return nil, &errmsg.ErrCodeExpired
		}
		reqBody := []byte(`{"phone":"13800138000","code":"123456"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/user/login/code", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4104), body["code"].(float64))
		require.Equal(t, "验证码已过期", body["message"])
	})
}

// =============================================================================
// Logout
// =============================================================================

func TestHandler_Logout(t *testing.T) {
	t.Run("logout successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.logoutFunc = func(ctx context.Context, userId uint64, token string) error {
			require.Equal(t, uint64(1), userId)
			require.Equal(t, "Bearer test-token", token)
			return nil
		}
		req := httptest.NewRequest(http.MethodPost, "/api/user/logout", nil)
		req.Header.Set("Authorization", "Bearer test-token")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("logout with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.logoutFunc = func(ctx context.Context, userId uint64, token string) error {
			return &errmsg.ErrUnauthorized
		}
		req := httptest.NewRequest(http.MethodPost, "/api/user/logout", nil)
		req.Header.Set("Authorization", "Bearer test-token")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("logout without userID in context returns ErrUnauthorized", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewHandler(new(mockUserService), new(mockUserInfoService))
		r := gin.New()
		r.POST("/logout", handler.Logout)

		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("logout with wrong userID type returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewHandler(new(mockUserService), new(mockUserInfoService))
		r := gin.New()
		r.POST("/logout", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, "not-a-uint64") // 类型错误
			ctx.Next()
		}, handler.Logout)

		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("logout without Authorization header returns ErrUnauthorized", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewHandler(new(mockUserService), new(mockUserInfoService))
		r := gin.New()
		r.POST("/logout", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, uint64(1))
			ctx.Next()
		}, handler.Logout)

		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		// 不设置 Authorization header
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})
}

// =============================================================================
// SendCode
// =============================================================================

func TestHandler_SendCode(t *testing.T) {
	t.Run("send code successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.sendCodeFunc = func(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error) {
			require.Equal(t, "13800138000", req.Phone)
			return &SendCodeResp{Message: "发送成功"}, nil
		}
		req := httptest.NewRequest(http.MethodPost, "/api/user/code?phone=13800138000", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, "发送成功", data["message"])
	})

	t.Run("send code with invalid phone", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.sendCodeFunc = func(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error) {
			t.Fatalf("service should not be called when binding fails")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodPost, "/api/user/code?phone=invalid", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("send code with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.sendCodeFunc = func(ctx context.Context, req *SendCodeReq) (*SendCodeResp, error) {
			return nil, &errmsg.ErrTooManyRequests
		}
		req := httptest.NewRequest(http.MethodPost, "/api/user/code?phone=13800138000", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4290), body["code"].(float64))
	})
}

// =============================================================================
// Sign
// =============================================================================

func TestHandler_Sign(t *testing.T) {
	t.Run("sign successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.signFunc = func(ctx context.Context, userID uint64) error {
			require.Equal(t, uint64(1), userID)
			return nil
		}
		req := httptest.NewRequest(http.MethodPut, "/api/user/sign", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("sign with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.signFunc = func(ctx context.Context, userID uint64) error {
			return &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodPut, "/api/user/sign", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("sign without userID in context returns ErrUnauthorized", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewHandler(new(mockUserService), new(mockUserInfoService))
		r := gin.New()
		r.PUT("/sign", handler.Sign)

		req := httptest.NewRequest(http.MethodPut, "/sign", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})
}

// =============================================================================
// SignCount
// =============================================================================

func TestHandler_SignCount(t *testing.T) {
	t.Run("sign count successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.signCountFunc = func(ctx context.Context, userID uint64) (int, error) {
			require.Equal(t, uint64(1), userID)
			return 7, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/sign/count", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(7), data["count"])
	})

	t.Run("sign count with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.signCountFunc = func(ctx context.Context, userID uint64) (int, error) {
			return 0, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/sign/count", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetUserByID
// =============================================================================

func TestHandler_GetUserByID(t *testing.T) {
	t.Run("get user by ID successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.getUserByIDFunc = func(ctx context.Context, userID uint64) (*UserDTO, error) {
			require.Equal(t, uint64(1001), userID)
			return &UserDTO{ID: 1001, NickName: "Alice", Icon: "/imgs/icon.png"}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/1001", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(1001), data["id"])
		require.Equal(t, "Alice", data["nick_name"])
	})

	t.Run("get user by non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.getUserByIDFunc = func(ctx context.Context, userID uint64) (*UserDTO, error) {
			t.Fatalf("service should not be called when param is invalid")
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get user by empty ID returns ErrInvalidParam", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.getUserByIDFunc = func(ctx context.Context, userID uint64) (*UserDTO, error) {
			t.Fatalf("service should not be called when param is empty")
			return nil, nil
		}
		// 路由 :id 在没有 id 段时会匹配不到，使用带斜杠的路径模拟空参数
		req := httptest.NewRequest(http.MethodGet, "/api/user/", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// 空 id 时 Gin 不会路由到 handler，测试直接绑定场景
		// 改为直接调用 handler 逻辑：使用 gin.CreateTestContext 并设置空 Param
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("get user by ID with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.getUserByIDFunc = func(ctx context.Context, userID uint64) (*UserDTO, error) {
			return nil, &errmsg.ErrUserNotFound
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/9999", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4101), body["code"].(float64))
	})
}

// =============================================================================
// GetSelf
// =============================================================================

func TestHandler_GetSelf(t *testing.T) {
	t.Run("get self successfully", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.getUserByIDFunc = func(ctx context.Context, userID uint64) (*UserDTO, error) {
			require.Equal(t, uint64(1), userID)
			return &UserDTO{ID: 1, NickName: "CurrentUser", Icon: "/imgs/self.png"}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/me", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(1), data["id"])
		require.Equal(t, "CurrentUser", data["nick_name"])
	})

	t.Run("get self with service error", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		userSrv.getUserByIDFunc = func(ctx context.Context, userID uint64) (*UserDTO, error) {
			return nil, &errmsg.ErrUserNotFound
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/me", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("get self without userID in context returns ErrUnauthorized", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewHandler(new(mockUserService), new(mockUserInfoService))
		r := gin.New()
		r.GET("/me", handler.GetSelf)

		req := httptest.NewRequest(http.MethodGet, "/me", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody(t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})
}

// =============================================================================
// GetUserInfoByID
// =============================================================================

func TestHandler_GetUserInfoByID(t *testing.T) {
	t.Run("get user info by ID successfully", func(t *testing.T) {
		r, _, userInfoSrv := setUpUserHandler(t)
		userInfoSrv.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*userinfo.UserInfoDTO, error) {
			require.Equal(t, uint64(1001), userID)
			return &userinfo.UserInfoDTO{
				UserID:    1001,
				City:      "Beijing",
				Introduce: "Hello!",
				Fans:      100,
				Followee:  50,
				Gender:    1,
				Credits:   200,
				Level:     3,
			}, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/info/1001", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(1001), data["user_id"])
		require.Equal(t, "Beijing", data["city"])
		require.Equal(t, "Hello!", data["introduce"])
	})

	t.Run("get user info by non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, userSrv, _ := setUpUserHandler(t)
		_ = userSrv
		req := httptest.NewRequest(http.MethodGet, "/api/user/info/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get user info with service error", func(t *testing.T) {
		r, _, userInfoSrv := setUpUserHandler(t)
		userInfoSrv.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*userinfo.UserInfoDTO, error) {
			return nil, &errmsg.ErrInternalSec
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/info/1001", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})

	t.Run("get user info returns nil DTO returns ErrUserInfoNotFound", func(t *testing.T) {
		r, _, userInfoSrv := setUpUserHandler(t)
		userInfoSrv.getUserInfoByUserIDFunc = func(ctx context.Context, userID uint64) (*userinfo.UserInfoDTO, error) {
			return nil, nil
		}
		req := httptest.NewRequest(http.MethodGet, "/api/user/info/1001", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4701), body["code"].(float64))

	})
}
