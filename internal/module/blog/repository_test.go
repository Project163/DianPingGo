//go:build integration

package blog

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setUpRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()

	ctx := context.Background()

	// Start MySQL container with test database
	ctr, err := tcmysql.Run(ctx,
		"mysql:8.0.36",
		tcmysql.WithDatabase("dianping_test"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
		tcmysql.WithScripts(filepath.Join("testdata", "schema.sql")),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, tc.TerminateContainer(ctr))
	})

	// Connect to MySQL using GORM
	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "loc=Local", "charset=utf8mb4")
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// Begin transaction for test isolation
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() {
		require.NoError(t, tx.Rollback().Error)
	})

	return NewRepository(tx), tx
}

// =============================================================================
// CreateBlog
// =============================================================================

func TestRepository_CreateBlog(t *testing.T) {
	t.Run("create blog successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		blog := &Blog{
			ShopID:  100,
			UserId:  1,
			Title:   "Test Blog",
			Content: "This is a test blog content",
			Images:  "/imgs/test.png",
		}
		err := repo.CreateBlog(ctx, blog)
		require.NoError(t, err)
		require.NotZero(t, blog.ID)

		// Verify blog was inserted into database
		var got Blog
		err = db.WithContext(ctx).Where("id = ?", blog.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, blog.ShopID, got.ShopID)
		require.Equal(t, blog.UserId, got.UserId)
		require.Equal(t, blog.Title, got.Title)
		require.Equal(t, blog.Content, got.Content)
		require.Equal(t, blog.Images, got.Images)
	})

	t.Run("create blog with default liked and comments", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		blog := &Blog{
			ShopID:  200,
			UserId:  2,
			Title:   "Default Values Blog",
			Content: "Testing default values",
			Images:  "/imgs/default.png",
		}
		err := repo.CreateBlog(ctx, blog)
		require.NoError(t, err)

		var got Blog
		err = db.WithContext(ctx).Where("id = ?", blog.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, 0, got.Liked)
		require.Equal(t, 0, got.Comments)
	})
}

// =============================================================================
// GetBlogByID
// =============================================================================

func TestRepository_GetBlogByID(t *testing.T) {
	t.Run("get blog by existing ID", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Seed a blog
		seed := &Blog{
			ShopID:  100,
			UserId:  1,
			Title:   "Seed Blog",
			Content: "Seed content",
			Images:  "/imgs/seed.png",
			Liked:   5,
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.GetBlogByID(ctx, seed.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, seed.ID, got.ID)
		require.Equal(t, seed.ShopID, got.ShopID)
		require.Equal(t, seed.UserId, got.UserId)
		require.Equal(t, seed.Title, got.Title)
		require.Equal(t, seed.Content, got.Content)
		require.Equal(t, seed.Images, got.Images)
		require.Equal(t, seed.Liked, got.Liked)
	})

	t.Run("get blog by non-existent ID returns nil", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetBlogByID(ctx, 999999)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("get blog by zero ID returns nil", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.GetBlogByID(ctx, 0)
		require.NoError(t, err)
		require.Nil(t, got)
	})
}

// =============================================================================
// ListHotBlogs
// =============================================================================

