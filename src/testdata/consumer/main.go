// Command consumer is an external-module template: tests copy it outside the checkout
// with go.mod.tmpl and import only the public kit package.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func main() {
	root, grammar := os.Args[1], os.Args[2]
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sel := kit.Selection{Grammar: grammar}
	inspect, err := kit.Inspect(ctx, kit.InspectRequest{Root: root, Selection: sel, Limits: kit.DefaultLimits()})
	emit("inspect", inspect, err)
	identity, err := kit.Identity(ctx, kit.IdentityRequest{Root: root, Selection: sel, Limits: kit.DefaultLimits()})
	emit("identity", identity, err)
	if len(os.Args) < 4 {
		return
	}
	expected, err := os.ReadFile(os.Args[3])
	if err != nil {
		panic(err)
	}
	verify, err := kit.Verify(ctx, kit.VerifyRequest{Root: root, Selection: sel, Expected: expected, Limits: kit.DefaultLimits(), ArchiveLimits: kit.DefaultArchiveLimits()})
	emit("verify", verify, err)
	if len(os.Args) < 5 {
		return
	}
	archive, err := kit.Verify(ctx, kit.VerifyRequest{Archive: os.Args[4], Expected: expected, Limits: kit.DefaultLimits(), ArchiveLimits: kit.DefaultArchiveLimits()})
	emit("verify-archive", archive, err)
}

func emit(name string, v any, err error) {
	data, merr := json.Marshal(v)
	if merr != nil {
		panic(merr)
	}
	code := "-"
	var e *kit.Error
	if errors.As(err, &e) {
		code = e.Kind + "/" + e.Code
	}
	fmt.Printf("%s\t%s\t%s\n", name, code, data)
}
