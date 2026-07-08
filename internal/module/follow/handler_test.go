package follow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dianping/internal/middleware"
	"dianping/internal/module/user"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock Implementations
// =============================================================================

type mockFollowRepository struct {
	followFunc              func(ctx context.Context, userID, followUserID uint64) (bool, error)
	unfollowFunc            func(ctx context.Context, userID, followUserID uint64) (bool, error)
	isFollowedFunc          func(ctx context.Context, userID, followUserID uint64) (bool, error)
	listFollowerUserIDsFunc func(ctx context.Context, userID uint64) ([]uint64, error)
	listFollowedUserIDsFunc func(ctx context.Context, userID uint64) ([]uint64, error)
}

func (m *mockFollowRepository) Follow(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if m.followFunc != nil {
		return m.followFunc(ctx, userID, followUserID)
	}
	return false, nil
}

func (m *mockFollowRepository) Unfollow(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if m.unfollowFunc != nil {
		return m.unfollowFunc(ctx, userID, followUserID)
	}
	return false, nil
}

func (m *mockFollowRepository) IsFollowed(ctx context.Context, userID, followUserID uint64) (bool, error) {
	if m.isFollowedFunc != nil {
		return m.isFollowedFunc(ctx, userID, followUserID)
	}
	return false, nil
}

func (m *mockFollowRepository) ListFollowerUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if m.listFollowerUserIDsFunc != nil {
		return m.listFollowerUserIDsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockFollowRepository) ListFollowedUserIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if m.listFollowedUserIDsFunc != nil {
		return m.listFollowedUserIDsFunc(ctx, userID)
	}
	return nil, nil
}

type mockUserService struct {
	listUsersByIDsFunc func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error)
}

func (m *mockUserService) ListUsersByIDs(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
	if m.listUsersByIDsFunc != nil {
		return m.listUsersByIDsFunc(ctx, userIDs)
	}
	return nil, nil
}

// =============================================================================
// Test Helpers
// =============================================================================

// setUpFollowHandler creates a gin test engine with all follow routes registered.
// All routes are placed under an auth group that injects userID=1 into the context.
// Returns the engine, mock repo, and mock user service.
func setUpFollowHandler(t *testing.T) (*gin.Engine, *mockFollowRepository, *mockUserService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	repo := new(mockFollowRepository)
	userSrv := new(mockUserService)
	service := NewService(repo, userSrv, rdb)
	handler := NewHandler(service)

	r := gin.New()

	auth := r.Group("/api/follow")
	auth.Use(func(ctx *gin.Context) {
		ctx.Set(middleware.CtxUserIDKey, uint64(1))
		ctx.Next()
	})
	{
		auth.POST("/:id/:isFollow", handler.Follow)
		auth.GET("/or/not/:id", handler.IsFollowed)
		auth.GET("/common/:id", handler.FollowCommon)
		auth.GET("/followed", handler.ListFollowedUserIDs)
		auth.GET("/follower", handler.ListFollowerUserIDs)
	}

	return r, repo, userSrv
}

// setUpFollowHandlerNoAuth creates a gin engine without auth middleware,
// used to test cases where no userID is in the context.
func setUpFollowHandlerNoAuth(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, rdb.Close()) })

	service := NewService(new(mockFollowRepository), new(mockUserService), rdb)
	handler := NewHandler(service)

	r := gin.New()
	r.POST("/api/follow/:id/:isFollow", handler.Follow)
	r.GET("/api/follow/or/not/:id", handler.IsFollowed)
	r.GET("/api/follow/common/:id", handler.FollowCommon)
	r.GET("/api/follow/followed", handler.ListFollowedUserIDs)
	r.GET("/api/follow/follower", handler.ListFollowerUserIDs)

	return r
}

