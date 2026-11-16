package github

// RecentBatchSize exposes recentBatchSize to the external tests.
func RecentBatchSize(n, concurrency int) int { return recentBatchSize(n, concurrency) }
