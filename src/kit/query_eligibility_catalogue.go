package kit

import evidencepins "github.com/wotjr1649/tree-sitter-grammar-kit/src/contracts/evidence-pins"

// Retained for fixture builders; verification selects immutable approved objects directly.
var eligibilityCatalogueJSON = func() string {
	store, err := evidencepins.Repository()
	if err != nil {
		return ""
	}
	ref, err := store.Reference()
	if err != nil {
		return ""
	}
	data, err := store.Object(ref.CatalogueSHA256)
	if err != nil {
		return ""
	}
	return string(data)
}()
