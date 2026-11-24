package registry

// AquaFilter exposes aquaFilter to the tests.
func AquaFilter(expr string) (string, string) { return aquaFilter(expr) }
