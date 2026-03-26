package market

// builtinCatalog returns an empty catalog.
// Run market sync to populate the catalog from the Olares appstore.
// No external URLs — everything must be local.
func builtinCatalog() []MarketApp {
	return []MarketApp{}
}
