package remote

// ChartTagPrefix exposes chartTagPrefix to the tests.
func ChartTagPrefix(chart string, versions, tags []string) (string, bool) {
	return chartTagPrefix(chart, versions, tags)
}
