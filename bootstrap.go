package nodejs

import (
	_ "embed"

	"github.com/KarpelesLab/pjson"
)

//go:embed bootstrap.js
var bootstrapJs string

var bootstrapEnc, _ = pjson.Marshal(bootstrapJs)

// GetBootstrapContent returns the raw bootstrap.js content (for debugging)
func GetBootstrapContent() string {
	return bootstrapJs
}
