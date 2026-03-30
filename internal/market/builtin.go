package market

// builtinCatalog returns an empty catalog.
// The catalog is loaded from /data/market/catalog.json (baked into the image).
func builtinCatalog() []MarketApp {
	return []MarketApp{}
}
