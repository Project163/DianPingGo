package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/internal/middleware"
	"dianping/internal/module/user"
	"dianping/pkg/validator"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock dependencies for handler tests
// =============================================================================

type mockBlogRepo struct {
	createBlogFunc        func(ctx context.Context, blog *Blog) error
	getBlogByIDFunc       func(ctx context.Context, id uint64) (*Blog, error)
	listHotBlogsFunc      func(ctx context.Context, offset, limit int) ([]Blog, error)
	listBlogsByUserIDFunc func(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error)
	incrementLikedFunc    func(ctx context.Context, blogID uint64) error
	decrementLikedFunc    func(ctx context.Context, blogID uint64) error
	listBlogsByIDsFunc    func(ctx context.Context, blogIDs []uint64) ([]Blog, error)
}

func (m *mockBlogRepo) CreateBlog(ctx context.Context, blog *Blog) error {
	if m.createBlogFunc != nil {
		return m.createBlogFunc(ctx, blog)
	}
	return nil
}

func (m *mockBlogRepo) GetBlogByID(ctx context.Context, id uint64) (*Blog, error) {
	if m.getBlogByIDFunc != nil {
		return m.getBlogByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockBlogRepo) ListHotBlogs(ctx context.Context, offset, limit int) ([]Blog, error) {
	if m.listHotBlogsFunc != nil {
		return m.listHotBlogsFunc(ctx, offset, limit)
	}
	return []Blog{}, nil
}

func (m *mockBlogRepo) ListBlogsByUserID(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error) {
	if m.listBlogsByUserIDFunc != nil {
		return m.listBlogsByUserIDFunc(ctx, userID, offset, limit)
	}
	return []Blog{}, nil
}

func (m *mockBlogRepo) IncrementLiked(ctx context.Context, blogID uint64) error {
	if m.incrementLikedFunc != nil {
		return m.incrementLikedFunc(ctx, blogID)
	}
	return nil
}

func (m *mockBlogRepo) DecrementLiked(ctx context.Context, blogID uint64) error {
	if m.decrementLikedFunc != nil {
		return m.decrementLikedFunc(ctx, blogID)
	}
	return nil
}

func (m *mockBlogRepo) ListBlogsByIDs(ctx context.Context, blogIDs []uint64) ([]Blog, error) {
	if m.listBlogsByIDsFunc != nil {
		return m.listBlogsByIDsFunc(ctx, blogIDs)
	}
	return []Blog{}, nil
}

type mockUserSrv struct {
	getUserByIDFunc    func(ctx context.Context, userID uint64) (*user.UserDTO, error)
	listUsersByIDsFunc func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error)
}

func (m *mockUserSrv) GetUserByID(ctx context.Context, userID uint64) (*user.UserDTO, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockUserSrv) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
	if m.listUsersByIDsFunc != nil {
		return m.listUsersByIDsFunc(ctx, userIDs)
	}
	return nil, nil
}

type mockFollowSrv struct {
	listFollowedUserIDsFunc func(ctx context.Context, userID uint64) ([]uint64, error)
	listFollowerUserIDsFunc func(ctx context.Context, userID uint64) ([]uint64, error)
}

func (m *mockFollowSrv) ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if m.listFollowedUserIDsFunc != nil {
		return m.listFollowedUserIDsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockFollowSrv) ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if m.listFollowerUserIDsFunc != nil {
		return m.listFollowerUserIDsFunc(ctx, userID)
	}
	return nil, nil
}

// =============================================================================
// Test setup helpers
// =============================================================================

// setUpBlogHandler creates a gin engine with all blog handler routes registered.
// Auth routes inject userID=1 via middleware.CtxUserIDKey.
// Returns the engine and mock dependencies for test case customization.
func setUpBlogHandler(t *testing.T) (*gin.Engine, *mockBlogRepo, *mockUserSrv, *mockFollowSrv, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	validator.InitValidator()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	repo := &mockBlogRepo{}
	userSrv := &mockUserSrv{}
	followSrv := &mockFollowSrv{}

	srv := NewService(repo, rdb, userSrv, followSrv)
	handler := NewHandler(srv)

	r := gin.New()

	// Auth routes: inject userID=1 into context
	auth := r.Group("/api/blog")
	auth.Use(func(ctx *gin.Context) {
		ctx.Set(middleware.CtxUserIDKey, uint64(1))
		ctx.Next()
	})
	{
		auth.POST("", handler.CreateBlog)
		auth.PUT("/like/:id", handler.LikeBlog)
		auth.GET("/of/me", handler.GetBlogSelf)
		auth.GET("/of/user/:id", handler.GetBlogByUserID)
		auth.GET("/:id", handler.GetBlogByID)
		auth.GET("/of/follow", handler.GetBlogOfFollow)
	}

	// Hot blogs route (optional auth — placed in auth group for convenience, gives userID=1)
	auth.GET("/of/hot", handler.GetBlogsHot)

	// No-auth routes
	r.GET("/api/blog/likes/:id", handler.GetBlogLikesByID)

	return r, repo, userSrv, followSrv, mr
}

