// Command query-reference-run supervises a reviewed reference build or execution.
package main

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
)

func run(in io.Reader, out io.Writer) error {
	b, err := io.ReadAll(io.LimitReader(in, 65537))
	if err != nil || len(b) > 65536 {
		return fmt.Errorf("request read/limit")
	}
	var spec runner.Spec
	if err := jsonv2.Unmarshal(b, &spec, jsonv2.RejectUnknownMembers(true), json.FormatDurationAsNano(true)); err != nil {
		return fmt.Errorf("request JSON: %w", err)
	}
	if spec.Wall <= 0 || spec.Wall > 15*time.Minute || spec.Grace <= 0 || spec.Grace > 10*time.Second || spec.Memory.Bytes > 4<<30 || spec.StdoutBytes > 64<<20 || spec.StderrBytes > 4<<20 || spec.Interactive {
		return fmt.Errorf("request exceeds reference limits")
	}
	ctx, cancel := context.WithTimeout(context.Background(), spec.Wall+spec.Grace+5*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, spec)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(struct {
		Execution runner.Result `json:"execution"`
		Stdout    string        `json:"stdout"`
		Stderr    string        `json:"stderr"`
	}{result, string(result.Stdout), string(result.Stderr)}); err != nil {
		return err
	}
	if !result.Succeeded() {
		return fmt.Errorf("reference process: %s/%s", result.Status, result.Reason)
	}
	return nil
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