// decodebody unmarshals the response body into a generic map.
func decodebody[T any](t *testing.T, w *httptest.ResponseRecorder) map[string]T {
	t.Helper()
	var body map[string]T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// =============================================================================
// Follow
// =============================================================================

func TestHandler_Follow(t *testing.T) {
	t.Run("follow successfully", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			require.Equal(t, uint64(1), userID)
			require.Equal(t, uint64(2), followUserID)
			return true, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/api/follow/2/true", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("unfollow successfully", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.unfollowFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			require.Equal(t, uint64(1), userID)
			require.Equal(t, uint64(2), followUserID)
			return true, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/api/follow/2/false", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
	})

	t.Run("follow yourself returns ErrFollowYourself", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called for self-follow")
			return false, nil
		}

		// userID from context is 1, followUserID from URL is also 1
		req := httptest.NewRequest(http.MethodPost, "/api/follow/1/true", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4601), body["code"].(float64))
		require.Equal(t, "不能关注自己", body["message"])
	})

	t.Run("unfollow yourself returns ErrFollowYourself", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.unfollowFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called for self-unfollow")
			return false, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/api/follow/1/false", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4601), body["code"].(float64))
	})

	t.Run("non-numeric id returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called when param is invalid")
			return false, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/api/follow/abc/true", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("invalid isFollow param returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called when param is invalid")
			return false, nil
		}

		req := httptest.NewRequest(http.MethodPost, "/api/follow/2/xyz", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("missing auth returns ErrUnauthorized", func(t *testing.T) {
		r := setUpFollowHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodPost, "/api/follow/2/true", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("wrong userID type returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { require.NoError(t, rdb.Close()) })

		handler := NewHandler(NewService(new(mockFollowRepository), new(mockUserService), rdb))
		r := gin.New()
		r.POST("/follow/:id/:isFollow", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, "not-a-uint64")
			ctx.Next()
		}, handler.Follow)

		req := httptest.NewRequest(http.MethodPost, "/follow/2/true", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("service error returns ErrInternalSec", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.followFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, fmt.Errorf("db connection lost")
		}

		req := httptest.NewRequest(http.MethodPost, "/api/follow/2/true", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(5000), body["code"].(float64))
	})
}

// =============================================================================
// IsFollowed
// =============================================================================

func TestHandler_IsFollowed(t *testing.T) {
	t.Run("is followed returns true", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			require.Equal(t, uint64(1), userID)
			require.Equal(t, uint64(2), followUserID)
			return true, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/or/not/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, true, data["is_followed"])
	})

	t.Run("is not followed returns false", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/or/not/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].(map[string]any)
		require.Equal(t, false, data["is_followed"])
	})

	t.Run("non-numeric id returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			t.Fatalf("repo should not be called when param is invalid")
			return false, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/or/not/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("missing auth returns ErrUnauthorized", func(t *testing.T) {
		r := setUpFollowHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/follow/or/not/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("wrong userID type returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { require.NoError(t, rdb.Close()) })

		handler := NewHandler(NewService(new(mockFollowRepository), new(mockUserService), rdb))
		r := gin.New()
		r.GET("/or/not/:id", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, "not-a-uint64")
			ctx.Next()
		}, handler.IsFollowed)

		req := httptest.NewRequest(http.MethodGet, "/or/not/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("service error returns ErrInternalSec", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.isFollowedFunc = func(ctx context.Context, userID, followUserID uint64) (bool, error) {
			return false, fmt.Errorf("db connection lost")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/or/not/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(5000), body["code"].(float64))
	})
}

// =============================================================================
// FollowCommon
// =============================================================================