// setUpBlogHandlerNoAuth creates a gin engine without auth middleware.
// Used to test unauthorized access scenarios.
func setUpBlogHandlerNoAuth(t *testing.T) (*gin.Engine, *mockBlogRepo, *mockUserSrv, *mockFollowSrv, *miniredis.Miniredis) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	validator.InitValidator()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		require.NoError(t, rdb.Close())
	})

	repo := &mockBlogRepo{}
	userSrv := &mockUserSrv{}
	followSrv := &mockFollowSrv{}

	srv := NewService(repo, rdb, userSrv, followSrv)
	handler := NewHandler(srv)

	r := gin.New()
	r.POST("/api/blog", handler.CreateBlog)
	r.PUT("/api/blog/like/:id", handler.LikeBlog)
	r.GET("/api/blog/of/me", handler.GetBlogSelf)
	r.GET("/api/blog/of/hot", handler.GetBlogsHot)
	r.GET("/api/blog/of/user/:id", handler.GetBlogByUserID)
	r.GET("/api/blog/likes/:id", handler.GetBlogLikesByID)
	r.GET("/api/blog/:id", handler.GetBlogByID)
	r.GET("/api/blog/of/follow", handler.GetBlogOfFollow)

	return r, repo, userSrv, followSrv, mr
}

// decodebody unmarshals the response body into map[string]any.
func decodebody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// CreateBlog
// =============================================================================

