package keys

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/haakco/mcp-kit/audit"
)

// DefaultRotationInterval is the default automatic signing-key rotation cadence.
const DefaultRotationInterval = 90 * 24 * time.Hour

// DefaultRotationGrace is the default window during which retired keys remain
// in JWKS so existing tokens continue to verify.
const DefaultRotationGrace = 48 * time.Hour

const (
	minRotationInterval = time.Hour
	minRotationGrace    = time.Hour
)

// RotationConfig configures the rotation loop.
type RotationConfig struct {
	Interval time.Duration
	Grace    time.Duration
	Now      func() time.Time
}

func (c *RotationConfig) applyDefaults() {
	if c.Interval == 0 {
		c.Interval = DefaultRotationInterval
	}
	if c.Interval < minRotationInterval {
		c.Interval = minRotationInterval
	}
	if c.Grace == 0 {
		c.Grace = DefaultRotationGrace
	}
	if c.Grace < minRotationGrace {
		c.Grace = minRotationGrace
	}
	if c.Now == nil {
		c.Now = time.Now
	}
}

// RotatorOption configures a Rotator.
type RotatorOption func(*Rotator)

// WithAuditEmitter emits an audit event after successful key rotation.
func WithAuditEmitter(emitter audit.Emitter) RotatorOption {
	return func(rotator *Rotator) {
		rotator.auditEmitter = emitter
	}
}

// Rotator owns the periodic signing-key rotation loop.
type Rotator struct {
	manager      *Manager
	cfg          RotationConfig
	logger       *slog.Logger
	auditEmitter audit.Emitter
}

// NewRotator constructs a Rotator. Call Run once during server startup.
func NewRotator(manager *Manager, cfg RotationConfig, logger *slog.Logger, opts ...RotatorOption) *Rotator {
	cfg.applyDefaults()
	if logger == nil {
		logger = slog.Default()
	}

	rotator := &Rotator{
		manager: manager,
		cfg:     cfg,
		logger:  logger.With("component", "oauth.keys.rotator"),
	}
	for _, opt := range opts {
		opt(rotator)
	}
	return rotator
}

// Run blocks until ctx is cancelled.
func (r *Rotator) Run(ctx context.Context) {
	r.logger.Info("starting", "interval", r.cfg.Interval, "grace", r.cfg.Grace)
	defer r.logger.Info("stopped")

	delay, err := r.delayUntilFirstRotation(ctx)
	if err != nil {
		r.logger.Error("compute first rotation delay; falling back to interval", "error", err)
		delay = r.cfg.Interval
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.rotateAndCleanup(ctx)
			timer.Reset(r.cfg.Interval)
		}
	}
}

func (r *Rotator) delayUntilFirstRotation(ctx context.Context) (time.Duration, error) {
	current, err := r.manager.store.FindActiveSigningKey(ctx)
	switch {
	case errors.Is(err, ErrNotFound):
		return r.cfg.Interval, nil
	case err != nil:
		return 0, err
	}

	age := r.cfg.Now().Sub(current.CreatedAt)
	if age >= r.cfg.Interval {
		return 0, nil
	}
	return r.cfg.Interval - age, nil
}

func (r *Rotator) rotateAndCleanup(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	created, err := r.manager.RotateSigningKey(ctx, r.cfg.Grace)
	switch {
	case err == nil:
		r.logger.Info("rotated signing key", "kid", created.KID, "grace", r.cfg.Grace)
		r.emitKeyRotated(ctx, created.KID)
	case errors.Is(err, context.Canceled):
		return
	default:
		r.logger.Error("rotate signing key failed", "error", err)
	}

	deleted, err := r.manager.RetireExpiredKeys(ctx)
	if err != nil {
		r.logger.Error("retire expired keys failed", "error", err)
		return
	}
	if deleted > 0 {
		r.logger.Info("retired expired signing keys", "deleted", deleted)
	}
}

func (r *Rotator) emitKeyRotated(ctx context.Context, newKID string) {
	if r.auditEmitter == nil {
		return
	}
	if err := r.auditEmitter.Emit(ctx, audit.Event{
		EntityType: "oauth_key",
		EntityID:   newKID,
		Action:     "rotated",
		Timestamp:  r.cfg.Now(),
	}); err != nil {
		r.logger.Error("emit key_rotated audit event", "error", err)
	}
}
