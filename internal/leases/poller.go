// Package leases ingests on-chain lease state via the provider-services CLI so
// the service knows which (deployer, provider, dseq) tuples really transacted.
package leases

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"time"

	"github.com/shimpa1/akash-reputation/internal/store"
)

// Config controls the lease poller.
type Config struct {
	Bin      string        // path to provider-services binary
	Node     string        // --node RPC endpoint, e.g. https://your-akash-rpc:443
	Provider string        // provider address to scope lease queries to
	Interval time.Duration // how often to refresh
	PageSize int           // leases per page
}

// Poller periodically syncs leases into the store.
type Poller struct {
	cfg   Config
	store *store.Store
	log   *slog.Logger
}

// New builds a Poller, applying sensible defaults.
func New(cfg Config, st *store.Store, log *slog.Logger) *Poller {
	if cfg.Bin == "" {
		cfg.Bin = "provider-services"
	}
	if cfg.Interval == 0 {
		cfg.Interval = 10 * time.Minute
	}
	if cfg.PageSize == 0 {
		cfg.PageSize = 200
	}
	return &Poller{cfg: cfg, store: st, log: log}
}

// Run syncs immediately, then on Interval until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.cfg.Interval)
	defer t.Stop()
	if err := p.Sync(ctx); err != nil {
		p.log.Error("initial lease sync failed", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := p.Sync(ctx); err != nil {
				p.log.Error("lease sync failed", "err", err)
			}
		}
	}
}

// leaseListResponse mirrors `provider-services query market lease list -o json`.
type leaseListResponse struct {
	Leases []struct {
		Lease struct {
			ID struct {
				Owner    string `json:"owner"`
				DSeq     string `json:"dseq"`
				GSeq     int    `json:"gseq"`
				OSeq     int    `json:"oseq"`
				Provider string `json:"provider"`
			} `json:"id"`
			State string `json:"state"`
		} `json:"lease"`
	} `json:"leases"`
	Pagination struct {
		Total string `json:"total"`
	} `json:"pagination"`
}

// leaseStates are queried individually: the chain requires a state filter
// whenever pagination uses an offset (page > 1).
var leaseStates = []string{"active", "closed", "insufficient_funds"}

// Sync pulls every page of leases (across all states) for the configured
// provider and upserts them.
func (p *Poller) Sync(ctx context.Context) error {
	var total int
	for _, state := range leaseStates {
		for page := 1; ; page++ {
			resp, err := p.queryPage(ctx, state, page)
			if err != nil {
				return fmt.Errorf("state %s page %d: %w", state, page, err)
			}
			for _, l := range resp.Leases {
				rec := store.Lease{
					Owner:    l.Lease.ID.Owner,
					DSeq:     l.Lease.ID.DSeq,
					GSeq:     strconv.Itoa(l.Lease.ID.GSeq),
					OSeq:     strconv.Itoa(l.Lease.ID.OSeq),
					Provider: l.Lease.ID.Provider,
					State:    l.Lease.State,
				}
				if err := p.store.UpsertLease(ctx, rec); err != nil {
					return fmt.Errorf("upsert lease %s/%s: %w", rec.Owner, rec.DSeq, err)
				}
				total++
			}
			if len(resp.Leases) < p.cfg.PageSize {
				break
			}
		}
	}
	p.log.Info("lease sync complete", "leases", total)
	return nil
}

func (p *Poller) queryPage(ctx context.Context, state string, page int) (*leaseListResponse, error) {
	args := []string{
		"query", "market", "lease", "list",
		"--provider", p.cfg.Provider,
		"--state", state,
		"--page", strconv.Itoa(page),
		"--limit", strconv.Itoa(p.cfg.PageSize),
		"-o", "json",
	}
	if p.cfg.Node != "" {
		args = append(args, "--node", p.cfg.Node)
	}
	cmd := exec.CommandContext(ctx, p.cfg.Bin, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("provider-services: %w", err)
	}
	return decodeLeaseList(out)
}

// decodeLeaseList parses lease-list output, tolerating non-JSON noise (e.g. the
// "minimum-gas-prices is not set" warning) that provider-services may emit
// before the JSON document.
func decodeLeaseList(out []byte) (*leaseListResponse, error) {
	if i := bytes.IndexByte(out, '{'); i > 0 {
		out = out[i:]
	}
	var resp leaseListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("decode lease list: %w", err)
	}
	return &resp, nil
}

// VerifyLeaseLive does a direct single-lease lookup as a fallback when the cache
// has no record yet. It returns true if the exact lease identity exists on-chain.
func (p *Poller) VerifyLeaseLive(ctx context.Context, owner, dseq, gseq, oseq string) (bool, error) {
	args := []string{
		"query", "market", "lease", "list",
		"--owner", owner, "--dseq", dseq, "--gseq", gseq, "--oseq", oseq,
		"--provider", p.cfg.Provider, "-o", "json",
	}
	if p.cfg.Node != "" {
		args = append(args, "--node", p.cfg.Node)
	}
	out, err := exec.CommandContext(ctx, p.cfg.Bin, args...).Output()
	if err != nil {
		return false, fmt.Errorf("provider-services: %w", err)
	}
	resp, err := decodeLeaseList(out)
	if err != nil {
		return false, err
	}
	return len(resp.Leases) > 0, nil
}
