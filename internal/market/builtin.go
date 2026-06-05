package market

// builtinCatalog returns an empty list as the last-resort fallback used when
// the charts directory is entirely missing (e.g. during bare unit-test runs
// without market assets). In production the Docker image always ships a
// populated charts/ directory.
func builtinCatalog() []MarketApp {
	return []MarketApp{}
}
