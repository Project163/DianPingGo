package follow

import "time"

const (
	BizFollowKey      = "follows:" // userID
	BizFollowedSuffix = ":loaded"  // followUserID
	BizFollowerTTL    = 7 * 24 * time.Hour
)
