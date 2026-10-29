package computer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/z-chenhao/collie/internal/llm"
	"path"
	"regexp"
	"strings"
)

var (
	ErrElement       = errors.New("stale Computer element")
	ErrInvalid       = errors.New("invalid Computer request")
	ErrUnavailable   = errors.New("Computer provider unavailable")
	ErrBusy          = errors.New("Computer is in use")
	ErrStale         = errors.New("stale Computer epoch or owner")
	ErrIntegrity     = errors.New("Computer checkpoint integrity failure")
	ErrCompatibility = errors.New("Computer compatibility snapshot changed")
	ErrUnknown       = errors.New("Computer operation outcome unknown")
)

// Compatibility is frozen by the Host. It contains no credential or transport
// address and is never replaced during recovery of an accepted operation.
type Compatibility struct {
	ProviderVersion string `json:"provider_version" yaml:"provider_version"`
	GuestBuild      string `json:"guest_build" yaml:"guest_build"`
	DriverVersion   string `json:"driver_version" yaml:"driver_version"`
	DriverSHA256    string `json:"driver_sha256" yaml:"driver_sha256"`
	DriverTeam      string `json:"driver_team" yaml:"driver_team"`
	DriverBundle    string `json:"driver_bundle" yaml:"driver_bundle"`
	Architecture    string `json:"architecture" yaml:"architecture"`
}

type ApplicationIntent struct {
	BundleID  string   `json:"bundle_id" yaml:"bundle_id"`
	Documents []string `json:"documents" yaml:"documents"`
}

// RestorationManifest stores only semantic reopening intent and stable Artifact
// references. Driver sessions, surface handles and JavaScript state have no field.
type RestorationManifest struct {
	Version      int                 `json:"version" yaml:"version"`
	Applications []ApplicationIntent `json:"applications" yaml:"applications"`
	ArtifactRefs []string            `json:"artifact_refs" yaml:"artifact_refs"`
}

var bundleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,254}$`)

func (m RestorationManifest) Validate() error {
	if m.Version != 1 || len(m.Applications) > 32 || len(m.ArtifactRefs) > 128 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, app := range m.Applications {
		if !bundleID.MatchString(app.BundleID) || seen[app.BundleID] || len(app.Documents) > 32 {
			return ErrInvalid
		}
		seen[app.BundleID] = true
		for _, document := range app.Documents {
			if !path.IsAbs(document) || path.Clean(document) != document || len(document) > 4096 || strings.ContainsAny(document, "\x00\r\n") {
				return ErrInvalid
			}
		}
	}
	for _, ref := range m.ArtifactRefs {
		if !strings.HasPrefix(ref, "artifact:") || len(ref) > 128 || strings.ContainsAny(ref, "\x00\r\n") {
			return ErrInvalid
		}
	}
	return nil
}

func Fingerprint(value any) (string, error) {
	raw, err := llm.CanonicalJSON(value)
	if err != nil {
		return "", ErrInvalid
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// Instance is an internal provider handle. Only ID and Epoch cross the Computer
// Use boundary. Native VM names and addresses never become model parameters.
type Instance struct {
	ComputerID string `json:"computer_id"`
	Epoch      int64  `json:"instance_epoch"`
	NativeName string `json:"native_name"`
}

type Request struct {
	Instance      Instance            `json:"instance"`
	OperationID   string              `json:"operation_id"`
	CheckpointID  string              `json:"checkpoint_id"`
	Compatibility Compatibility       `json:"compatibility"`
	Manifest      RestorationManifest `json:"manifest"`
	// Fence checks the accepted Host execution before an effect and completion.
	// Providers receive no domain database and cannot construct this authority.
	Fence func(context.Context) error `json:"-"`
}

type Checkpoint struct {
	ID             string `json:"checkpoint_id"`
	ComputerID     string `json:"computer_id"`
	SourceEpoch    int64  `json:"source_epoch"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type Status struct {
	ComputerID string      `json:"computer_id"`
	Epoch      int64       `json:"instance_epoch"`
	Phase      string      `json:"phase"`
	Checkpoint *Checkpoint `json:"checkpoint"`
}

// Provider manages instances and durable units. Desktop operations belong to
// ComputerUseRuntime, not this interface. Warm memory snapshots are optional.
type Provider interface {
	Ensure(context.Context, Request) (Instance, error)
	Checkpoint(context.Context, Request) (Checkpoint, error)
	Restore(context.Context, Request, Checkpoint) (Instance, error)
	Destroy(context.Context, Request) error
}
