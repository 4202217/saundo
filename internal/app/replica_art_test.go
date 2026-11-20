//go:build preview

package app

import (
	"encoding/json"
	"os"
	"testing"
)

// The landing page's artwork, cover colours, cover lamps, and template bases
// are float-derived: the art helpers use fused multiply-add on arm64 and not
// on amd64, so encoding them twice yields different bytes on the two CI hosts.
// The dataset's prose and structure still come from the app fixture; only the
// art bytes are pinned here. The fixture was generated from the committed
// dataset and is replayed verbatim by replicaDataValue.

const replicaArtPath = "testdata/replica-art.json"

type replicaArt struct {
	Marks     map[string]string             `json:"marks"`
	Covers    map[string]replicaArtCover    `json:"covers"`
	Templates map[string]replicaArtTemplate `json:"templates"`
}

type replicaArtCover struct {
	Cover  string       `json:"cover"`
	Colors []string     `json:"colors"`
	Lamp   *replicaLamp `json:"lamp"`
}

type replicaArtTemplate struct {
	Base  string        `json:"base"`
	Blobs []replicaBlob `json:"blobs"`
}

func loadReplicaArt(t *testing.T) replicaArt {
	t.Helper()
	b, err := os.ReadFile(replicaArtPath)
	if err != nil {
		t.Fatalf("%s: %v", replicaArtPath, err)
	}
	var art replicaArt
	if err := json.Unmarshal(b, &art); err != nil {
		t.Fatalf("%s: %v", replicaArtPath, err)
	}
	return art
}

// replicaArtCoverKey is the stable name a track's artwork is pinned under.
// The English fixture renames the first track; the artwork belongs to the
// same painting, so both names resolve to one fixture entry.
func replicaArtCoverKey(title string) string {
	if canonical, ok := coverAliases[title]; ok {
		return canonical
	}
	return title
}
