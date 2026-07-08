//go:build integration
package follow

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

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "loc=Local", "charset=utf8mb4")
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() {
		require.NoError(t, tx.Rollback().Error)
	})

	return NewRepository(tx), tx
}

// =============================================================================
// Follow
// =============================================================================

func TestRepository_Follow(t *testing.T) {
	t.Run("follow user successfully", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		created, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, created)

		// Verify the record was inserted into the database
		var count int64
		err = db.WithContext(ctx).Model(&Follow{}).
			Where("user_id = ? AND follow_user_id = ?", 1, 2).
			Count(&count).Error
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})

	t.Run("duplicate follow is idempotent", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// First follow
		created, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, created)

		// Second follow of the same pair
		created, err = repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		require.False(t, created) // idempotent, no new row affected

		// Verify only one record exists
		var count int64
		err = db.WithContext(ctx).Model(&Follow{}).
			Where("user_id = ? AND follow_user_id = ?", 1, 2).
			Count(&count).Error
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})

	t.Run("different user pairs create separate records", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		created1, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, created1)

		created2, err := repo.Follow(ctx, 1, 3)
		require.NoError(t, err)
		require.True(t, created2)

		created3, err := repo.Follow(ctx, 2, 1)
		require.NoError(t, err)
		require.True(t, created3)

		var count int64
		err = db.WithContext(ctx).Model(&Follow{}).Count(&count).Error
		require.NoError(t, err)
		require.Equal(t, int64(3), count)
	})
}

// =============================================================================
// Unfollow
// =============================================================================

func TestRepository_Unfollow(t *testing.T) {
	t.Run("unfollow existing relationship", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Create a follow relationship first
		_, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)

		// Unfollow
		deleted, err := repo.Unfollow(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, deleted)

		// Verify the record is gone
		var count int64
		err = db.WithContext(ctx).Model(&Follow{}).
			Where("user_id = ? AND follow_user_id = ?", 1, 2).
			Count(&count).Error
		require.NoError(t, err)
		require.Equal(t, int64(0), count)
	})

	t.Run("unfollow non-existent relationship returns false", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		deleted, err := repo.Unfollow(ctx, 1, 999)
		require.NoError(t, err)
		require.False(t, deleted)
	})

	t.Run("unfollow only deletes the specified pair", func(t *testing.T) {
		repo, db := setUpRepository(t)
		ctx := context.Background()

		// Setup: user 1 follows users 2 and 3
		_, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		_, err = repo.Follow(ctx, 1, 3)
		require.NoError(t, err)

		// Unfollow only user 2
		deleted, err := repo.Unfollow(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, deleted)

		// User 3 should still be followed
		var count int64
		err = db.WithContext(ctx).Model(&Follow{}).
			Where("user_id = ? AND follow_user_id = ?", 1, 3).
			Count(&count).Error
		require.NoError(t, err)
		require.Equal(t, int64(1), count)
	})
}

// =============================================================================
// IsFollowed
// =============================================================================

func TestRepository_IsFollowed(t *testing.T) {
	t.Run("returns true when following exists", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		_, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)

		isFollowed, err := repo.IsFollowed(ctx, 1, 2)
		require.NoError(t, err)
		require.True(t, isFollowed)
	})

	t.Run("returns false when not following", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		isFollowed, err := repo.IsFollowed(ctx, 1, 999)
		require.NoError(t, err)
		require.False(t, isFollowed)
	})

	t.Run("direction matters: A follows B does not mean B follows A", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// User 1 follows user 2
		_, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)

		// User 2 does NOT follow user 1 (unless explicitly created)
		isFollowed, err := repo.IsFollowed(ctx, 2, 1)
		require.NoError(t, err)
		require.False(t, isFollowed)
	})

	t.Run("empty table returns false", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		isFollowed, err := repo.IsFollowed(ctx, 1, 1)
		require.NoError(t, err)
		require.False(t, isFollowed)
	})
}

// =============================================================================
// ListFollowerUserIDs
// =============================================================================

func TestRepository_ListFollowerUserIDs(t *testing.T) {
	t.Run("returns all follower IDs for a user", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// Users 2, 3, and 4 follow user 1
		_, err := repo.Follow(ctx, 2, 1)
		require.NoError(t, err)
		_, err = repo.Follow(ctx, 3, 1)
		require.NoError(t, err)
		_, err = repo.Follow(ctx, 4, 1)
		require.NoError(t, err)
		// User 1 follows user 5 (not a follower of 1)
		_, err = repo.Follow(ctx, 1, 5)
		require.NoError(t, err)

		followerIDs, err := repo.ListFollowerUserIDs(ctx, 1)
		require.NoError(t, err)
		require.ElementsMatch(t, []uint64{2, 3, 4}, followerIDs)
	})

	t.Run("returns empty slice when no followers", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		followerIDs, err := repo.ListFollowerUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Empty(t, followerIDs)
	})

	t.Run("excludes users that are followed but not followers", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// User 1 follows user 2 (user 1 is NOT a follower of user 2)
		_, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		// User 3 follows user 2
		_, err = repo.Follow(ctx, 3, 2)
		require.NoError(t, err)

		// For user 2, only user 3 is a follower
		followerIDs, err := repo.ListFollowerUserIDs(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, []uint64{3}, followerIDs)
	})
}

// =============================================================================
// ListFollowedUserIDs
// =============================================================================

func TestRepository_ListFollowedUserIDs(t *testing.T) {
	t.Run("returns all followed user IDs for a user", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// User 1 follows users 2, 3, and 4
		_, err := repo.Follow(ctx, 1, 2)
		require.NoError(t, err)
		_, err = repo.Follow(ctx, 1, 3)
		require.NoError(t, err)
		_, err = repo.Follow(ctx, 1, 4)
		require.NoError(t, err)
		// User 5 follows user 1 (not followed by 1)
		_, err = repo.Follow(ctx, 5, 1)
		require.NoError(t, err)

		followedIDs, err := repo.ListFollowedUserIDs(ctx, 1)
		require.NoError(t, err)
		require.ElementsMatch(t, []uint64{2, 3, 4}, followedIDs)
	})

	t.Run("returns empty slice when no followed users", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		followedIDs, err := repo.ListFollowedUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Empty(t, followedIDs)
	})

	t.Run("excludes followers that are not followed", func(t *testing.T) {
		repo, _ := setUpRepository(t)
		ctx := context.Background()

		// User 2 follows user 1 (user 2 is a follower of user 1)
		_, err := repo.Follow(ctx, 2, 1)
		require.NoError(t, err)
		// User 1 follows user 3
		_, err = repo.Follow(ctx, 1, 3)
		require.NoError(t, err)

		// For user 1, only user 3 is followed
		followedIDs, err := repo.ListFollowedUserIDs(ctx, 1)
		require.NoError(t, err)
		require.Equal(t, []uint64{3}, followedIDs)
	})
}