func TestHandler_FollowCommon(t *testing.T) {
	t.Run("common users found", func(t *testing.T) {
		r, repo, userSrv := setUpFollowHandler(t)

		// ensureCache calls repo.ListFollowedUserIDs for userID=1 and userID=2
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			if userID == 1 {
				return []uint64{2, 3, 4}, nil
			}
			return []uint64{2, 4, 5}, nil // userID == 2
		}

		// SInter(follows:1, follows:2) returns {"2", "4"}
		// commonUserIDs = [2, 4]
		userSrv.listUsersByIDsFunc = func(ctx context.Context, userIDs []uint64) ([]user.UserDTO, error) {
			require.ElementsMatch(t, []uint64{2, 4}, userIDs)
			return []user.UserDTO{
				{ID: 2, NickName: "Bob", Icon: "/imgs/bob.png"},
				{ID: 4, NickName: "Dave", Icon: "/imgs/dave.png"},
			}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/common/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 2)
	})

	t.Run("no common users returns empty array", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)

		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			if userID == 1 {
				return []uint64{2, 3}, nil
			}
			return []uint64{4, 5}, nil // no overlap with user 1
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/common/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Empty(t, data)
	})

	t.Run("non-numeric id returns ErrInvalidParam", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			t.Fatalf("repo should not be called when param is invalid")
			return nil, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/common/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("missing auth returns ErrUnauthorized", func(t *testing.T) {
		r := setUpFollowHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/follow/common/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("wrong userID type returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { require.NoError(t, rdb.Close()) })

		handler := NewHandler(NewService(new(mockFollowRepository), new(mockUserService), rdb))
		r := gin.New()
		r.GET("/common/:id", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, "not-a-uint64")
			ctx.Next()
		}, handler.FollowCommon)

		req := httptest.NewRequest(http.MethodGet, "/common/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("repo error returns ErrInternalSec", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/common/2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(5000), body["code"].(float64))
	})
}

// =============================================================================
// ListFollowedUserIDs
// =============================================================================

func TestHandler_ListFollowedUserIDs(t *testing.T) {
	t.Run("returns followed user IDs", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			require.Equal(t, uint64(1), userID)
			return []uint64{2, 3, 4}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/followed", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 3)
	})

	t.Run("returns empty list when no followed users", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return []uint64{}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/followed", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Empty(t, data)
	})

	t.Run("missing auth returns ErrUnauthorized", func(t *testing.T) {
		r := setUpFollowHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/follow/followed", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("wrong userID type returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { require.NoError(t, rdb.Close()) })

		handler := NewHandler(NewService(new(mockFollowRepository), new(mockUserService), rdb))
		r := gin.New()
		r.GET("/followed", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, "not-a-uint64")
			ctx.Next()
		}, handler.ListFollowedUserIDs)

		req := httptest.NewRequest(http.MethodGet, "/followed", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("service error returns ErrInternalSec", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowedUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/followed", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(5000), body["code"].(float64))
	})
}

// =============================================================================
// ListFollowerUserIDs
// =============================================================================

func TestHandler_ListFollowerUserIDs(t *testing.T) {
	t.Run("returns follower user IDs", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowerUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			require.Equal(t, uint64(1), userID)
			return []uint64{5, 6, 7}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/follower", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Len(t, data, 3)
	})

	t.Run("returns empty list when no followers", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowerUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return []uint64{}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/follower", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, true, body["success"])
		data := body["data"].([]any)
		require.Empty(t, data)
	})

	t.Run("missing auth returns ErrUnauthorized", func(t *testing.T) {
		r := setUpFollowHandlerNoAuth(t)

		req := httptest.NewRequest(http.MethodGet, "/api/follow/follower", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4010), body["code"].(float64))
	})

	t.Run("wrong userID type returns ErrInvalidParam", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { require.NoError(t, rdb.Close()) })

		handler := NewHandler(NewService(new(mockFollowRepository), new(mockUserService), rdb))
		r := gin.New()
		r.GET("/follower", func(ctx *gin.Context) {
			ctx.Set(middleware.CtxUserIDKey, "not-a-uint64")
			ctx.Next()
		}, handler.ListFollowerUserIDs)

		req := httptest.NewRequest(http.MethodGet, "/follower", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, float64(4001), body["code"].(float64))
	})

	t.Run("service error returns ErrInternalSec", func(t *testing.T) {
		r, repo, _ := setUpFollowHandler(t)
		repo.listFollowerUserIDsFunc = func(ctx context.Context, userID uint64) ([]uint64, error) {
			return nil, fmt.Errorf("db connection lost")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/follow/follower", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := decodebody[any](t, w)
		require.Equal(t, false, body["success"])
		require.Equal(t, float64(5000), body["code"].(float64))
	})
}