func TestHandler_CreateBlog(t *testing.T) {
	t.Run("create blog successfully", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.createBlogFunc = func(ctx context.Context, blog *Blog) error {
			require.Equal(t, uint64(1), blog.UserId)
			require.Equal(t, uint64(100), blog.ShopID)
			require.Equal(t, "Test Title", blog.Title)
			require.Equal(t, "Test Content", blog.Content)
			blog.ID = 1
			return nil
		}

		reqBody := []byte(`{"shop_id":100,"title":"Test Title","images":"/imgs/test.png","content":"Test Content"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/blog", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		require.Equal(t, float64(2000), body["code"])
	})

	t.Run("create blog with invalid params returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.createBlogFunc = func(ctx context.Context, blog *Blog) error {
			require.FailNow(t, "service should not be called when binding fails")
			return nil
		}

		reqBody := []byte(`{"shop_id":100}`)
		req := httptest.NewRequest(http.MethodPost, "/api/blog", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
		require.Equal(t, "请求参数错误", body["message"])
	})

	t.Run("create blog without auth returns ErrUnauthorized", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		reqBody := []byte(`{"shop_id":100,"title":"Test","images":"/imgs/test.png","content":"Test Content"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/blog", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("create blog with service error returns ErrInternalSec", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.createBlogFunc = func(ctx context.Context, blog *Blog) error {
			return errors.New("db connection lost")
		}

		reqBody := []byte(`{"shop_id":100,"title":"Test Title","images":"/imgs/test.png","content":"Test Content"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/blog", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(5000), body["code"].(float64))
	})
}

// =============================================================================
// LikeBlog
// =============================================================================

func TestHandler_LikeBlog(t *testing.T) {
	t.Run("like blog successfully", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.incrementLikedFunc = func(ctx context.Context, blogID uint64) error {
			require.Equal(t, uint64(1), blogID)
			return nil
		}

		req := httptest.NewRequest(http.MethodPut, "/api/blog/like/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("like blog with non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodPut, "/api/blog/like/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("like blog without auth returns ErrUnauthorized", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodPut, "/api/blog/like/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("like blog with service error propagates", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.incrementLikedFunc = func(ctx context.Context, blogID uint64) error {
			return errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodPut, "/api/blog/like/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetBlogSelf
// =============================================================================

func TestHandler_GetBlogSelf(t *testing.T) {
	t.Run("get self blogs successfully", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.listBlogsByUserIDFunc = func(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error) {
			require.Equal(t, uint64(1), userID)
			require.Equal(t, 0, offset)
			require.Equal(t, MaxPageSize, limit)
			return []Blog{
				{ID: 10, UserId: 1, Title: "My Blog", Content: "Hello", Images: "/imgs/a.png"},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/me?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 1)
	})

	t.Run("get self blogs without auth returns ErrUnauthorized", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/me", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("get self blogs with invalid current param returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/me?current=abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get self blogs with current less than 1 returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/me?current=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get self blogs with service error propagates", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.listBlogsByUserIDFunc = func(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error) {
			return nil, errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/me?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetBlogsHot
// =============================================================================

func TestHandler_GetBlogsHot(t *testing.T) {
	t.Run("get hot blogs successfully", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.listHotBlogsFunc = func(ctx context.Context, offset, limit int) ([]Blog, error) {
			require.Equal(t, 0, offset)
			require.Equal(t, MaxPageSize, limit)
			return []Blog{
				{ID: 1, UserId: 2, Title: "Hot Blog", Content: "Great!", Images: "/imgs/hot.png", Liked: 100},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/hot?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 1)
	})

	t.Run("get hot blogs with invalid current param returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/hot?current=abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get hot blogs with service error propagates", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.listHotBlogsFunc = func(ctx context.Context, offset, limit int) ([]Blog, error) {
			return nil, errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/hot?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetBlogByUserID
// =============================================================================

func TestHandler_GetBlogByUserID(t *testing.T) {
	t.Run("get blogs by user ID successfully", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.listBlogsByUserIDFunc = func(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error) {
			require.Equal(t, uint64(2), userID)
			require.Equal(t, 0, offset)
			return []Blog{
				{ID: 20, UserId: 2, Title: "User2 Blog", Content: "Hi", Images: "/imgs/b.png"},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/user/2?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 1)
	})

	t.Run("get blogs by user ID with non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/user/abc?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get blogs by user ID without auth returns ErrUnauthorized", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/user/2?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("get blogs by user ID with invalid current param returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/user/2?current=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get blogs by user ID with service error propagates", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.listBlogsByUserIDFunc = func(ctx context.Context, userID uint64, offset, limit int) ([]Blog, error) {
			return nil, errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/user/2?current=1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}

// =============================================================================
// GetBlogLikesByID
// =============================================================================

func TestHandler_GetBlogLikesByID(t *testing.T) {
	t.Run("get blog likes successfully with users", func(t *testing.T) {
		r, _, userSrv, _, mr := setUpBlogHandlerNoAuth(t)

		// Populate Redis ZSet with likes
		key := BizBlogLikedKey + "1"
		mr.ZAdd(key, 1000, "100")
		mr.ZAdd(key, 2000, "200")

		userSrv.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
			require.Len(t, userIDs, 2)
			return []user.UserDTO{
				{ID: 100, NickName: "Alice", Icon: "/imgs/a.png"},
				{ID: 200, NickName: "Bob", Icon: "/imgs/b.png"},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/likes/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 2)
	})

	t.Run("get blog likes with no likes returns empty array", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/likes/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Empty(t, data)
	})

	t.Run("get blog likes with non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/likes/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})
}

// =============================================================================
// GetBlogByID
// =============================================================================

func TestHandler_GetBlogByID(t *testing.T) {
	t.Run("get blog by ID successfully", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.getBlogByIDFunc = func(ctx context.Context, id uint64) (*Blog, error) {
			require.Equal(t, uint64(1), id)
			return &Blog{
				ID:      1,
				UserId:  2,
				ShopID:  100,
				Title:   "Test Blog",
				Content: "Blog Content",
				Images:  "/imgs/test.png",
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, float64(1), data["id"])
		require.Equal(t, "Test Blog", data["title"])
	})

	t.Run("get blog by non-numeric ID returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/abc", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get blog by ID without auth returns ErrUnauthorized", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("get blog by ID not found returns ErrBlogNotFound", func(t *testing.T) {
		r, repo, _, _, _ := setUpBlogHandler(t)
		repo.getBlogByIDFunc = func(ctx context.Context, id uint64) (*Blog, error) {
			return nil, nil // not found
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/999", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4501), body["code"].(float64))
	})
}

// =============================================================================
// GetBlogOfFollow
// =============================================================================

func TestHandler_GetBlogOfFollow(t *testing.T) {
	t.Run("get blogs of follow successfully", func(t *testing.T) {
		r, repo, _, _, mr := setUpBlogHandler(t)

		// Populate Redis ZSet with feed entries
		feedKey := BizBlogFeedKey + "1"
		mr.ZAdd(feedKey, 3000, "10")
		mr.ZAdd(feedKey, 2000, "20")

		repo.listBlogsByIDsFunc = func(ctx context.Context, blogIDs []uint64) ([]Blog, error) {
			require.Len(t, blogIDs, 2)
			return []Blog{
				{ID: 10, UserId: 2, Title: "Followed Blog 1", Content: "Content 1", Images: "/imgs/1.png"},
				{ID: 20, UserId: 3, Title: "Followed Blog 2", Content: "Content 2", Images: "/imgs/2.png"},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/follow?last_id=9999&offset=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.NotNil(t, data["list"])
		list := data["list"].([]any)
		require.Len(t, list, 2)
	})

	t.Run("get blogs of follow with empty feed returns empty result", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/follow?last_id=9999&offset=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody(t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		list := data["list"].([]any)
		require.Empty(t, list)
	})

	t.Run("get blogs of follow without auth returns ErrUnauthorized", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/follow?last_id=9999&offset=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("get blogs of follow with invalid last_id returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/follow?last_id=0&offset=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get blogs of follow with negative offset returns ErrInvalidParam", func(t *testing.T) {
		r, _, _, _, _ := setUpBlogHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/follow?last_id=9999&offset=-1", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("get blogs of follow with service error propagates", func(t *testing.T) {
		r, repo, _, _, mr := setUpBlogHandler(t)

		// Populate Redis feed so service calls repo
		feedKey := BizBlogFeedKey + "1"
		mr.ZAdd(feedKey, 3000, "10")

		repo.listBlogsByIDsFunc = func(ctx context.Context, blogIDs []uint64) ([]Blog, error) {
			return nil, errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/blog/of/follow?last_id=9999&offset=0", nil)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody(t, w)
		require.Equal(t, false, body["success"])
	})
}
