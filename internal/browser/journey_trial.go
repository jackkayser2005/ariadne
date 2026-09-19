package browser

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"runtime"
	"slices"
	"time"
)

// TrialIdentity binds two fresh captures to one local experiment. Identifiers
// are random, never hashes of synthetic values or destination names.
type TrialIdentity struct {
	PairID     string `json:"pair_id"`
	InputSetID string `json:"input_set_id"`
	Role       string `json:"role"`
	Order      string `json:"order"`
}

// TrialSettings is private context recording the controls actually installed
// by the capture engine. It never appears in a portable journey export.
type TrialSettings struct {
	SchemaVersion int           `json:"schema_version"`
	Identity      TrialIdentity `json:"identity"`
	StartedAt     int64         `json:"started_at"`
	Browser       string        `json:"browser"`
	Platform      string        `json:"platform"`
	Location      string        `json:"location"`
	BlockOrigins  []string      `json:"block_origins"`
}

// NewTrialIdentity allocates a paired experiment without retaining input values.
func NewTrialIdentity(order string) TrialIdentity {
	identity := func() string { sum := sha256.Sum256([]byte(rand.Text())); return hex.EncodeToString(sum[:]) }
	return TrialIdentity{PairID: identity(), InputSetID: identity(), Role: "baseline", Order: order}
}

func (identity TrialIdentity) validate() error {
	if !journeyDigest(identity.PairID) || !journeyDigest(identity.InputSetID) || !slices.Contains([]string{"baseline", "treatment"}, identity.Role) || !slices.Contains([]string{"baseline-treatment", "treatment-baseline"}, identity.Order) {
		return errors.New("trial identity is invalid")
	}
	return nil
}

func journeyDigest(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == 32 && value == hex.EncodeToString(data)
}

func newTrialSettings(identity TrialIdentity, product string, options CaptureOptions) (*TrialSettings, error) {
	settings := &TrialSettings{SchemaVersion: 1, Identity: identity, StartedAt: time.Now().UnixNano(), Browser: product, Platform: runtime.GOOS, Location: options.Location, BlockOrigins: slices.Clone(options.BlockOrigins)}
	if settings.BlockOrigins == nil {
		settings.BlockOrigins = []string{}
	}
	slices.Sort(settings.BlockOrigins)
	settings.BlockOrigins = slices.Compact(settings.BlockOrigins)
	return settings, settings.validate()
}

func (settings TrialSettings) validate() error {
	if settings.SchemaVersion != 1 || settings.Identity.validate() != nil || settings.StartedAt <= 0 || !regexp.MustCompile(`^(HeadlessChrome|Chrome|Edg)/[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(settings.Browser) || len(settings.Browser) > 80 || !slices.Contains([]string{"windows", "linux", "darwin"}, settings.Platform) || !slices.Contains([]string{"unchanged", "deny", "approximate"}, settings.Location) || settings.BlockOrigins == nil || len(settings.BlockOrigins) > 64 {
		return errors.New("trial settings are invalid")
	}
	if settings.Identity.Role == "baseline" && (settings.Location != "unchanged" || len(settings.BlockOrigins) > 0) {
		return errors.New("baseline must use unchanged controls")
	}
	for i, origin := range settings.BlockOrigins {
		u, err := InvestigationURL(origin)
		if err != nil || u.Scheme+"://"+u.Host != origin || (i > 0 && settings.BlockOrigins[i-1] >= origin) {
			return errors.New("trial blocked origins are invalid")
		}
	}
	return nil
}
