package main

import (
	"maps"
	"sync"
)

// Parsing the committed contracts (inventory, svc and inline assets, ordinary claims) is about half of every
// merge call; the contracts do not change within a test binary, so the parse is shared across the tests.
var (
	contractsMu    sync.Mutex
	contractsCache = map[string]contracts{}
)

func init() {
	contractsFor = func(root string) (contracts, error) {
		contractsMu.Lock()
		defer contractsMu.Unlock()
		c, ok := contractsCache[root]
		if !ok {
			var err error
			if c, err = loadContracts(root); err != nil {
				return c, err
			}
			contractsCache[root] = c
		}
		// mergeRun writes into req.ResultPins and the report keeps claims: hand each call its own maps
		c.req.ResultPins, c.claims = maps.Clone(c.req.ResultPins), maps.Clone(c.claims)
		return c, nil
	}
}