func TestRepository_ListHotBlogs(t *testing.T) {
	t.Run("list hot blogs ordered by liked DESC", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Seed blogs with different like counts
		blogs := []Blog{
			{ShopID: 100, UserId: 1, Title: "Low", Content: "C", Liked: 10},
			{ShopID: 200, UserId: 2, Title: "High", Content: "C", Liked: 100},
			{ShopID: 300, UserId: 3, Title: "Mid", Content: "C", Liked: 50},
		}
		err := db.WithContext(ctx).Create(&blogs).Error
		require.NoError(t, err)

		got, err := repo.ListHotBlogs(ctx, 0, 10)
		require.NoError(t, err)
		require.Len(t, got, 3)
		// Should be ordered by liked DESC
		require.Equal(t, 100, got[0].Liked)
		require.Equal(t, "High", got[0].Title)
		require.Equal(t, 50, got[1].Liked)
		require.Equal(t, "Mid", got[1].Title)
		require.Equal(t, 10, got[2].Liked)
		require.Equal(t, "Low", got[2].Title)
	})

	t.Run("list hot blogs with offset and limit", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Seed blogs
		blogs := []Blog{
			{ShopID: 100, UserId: 1, Title: "A", Content: "C", Liked: 10},
			{ShopID: 200, UserId: 2, Title: "B", Content: "C", Liked: 20},
			{ShopID: 300, UserId: 3, Title: "C", Content: "C", Liked: 30},
		}
		err := db.WithContext(ctx).Create(&blogs).Error
		require.NoError(t, err)

		// Offset 1, limit 1 should return the second most liked
		got, err := repo.ListHotBlogs(ctx, 1, 1)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, 20, got[0].Liked)
	})

	t.Run("list hot blogs with empty table", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.ListHotBlogs(ctx, 0, 10)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

// =============================================================================
// ListBlogsByUserID
// =============================================================================

func TestRepository_ListBlogsByUserID(t *testing.T) {
	t.Run("list blogs by user ID ordered by update_time DESC", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Seed blogs for specific user
		blogs := []Blog{
			{ShopID: 100, UserId: 1, Title: "First", Content: "C"},
			{ShopID: 200, UserId: 1, Title: "Second", Content: "C"},
			{ShopID: 300, UserId: 2, Title: "Other User", Content: "C"},
		}
		err := db.WithContext(ctx).Create(&blogs).Error
		require.NoError(t, err)

		got, err := repo.ListBlogsByUserID(ctx, 1, 0, 10)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, uint64(1), got[0].UserId)
		require.Equal(t, uint64(1), got[1].UserId)
	})

	t.Run("list blogs by user ID with offset and limit", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		blogs := []Blog{
			{ShopID: 100, UserId: 5, Title: "B1", Content: "C"},
			{ShopID: 200, UserId: 5, Title: "B2", Content: "C"},
			{ShopID: 300, UserId: 5, Title: "B3", Content: "C"},
		}
		err := db.WithContext(ctx).Create(&blogs).Error
		require.NoError(t, err)

		got, err := repo.ListBlogsByUserID(ctx, 5, 0, 2)
		require.NoError(t, err)
		require.Len(t, got, 2)

		got2, err := repo.ListBlogsByUserID(ctx, 5, 2, 2)
		require.NoError(t, err)
		require.Len(t, got2, 1)
	})

	t.Run("list blogs by non-existent user ID returns empty", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.ListBlogsByUserID(ctx, 999, 0, 10)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

// =============================================================================
// IncrementLiked
// =============================================================================

func TestRepository_IncrementLiked(t *testing.T) {
	t.Run("increment liked count for existing blog", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Blog{
			ShopID:  100,
			UserId:  1,
			Title:   "Like Test",
			Content: "C",
			Liked:   5,
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		err = repo.IncrementLiked(ctx, seed.ID)
		require.NoError(t, err)

		var got Blog
		err = db.WithContext(ctx).Where("id = ?", seed.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, 6, got.Liked)
	})

	t.Run("increment liked count multiple times", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Blog{
			ShopID:  100,
			UserId:  1,
			Title:   "Multi Like",
			Content: "C",
			Liked:   0,
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			err = repo.IncrementLiked(ctx, seed.ID)
			require.NoError(t, err)
		}

		var got Blog
		err = db.WithContext(ctx).Where("id = ?", seed.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, 3, got.Liked)
	})

	t.Run("increment liked for non-existent blog returns no error", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// GORM UpdateColumn returns no error even if no rows matched
		err := repo.IncrementLiked(ctx, 999999)
		require.NoError(t, err)
	})
}

// =============================================================================
// DecrementLiked
// =============================================================================

func TestRepository_DecrementLiked(t *testing.T) {
	t.Run("decrement liked count for existing blog", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Blog{
			ShopID:  100,
			UserId:  1,
			Title:   "Unlike Test",
			Content: "C",
			Liked:   10,
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		err = repo.DecrementLiked(ctx, seed.ID)
		require.NoError(t, err)

		var got Blog
		err = db.WithContext(ctx).Where("id = ?", seed.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, 9, got.Liked)
	})

	t.Run("decrement liked count multiple times", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Blog{
			ShopID:  100,
			UserId:  1,
			Title:   "Multi Unlike",
			Content: "C",
			Liked:   5,
		}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			err = repo.DecrementLiked(ctx, seed.ID)
			require.NoError(t, err)
		}

		var got Blog
		err = db.WithContext(ctx).Where("id = ?", seed.ID).First(&got).Error
		require.NoError(t, err)
		require.Equal(t, 2, got.Liked)
	})

	t.Run("decrement liked for non-existent blog returns no error", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		err := repo.DecrementLiked(ctx, 999999)
		require.NoError(t, err)
	})
}

// =============================================================================
// ListBlogsByIDs
// =============================================================================

func TestRepository_ListBlogsByIDs(t *testing.T) {
	t.Run("list blogs by IDs returns matching blogs", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Seed multiple blogs
		blogs := []Blog{
			{ShopID: 100, UserId: 1, Title: "A", Content: "CA"},
			{ShopID: 200, UserId: 2, Title: "B", Content: "CB"},
			{ShopID: 300, UserId: 3, Title: "C", Content: "CC"},
		}
		err := db.WithContext(ctx).Create(&blogs).Error
		require.NoError(t, err)

		// Query subset of IDs
		got, err := repo.ListBlogsByIDs(ctx, []uint64{blogs[0].ID, blogs[2].ID})
		require.NoError(t, err)
		require.Len(t, got, 2)
		ids := make(map[uint64]bool)
		for _, g := range got {
			ids[g.ID] = true
		}
		require.True(t, ids[blogs[0].ID])
		require.True(t, ids[blogs[2].ID])
	})

	t.Run("list blogs by IDs with non-existent IDs returns only existing", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		seed := &Blog{ShopID: 100, UserId: 1, Title: "Only", Content: "C"}
		err := db.WithContext(ctx).Create(seed).Error
		require.NoError(t, err)

		got, err := repo.ListBlogsByIDs(ctx, []uint64{seed.ID, 999999})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, seed.ID, got[0].ID)
	})

	t.Run("list blogs by IDs with empty IDs returns empty", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.ListBlogsByIDs(ctx, []uint64{})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("list blogs by IDs with nil IDs returns empty", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		got, err := repo.ListBlogsByIDs(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}
